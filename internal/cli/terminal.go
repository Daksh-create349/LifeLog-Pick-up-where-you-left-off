package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lifelog/lifelog/internal/terminal"
)

func runTerminal(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printTerminalUsage(stdout)
		return ExitSuccess
	}

	switch args[0] {
	case "record":
		return runTerminalRecord(args[1:], stdout, stderr)
	case "history", "list":
		return runTerminalHistory(args[1:], stdout, stderr)
	case "inspect", "show":
		return runTerminalInspect(args[1:], stdout, stderr)
	case "delete":
		return runTerminalDelete(args[1:], stdout, stderr)
	case "clear":
		return runTerminalClear(args[1:], stdout, stderr)
	case "help", "--help", "-h":
		printTerminalUsage(stdout)
		return ExitSuccess
	default:
		fmt.Fprintf(stderr, "Unknown terminal subcommand: %s\nRun '%s terminal help' for usage details.\n", args[0], AppName)
		return ExitUsageError
	}
}

func printTerminalUsage(w io.Writer) {
	fmt.Fprintf(w, "LIFELOG Privacy-First Terminal Memory Engine\n\n")
	fmt.Fprintf(w, "Usage:\n  %s terminal <command> [arguments]\n\n", AppName)
	fmt.Fprintf(w, "Commands:\n")
	fmt.Fprintf(w, "  record [--shell] [--] <cmd> [args...]  Execute command, stream output, and record metadata\n")
	fmt.Fprintf(w, "  history [--limit N] [--dir /path]      List recorded command history (alias: list)\n")
	fmt.Fprintf(w, "  inspect <record-id>                    Display details of a recorded execution (alias: show)\n")
	fmt.Fprintf(w, "  delete <record-id>                     Delete a single terminal execution record\n")
	fmt.Fprintf(w, "  clear --confirm                        Clear all recorded terminal command history\n")
	fmt.Fprintf(w, "  help                                   Show this help information\n\n")
	fmt.Fprintf(w, "Privacy Guarantees:\n")
	fmt.Fprintf(w, "  - Command output (stdout/stderr) is never stored or indexed\n")
	fmt.Fprintf(w, "  - Sensitive flags, passwords, API keys, tokens, and assignments are redacted\n")
	fmt.Fprintf(w, "  - Credential managers and auth tools (pass, security, op, vault) have details omitted\n")
	fmt.Fprintf(w, "  - Direct execution by default (no shell invocation unless --shell is explicitly specified)\n")
}

func runTerminalRecord(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "Usage: %s terminal record [--shell] [--] <command> [args...]\n", AppName)
		return ExitUsageError
	}

	var isShell bool
	var cmdArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--shell" || arg == "-s" {
			isShell = true
			continue
		}
		if arg == "--" {
			cmdArgs = args[i+1:]
			break
		}
		if !strings.HasPrefix(arg, "-") || len(cmdArgs) > 0 {
			cmdArgs = args[i:]
			break
		}
	}

	if len(cmdArgs) == 0 {
		fmt.Fprintf(stderr, "Error: No command specified to record.\nUsage: %s terminal record [--shell] [--] <command> [args...]\n", AppName)
		return ExitUsageError
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	ctx := context.Background()

	// Execute command, stream output to stdout/stderr, sanitize metadata
	rec, exitStatus, execErr := terminal.Execute(ctx, cwd, cmdArgs, isShell, stdout, stderr, os.Stdin)

	// Persist record to database
	if rec != nil {
		db, _, err := openDB()
		if err != nil {
			fmt.Fprintf(stderr, "[lifelog] Warning: Could not open database to save terminal record: %v\n", err)
		} else {
			defer db.Close()
			saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := db.RecordTerminalExecution(saveCtx, rec); err != nil {
				fmt.Fprintf(stderr, "[lifelog] Warning: Failed to persist terminal record: %v\n", err)
			}
		}
	}

	if execErr != nil && exitStatus != 0 {
		fmt.Fprintf(stderr, "[lifelog] Execution error: %v\n", execErr)
	}

	return exitStatus
}

func runTerminalHistory(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("terminal history", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var limit int
	var dirFilter string
	var chronological bool

	fs.IntVar(&limit, "limit", 20, "maximum number of records to return")
	fs.StringVar(&dirFilter, "dir", "", "filter records by working directory path")
	fs.BoolVar(&chronological, "chronological", false, "display in chronological order (default: latest first)")

	if err := fs.Parse(args); err != nil {
		return ExitUsageError
	}

	db, _, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	if dirFilter != "" {
		if abs, err := filepath.Abs(dirFilter); err == nil {
			if resolved, err := filepath.EvalSymlinks(abs); err == nil {
				dirFilter = resolved
			} else {
				dirFilter = abs
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	records, err := db.ListTerminalRecords(ctx, terminal.RecordFilter{
		WorkingDir:    dirFilter,
		Limit:         limit,
		Chronological: chronological,
	})
	if err != nil {
		fmt.Fprintf(stderr, "Error querying terminal history: %v\n", err)
		return ExitGeneralErr
	}

	if len(records) == 0 {
		if dirFilter != "" {
			fmt.Fprintf(stdout, "No recorded terminal commands found for directory %s.\n", dirFilter)
		} else {
			fmt.Fprintf(stdout, "No recorded terminal commands found.\nRun '%s terminal record -- <command>' to record a command.\n", AppName)
		}
		return ExitSuccess
	}

	fmt.Fprintf(stdout, "LIFELOG Terminal Command History (showing %d)\n", len(records))
	fmt.Fprintf(stdout, "%s\n", strings.Repeat("=", 88))
	fmt.Fprintf(stdout, "%-22s %-10s %-10s %-24s %s\n", "RECORD ID", "STATUS", "DURATION", "START TIME (UTC)", "COMMAND")

	for _, r := range records {
		statusStr := "OK"
		if r.ExitCode == nil {
			statusStr = "SIGNAL"
		} else if *r.ExitCode != 0 {
			statusStr = fmt.Sprintf("ERR (%d)", *r.ExitCode)
		}

		durationStr := fmt.Sprintf("%dms", r.DurationMs)
		if r.DurationMs >= 1000 {
			durationStr = fmt.Sprintf("%.2fs", float64(r.DurationMs)/1000.0)
		}

		timeStr := r.StartTime.UTC().Format("2006-01-02 15:04:05 UTC")

		cmdStr := r.SanitizedCommand
		if len(cmdStr) > 40 {
			cmdStr = cmdStr[:37] + "..."
		}

		fmt.Fprintf(stdout, "%-22s %-10s %-10s %-24s %s\n", r.ID, statusStr, durationStr, timeStr, cmdStr)
	}

	return ExitSuccess
}

func runTerminalInspect(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "Error: Record ID required.\nUsage: %s terminal inspect <record-id>\n", AppName)
		return ExitUsageError
	}
	recordID := args[0]

	db, _, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rec, err := db.GetTerminalRecord(ctx, recordID)
	if err != nil {
		fmt.Fprintf(stderr, "Error: Terminal record not found: %s\n", recordID)
		return ExitGeneralErr
	}

	statusDesc := "Terminated by signal or interrupted (no exit code)"
	if rec.ExitCode != nil {
		if *rec.ExitCode == 0 {
			statusDesc = "0 (Success)"
		} else {
			statusDesc = fmt.Sprintf("%d (Error)", *rec.ExitCode)
		}
	}

	privacyStatus := "Clean (no secrets detected)"
	if rec.IsOmitted {
		privacyStatus = fmt.Sprintf("Omitted (%s)", rec.OmissionReason)
	} else if rec.IsRedacted {
		privacyStatus = "Redacted (sensitive arguments or tokens masked)"
	}

	fmt.Fprintf(stdout, "LIFELOG Terminal Record: %s\n", rec.ID)
	fmt.Fprintf(stdout, "%s\n", strings.Repeat("=", 50))
	fmt.Fprintf(stdout, "Record ID:        %s\n", rec.ID)
	fmt.Fprintf(stdout, "Working Dir:      %s\n", rec.WorkingDir)
	fmt.Fprintf(stdout, "Command:          %s\n", rec.SanitizedCommand)
	fmt.Fprintf(stdout, "Executable:       %s\n", rec.Executable)
	fmt.Fprintf(stdout, "Collection Mode:  %s\n", rec.CollectionMode)
	fmt.Fprintf(stdout, "Exit Status:      %s\n", statusDesc)
	fmt.Fprintf(stdout, "Duration:         %dms\n", rec.DurationMs)
	fmt.Fprintf(stdout, "Start Time:       %s\n", rec.StartTime.UTC().Format("2006-01-02 15:04:05 UTC"))
	fmt.Fprintf(stdout, "End Time:         %s\n", rec.EndTime.UTC().Format("2006-01-02 15:04:05 UTC"))
	fmt.Fprintf(stdout, "Privacy Status:   %s\n", privacyStatus)
	fmt.Fprintf(stdout, "Recorded At:      %s\n", rec.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"))

	return ExitSuccess
}

func runTerminalDelete(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "Error: Record ID required.\nUsage: %s terminal delete <record-id>\n", AppName)
		return ExitUsageError
	}
	recordID := args[0]

	db, _, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	deleted, err := db.DeleteTerminalRecord(ctx, recordID)
	if err != nil {
		fmt.Fprintf(stderr, "Error deleting terminal record: %v\n", err)
		return ExitGeneralErr
	}
	if !deleted {
		fmt.Fprintf(stderr, "Terminal record not found: %s\n", recordID)
		return ExitGeneralErr
	}

	fmt.Fprintf(stdout, "Deleted terminal record: %s\n", recordID)
	return ExitSuccess
}

func runTerminalClear(args []string, stdout, stderr io.Writer) int {
	var confirmed bool
	for _, a := range args {
		if a == "--confirm" || a == "--force" {
			confirmed = true
			break
		}
	}

	if !confirmed {
		fmt.Fprintf(stderr, "Error: Clearing all terminal history is permanent.\nRun '%s terminal clear --confirm' or '--force' to proceed.\n", AppName)
		return ExitUsageError
	}

	db, _, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := db.ClearTerminalRecords(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "Error clearing terminal records: %v\n", err)
		return ExitGeneralErr
	}

	fmt.Fprintf(stdout, "Successfully cleared %d recorded terminal execution(s).\n", count)
	return ExitSuccess
}
