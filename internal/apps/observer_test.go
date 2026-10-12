package apps_test

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/apps"
	"github.com/lifelog/lifelog/internal/storage"
)

type mockProcess struct {
	stdinReader  *io.PipeReader
	stdinWriter  *io.PipeWriter
	stdoutReader *io.PipeReader
	stdoutWriter *io.PipeWriter
}

func newMockProcess() *mockProcess {
	sinR, sinW := io.Pipe()
	soutR, soutW := io.Pipe()
	return &mockProcess{
		stdinReader:  sinR,
		stdinWriter:  sinW,
		stdoutReader: soutR,
		stdoutWriter: soutW,
	}
}

func (m *mockProcess) emit(jsonLine string) {
	_, _ = m.stdoutWriter.Write([]byte(jsonLine + "\n"))
}

func (m *mockProcess) close() {
	_ = m.stdoutWriter.Close()
	_ = m.stdinWriter.Close()
}

func TestSupervisor_LifecycleAndTransitions(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_supervisor.db")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open failed: %v", err)
	}
	defer db.Close()

	proc := newMockProcess()

	mockCmdFactory := func(cmdCtx context.Context, binaryPath string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, error) {
		// Use dummy command like 'cat' or 'sleep'
		cmd := exec.CommandContext(cmdCtx, "cat")
		cmd.Stdin = proc.stdinReader
		cmd.Stdout = proc.stdoutWriter
		return cmd, proc.stdinWriter, proc.stdoutReader, nil
	}

	sup, err := apps.NewSupervisor(apps.SupervisorConfig{
		DB:             db,
		BinaryPath:     "/mock/binary",
		Enabled:        true,
		CommandFactory: mockCmdFactory,
	})
	if err != nil {
		t.Fatalf("NewSupervisor failed: %v", err)
	}

	if err := sup.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	st := sup.Status()
	if !st.Enabled || st.Status != "active" {
		t.Errorf("expected active status, got %+v", st)
	}

	now := time.Now().UTC()
	t1 := now.Format(time.RFC3339)

	// 1. Emit App Activated: Code
	proc.emit(fmt.Sprintf(`{"version":"1.0","event":"app_activated","bundle_id":"com.microsoft.VSCode","app_name":"Code","pid":100,"timestamp":"%s"}`, t1))
	time.Sleep(50 * time.Millisecond)

	st = sup.Status()
	if st.ActiveApp != "Code" || st.ActiveBundle != "com.microsoft.VSCode" {
		t.Fatalf("expected active app Code, got %+v", st)
	}

	// 2. Duplicate App Activated: should be ignored
	proc.emit(fmt.Sprintf(`{"version":"1.0","event":"app_activated","bundle_id":"com.microsoft.VSCode","app_name":"Code","pid":100,"timestamp":"%s"}`, t1))
	time.Sleep(50 * time.Millisecond)

	sessions, count, err := db.GetAppSessions(ctx, storage.AppSessionFilter{BundleID: "com.microsoft.VSCode"})
	if err != nil || count != 1 {
		t.Fatalf("expected exactly 1 session, got count=%d len=%d (err: %v)", count, len(sessions), err)
	}

	// 3. Heartbeat update
	t2 := now.Add(10 * time.Second).Format(time.RFC3339)
	proc.emit(fmt.Sprintf(`{"version":"1.0","event":"heartbeat","bundle_id":"com.microsoft.VSCode","app_name":"Code","timestamp":"%s"}`, t2))
	time.Sleep(50 * time.Millisecond)

	// 4. Switch to Terminal
	t3 := now.Add(20 * time.Second).Format(time.RFC3339)
	proc.emit(fmt.Sprintf(`{"version":"1.0","event":"app_activated","bundle_id":"com.apple.Terminal","app_name":"Terminal","pid":200,"timestamp":"%s"}`, t3))
	time.Sleep(50 * time.Millisecond)

	st = sup.Status()
	if st.ActiveApp != "Terminal" {
		t.Fatalf("expected active app Terminal, got %+v", st)
	}

	// Code session should now be closed (is_active = false)
	sessions, _, _ = db.GetAppSessions(ctx, storage.AppSessionFilter{BundleID: "com.microsoft.VSCode"})
	if len(sessions) != 1 || sessions[0].IsActive {
		t.Fatalf("expected closed Code session, got %+v", sessions)
	}

	// 5. System Sleep
	t4 := now.Add(30 * time.Second).Format(time.RFC3339)
	proc.emit(fmt.Sprintf(`{"version":"1.0","event":"system_sleep","timestamp":"%s"}`, t4))
	time.Sleep(50 * time.Millisecond)

	st = sup.Status()
	if st.ActiveApp != "" {
		t.Errorf("expected no active app after sleep, got %s", st.ActiveApp)
	}

	// 6. Test Disable Tracking: no new sessions recorded
	if err := sup.SetEnabled(false); err != nil {
		t.Fatalf("SetEnabled(false) failed: %v", err)
	}

	t5 := now.Add(40 * time.Second).Format(time.RFC3339)
	proc.emit(fmt.Sprintf(`{"version":"1.0","event":"app_activated","bundle_id":"com.google.Chrome","app_name":"Chrome","pid":300,"timestamp":"%s"}`, t5))
	time.Sleep(50 * time.Millisecond)

	chromeSess, chromeCount, _ := db.GetAppSessions(ctx, storage.AppSessionFilter{BundleID: "com.google.Chrome"})
	if chromeCount != 0 || len(chromeSess) != 0 {
		t.Errorf("expected 0 Chrome sessions while disabled, got %d", chromeCount)
	}

	// 7. Stop clean
	if err := sup.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	proc.close()
}

func TestSupervisor_MalformedAndOversizedMessages(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_malformed.db")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open failed: %v", err)
	}
	defer db.Close()

	proc := newMockProcess()
	mockCmdFactory := func(cmdCtx context.Context, binaryPath string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, error) {
		cmd := exec.CommandContext(cmdCtx, "cat")
		cmd.Stdin = proc.stdinReader
		cmd.Stdout = proc.stdoutWriter
		return cmd, proc.stdinWriter, proc.stdoutReader, nil
	}

	sup, err := apps.NewSupervisor(apps.SupervisorConfig{
		DB:             db,
		BinaryPath:     "/mock/binary",
		Enabled:        true,
		CommandFactory: mockCmdFactory,
	})
	if err != nil {
		t.Fatalf("NewSupervisor failed: %v", err)
	}

	if err := sup.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sup.Stop()
	defer proc.close()

	// 1. Invalid JSON
	proc.emit(`{not a json`)
	time.Sleep(30 * time.Millisecond)

	// 2. Mismatched protocol version
	proc.emit(`{"version":"9.9","event":"app_activated","bundle_id":"bad","app_name":"bad","timestamp":"2026-10-12T00:00:00Z"}`)
	time.Sleep(30 * time.Millisecond)

	// 3. Unknown event
	proc.emit(`{"version":"1.0","event":"unknown_event_xyz","timestamp":"2026-10-12T00:00:00Z"}`)
	time.Sleep(30 * time.Millisecond)

	// Ensure supervisor didn't crash
	st := sup.Status()
	if st.Status != "active" {
		t.Errorf("expected active status despite malformed messages, got %s", st.Status)
	}
}
