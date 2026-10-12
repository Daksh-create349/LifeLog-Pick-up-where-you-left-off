package terminal

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecuteSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	var stdout, stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rec, code, err := Execute(ctx, tmpDir, []string{"echo", "hello", "world"}, false, &stdout, &stderr, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if rec == nil {
		t.Fatalf("expected non-nil record")
	}
	if rec.ExitCode == nil || *rec.ExitCode != 0 {
		t.Fatalf("expected record ExitCode=0, got %v", rec.ExitCode)
	}
	if !strings.Contains(stdout.String(), "hello world") {
		t.Fatalf("expected stdout 'hello world', got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "[lifelog] Recording command:") {
		t.Fatalf("expected stderr explanation, got %q", stderr.String())
	}
	if rec.CollectionMode != "direct" {
		t.Fatalf("expected collection mode 'direct', got %q", rec.CollectionMode)
	}
	if rec.DurationMs < 0 {
		t.Fatalf("expected non-negative duration, got %d", rec.DurationMs)
	}
}

func TestExecuteNonZeroExit(t *testing.T) {
	tmpDir := t.TempDir()
	var stdout, stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rec, code, err := Execute(ctx, tmpDir, []string{"sh", "-c", "exit 42"}, false, &stdout, &stderr, nil)
	if err != nil {
		t.Fatalf("unexpected execution launch error: %v", err)
	}
	if code != 42 {
		t.Fatalf("expected exit code 42, got %d", code)
	}
	if rec.ExitCode == nil || *rec.ExitCode != 42 {
		t.Fatalf("expected record ExitCode=42, got %v", rec.ExitCode)
	}
}

func TestExecuteMissingExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	var stdout, stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rec, code, err := Execute(ctx, tmpDir, []string{"nonexistent_binary_xyz_123"}, false, &stdout, &stderr, nil)
	if err == nil {
		t.Fatalf("expected error for non-existent executable")
	}
	if code != 127 && code != 1 {
		t.Fatalf("expected failure exit code 127 or 1, got %d", code)
	}
	if rec == nil {
		t.Fatalf("expected record even on launch failure")
	}
	if rec.ExitCode != nil {
		t.Fatalf("expected ExitCode=nil for failed launch, got %v", rec.ExitCode)
	}
}

func TestExecuteUnicodeAndSpaces(t *testing.T) {
	tmpDir := t.TempDir()
	var stdout, stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rec, code, err := Execute(ctx, tmpDir, []string{"echo", "Hello 世界 🚀", "path with spaces"}, false, &stdout, &stderr, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Hello 世界 🚀") {
		t.Fatalf("expected unicode in stdout, got %q", stdout.String())
	}
	if !strings.Contains(rec.SanitizedCommand, "Hello 世界 🚀") {
		t.Fatalf("expected unicode in sanitized command, got %q", rec.SanitizedCommand)
	}
}

func TestExecuteWorkingDirCapture(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub project")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("creating subdir: %v", err)
	}

	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rec, code, err := Execute(ctx, subDir, []string{"pwd"}, false, &stdout, &stderr, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	evalSubDir, _ := filepath.EvalSymlinks(subDir)
	if rec.WorkingDir != evalSubDir {
		t.Fatalf("expected working dir %q, got %q", evalSubDir, rec.WorkingDir)
	}
}

func TestExecuteSensitiveCommandDetailsOmitted(t *testing.T) {
	tmpDir := t.TempDir()
	var stdout, stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use pass or security (they may not exist on system, but sanitize runs before exec)
	rec, _, _ := Execute(ctx, tmpDir, []string{"pass", "show", "master/password"}, false, &stdout, &stderr, nil)
	if rec == nil {
		t.Fatalf("expected record")
	}
	if !rec.IsOmitted {
		t.Fatalf("expected IsOmitted=true")
	}
	if strings.Contains(rec.SanitizedCommand, "master/password") {
		t.Fatalf("sensitive argument leaked in SanitizedCommand: %s", rec.SanitizedCommand)
	}
	if len(rec.SanitizedArgs) != 0 {
		t.Fatalf("expected empty SanitizedArgs for omitted tool, got %v", rec.SanitizedArgs)
	}
}

func TestExecuteContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	var stdout, stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	rec, code, _ := Execute(ctx, tmpDir, []string{"sleep", "5"}, false, &stdout, &stderr, nil)
	if rec == nil {
		t.Fatalf("expected record for cancelled process")
	}
	if code == 0 {
		t.Fatalf("expected non-zero code for cancelled process, got %d", code)
	}
}
