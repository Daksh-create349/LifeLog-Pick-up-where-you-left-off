package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

var recordSeqCounter uint64

// GenerateRecordID produces a unique, timestamp-ordered identifier for terminal records.
func GenerateRecordID() string {
	seq := atomic.AddUint64(&recordSeqCounter, 1)
	return fmt.Sprintf("term_%d_%04d", time.Now().UTC().UnixNano(), seq%10000)
}

// Execute runs the specified command, streaming stdout/stderr directly to the user,
// sanitizes the command metadata for privacy, and returns a populated Record and exit code.
func Execute(
	ctx context.Context,
	workingDir string,
	rawArgs []string,
	isShell bool,
	stdout, stderr io.Writer,
	stdin io.Reader,
) (*Record, int, error) {
	if len(rawArgs) == 0 {
		return nil, 2, errors.New("no command specified")
	}

	// 1. Resolve canonical working directory
	absDir, err := filepath.Abs(workingDir)
	if err != nil {
		absDir = workingDir
	}
	if resolved, err := filepath.EvalSymlinks(absDir); err == nil {
		absDir = resolved
	}

	// 2. Perform privacy sanitization upfront
	sanitized := Sanitize(rawArgs, isShell)

	// 3. Inform the user that LIFELOG is recording execution
	fmt.Fprintf(stderr, "[lifelog] Recording command: %s\n", sanitized.SanitizedCommand)

	// 4. Construct subprocess
	collectionMode := "direct"
	var cmd *exec.Cmd
	if isShell {
		collectionMode = "shell"
		shellCmd := strings.Join(rawArgs, " ")
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", shellCmd)
	} else {
		cmd = exec.CommandContext(ctx, rawArgs[0], rawArgs[1:]...)
	}

	cmd.Dir = absDir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// 5. Setup signal handling for graceful cancellation/forwarding
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	startTime := time.Now().UTC()

	if err := cmd.Start(); err != nil {
		endTime := time.Now().UTC()
		durationMs := endTime.Sub(startTime).Milliseconds()

		rec := &Record{
			ID:               GenerateRecordID(),
			WorkingDir:       absDir,
			Executable:       sanitized.Executable,
			SanitizedArgs:    sanitized.Args,
			SanitizedCommand: sanitized.SanitizedCommand,
			StartTime:        startTime,
			EndTime:          endTime,
			DurationMs:       durationMs,
			ExitCode:         nil, // Not started
			CollectionMode:   collectionMode,
			IsRedacted:       sanitized.IsRedacted,
			IsOmitted:        sanitized.IsOmitted,
			OmissionReason:   sanitized.OmissionReason,
			CreatedAt:        endTime,
		}

		exitCode := 1
		if errors.Is(err, exec.ErrNotFound) {
			exitCode = 127
		}
		return rec, exitCode, fmt.Errorf("starting command: %w", err)
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	var waitErr error
	var wasInterrupted bool

	select {
	case sig := <-sigChan:
		wasInterrupted = true
		if cmd.Process != nil {
			_ = cmd.Process.Signal(sig)
		}
		waitErr = <-waitDone
	case <-ctx.Done():
		wasInterrupted = true
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		waitErr = <-waitDone
	case waitErr = <-waitDone:
	}

	endTime := time.Now().UTC()
	durationMs := endTime.Sub(startTime).Milliseconds()

	var exitCode *int
	retCode := 0

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			ws, ok := exitErr.Sys().(syscall.WaitStatus)
			if ok && ws.Signaled() {
				// Interrupted or terminated by signal; do not invent an exit code
				exitCode = nil
				retCode = 128 + int(ws.Signal())
			} else {
				code := exitErr.ExitCode()
				exitCode = &code
				retCode = code
			}
		} else {
			retCode = 1
		}
	} else {
		zero := 0
		exitCode = &zero
		retCode = 0
	}

	if wasInterrupted && retCode == 0 {
		retCode = 130 // standard SIGINT exit status
	}

	rec := &Record{
		ID:               GenerateRecordID(),
		WorkingDir:       absDir,
		Executable:       sanitized.Executable,
		SanitizedArgs:    sanitized.Args,
		SanitizedCommand: sanitized.SanitizedCommand,
		StartTime:        startTime,
		EndTime:          endTime,
		DurationMs:       durationMs,
		ExitCode:         exitCode,
		CollectionMode:   collectionMode,
		IsRedacted:       sanitized.IsRedacted,
		IsOmitted:        sanitized.IsOmitted,
		OmissionReason:   sanitized.OmissionReason,
		CreatedAt:        endTime,
	}

	return rec, retCode, nil
}
