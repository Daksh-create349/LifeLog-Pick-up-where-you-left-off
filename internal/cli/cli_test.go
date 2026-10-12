package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/cli"
	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/paths"
	"github.com/lifelog/lifelog/internal/storage"
)

func TestVersionString(t *testing.T) {
	v := cli.VersionString()
	if !strings.Contains(v, cli.AppName) {
		t.Errorf("VersionString() = %q, want containing AppName %q", v, cli.AppName)
	}
	if !strings.Contains(v, cli.AppVersion) {
		t.Errorf("VersionString() = %q, want containing AppVersion %q", v, cli.AppVersion)
	}
	if !strings.Contains(v, runtime.GOOS) {
		t.Errorf("VersionString() = %q, want containing GOOS %q", v, runtime.GOOS)
	}
	if !strings.Contains(v, runtime.GOARCH) {
		t.Errorf("VersionString() = %q, want containing GOARCH %q", v, runtime.GOARCH)
	}
}

func TestRun_DefaultOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{}, &stdout, &stderr)

	if code != cli.ExitSuccess {
		t.Fatalf("expected exit code %d, got %d", cli.ExitSuccess, code)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr, got: %s", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, cli.AppName) || !strings.Contains(out, cli.AppVersion) {
		t.Errorf("stdout missing app metadata: %s", out)
	}
	if !strings.Contains(out, "--help") {
		t.Errorf("stdout missing help hint: %s", out)
	}
}

func TestRun_Help(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "flag-long", args: []string{"--help"}},
		{name: "flag-short", args: []string{"-h"}},
		{name: "command", args: []string{"help"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := cli.Run(tc.args, &stdout, &stderr)

			if code != cli.ExitSuccess {
				t.Fatalf("expected exit code %d, got %d", cli.ExitSuccess, code)
			}
			if stderr.Len() != 0 {
				t.Errorf("expected empty stderr, got: %s", stderr.String())
			}
			out := stdout.String()
			if !strings.Contains(out, "Usage:") {
				t.Errorf("stdout missing 'Usage:': %s", out)
			}
			if !strings.Contains(out, "git") {
				t.Errorf("stdout missing 'git' in help: %s", out)
			}
			if !strings.Contains(out, "repos") {
				t.Errorf("stdout missing 'repos' in help: %s", out)
			}
			if !strings.Contains(out, "watch") {
				t.Errorf("stdout missing 'watch' in help: %s", out)
			}
			if !strings.Contains(out, "events") {
				t.Errorf("stdout missing 'events' in help: %s", out)
			}
			if !strings.Contains(out, "init") {
				t.Errorf("stdout missing 'init' in help: %s", out)
			}
			if !strings.Contains(out, "status") {
				t.Errorf("stdout missing 'status' in help: %s", out)
			}
			if !strings.Contains(out, "doctor") {
				t.Errorf("stdout missing 'doctor' in help: %s", out)
			}
			if !strings.Contains(out, "--version") {
				t.Errorf("stdout missing '--version': %s", out)
			}
		})
	}
}

func TestRun_Version(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "flag-long", args: []string{"--version"}},
		{name: "flag-short", args: []string{"-v"}},
		{name: "command", args: []string{"version"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := cli.Run(tc.args, &stdout, &stderr)

			if code != cli.ExitSuccess {
				t.Fatalf("expected exit code %d, got %d", cli.ExitSuccess, code)
			}
			if stderr.Len() != 0 {
				t.Errorf("expected empty stderr, got: %s", stderr.String())
			}
			out := strings.TrimSpace(stdout.String())
			expected := cli.VersionString()
			if out != expected {
				t.Errorf("got %q, want %q", out, expected)
			}
		})
	}
}

func TestRun_InvalidFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"--invalid-flag"}, &stdout, &stderr)

	if code != cli.ExitUsageError {
		t.Fatalf("expected exit code %d, got %d", cli.ExitUsageError, code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "flag provided but not defined") {
		t.Errorf("stderr missing undefined flag notice: %s", errStr)
	}
	if !strings.Contains(errStr, "--help") {
		t.Errorf("stderr missing '--help' hint: %s", errStr)
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"nonexistent-command"}, &stdout, &stderr)

	if code != cli.ExitUsageError {
		t.Fatalf("expected exit code %d, got %d", cli.ExitUsageError, code)
	}
	errStr := stderr.String()
	if !strings.Contains(errStr, "unknown command: nonexistent-command") {
		t.Errorf("stderr missing unknown command notice: %s", errStr)
	}
}

func TestRun_Status(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv(paths.EnvDataDir, tempDir)

	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"status"}, &stdout, &stderr)

	if code != cli.ExitSuccess {
		t.Fatalf("expected exit code %d, got %d (stderr: %s)", cli.ExitSuccess, code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "LIFELOG Status") {
		t.Errorf("output missing title: %s", out)
	}
	if !strings.Contains(out, tempDir) {
		t.Errorf("output missing data directory: %s", out)
	}

	// Now initialize the database and run status again to test healthy branch
	dbPath := filepath.Join(tempDir, paths.DatabaseFile)
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	db.Close()

	stdout.Reset()
	code = cli.Run([]string{"status"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected exit code %d, got %d", cli.ExitSuccess, code)
	}
	out = stdout.String()
	if !strings.Contains(out, "Healthy") {
		t.Errorf("expected 'Healthy' in status output: %s", out)
	}
}

func TestRun_Doctor(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv(paths.EnvDataDir, tempDir)

	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"doctor"}, &stdout, &stderr)

	if code != cli.ExitSuccess {
		t.Fatalf("expected doctor exit code %d, got %d", cli.ExitSuccess, code)
	}
	out := stdout.String()
	if !strings.Contains(out, "LIFELOG Doctor Diagnostics") {
		t.Errorf("missing title in doctor output: %s", out)
	}
	if !strings.Contains(out, "Platform Compatibility") {
		t.Errorf("missing platform check in doctor output: %s", out)
	}
	if !strings.Contains(out, "0 failure(s)") {
		t.Errorf("expected 0 failures in clean run: %s", out)
	}

	// Test Doctor failure detection: write malformed config file
	configPath := filepath.Join(tempDir, paths.ConfigFile)
	if err := paths.EnsureDir(tempDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("invalid-json"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	code = cli.Run([]string{"doctor"}, &stdout, &stderr)
	if code != cli.ExitGeneralErr {
		t.Fatalf("expected doctor to fail with code %d on corrupted config, got %d", cli.ExitGeneralErr, code)
	}
	out = stdout.String()
	if !strings.Contains(out, "[FAIL]") {
		t.Errorf("expected [FAIL] in doctor output for bad config: %s", out)
	}
}

func TestRun_Init(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv(paths.EnvDataDir, tempDir)

	var stdout, stderr bytes.Buffer
	// 1. Initial run: should initialize storage
	code := cli.Run([]string{"init"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected init exit code %d, got %d (stderr: %s)", cli.ExitSuccess, code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "initialized successfully") {
		t.Errorf("output missing success message: %s", out)
	}

	// 2. Second run: idempotent check
	stdout.Reset()
	code = cli.Run([]string{"init"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected idempotent init exit code %d, got %d", cli.ExitSuccess, code)
	}
	out = stdout.String()
	if !strings.Contains(out, "already initialized") {
		t.Errorf("output missing already initialized message: %s", out)
	}
}

func TestRun_Watch_Validation(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv(paths.EnvDataDir, tempDir)

	var stdout, stderr bytes.Buffer

	// 1. Missing directory argument
	code := cli.Run([]string{"watch"}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected usage error when watching without path, got %d", code)
	}
	if !strings.Contains(stderr.String(), "explicit directory path") {
		t.Errorf("expected explicit path notice, got: %s", stderr.String())
	}

	// 2. Invalid interval
	stderr.Reset()
	code = cli.Run([]string{"watch", "--interval", "invalid", tempDir}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected usage error on invalid interval, got %d", code)
	}

	// 3. Watching restricted directory (home directory)
	home, err := os.UserHomeDir()
	if err == nil {
		stderr.Reset()
		code = cli.Run([]string{"watch", home}, &stdout, &stderr)
		if code != cli.ExitUsageError {
			t.Fatalf("expected error watching home directory, got %d", code)
		}
	}
}

func TestRun_Events(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv(paths.EnvDataDir, tempDir)

	var stdout, stderr bytes.Buffer

	// 1. Query on empty database
	code := cli.Run([]string{"events"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected exit code %d on empty events, got %d", cli.ExitSuccess, code)
	}
	if !strings.Contains(stdout.String(), "No filesystem events recorded yet") {
		t.Errorf("expected empty message, got: %s", stdout.String())
	}

	// 2. Invalid limits
	stderr.Reset()
	code = cli.Run([]string{"events", "--limit", "0"}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Errorf("expected error on 0 limit, got %d", code)
	}

	stderr.Reset()
	code = cli.Run([]string{"events", "--limit", "9999"}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Errorf("expected error on 9999 limit, got %d", code)
	}

	// 3. Populate database and query
	dbPath := filepath.Join(tempDir, paths.DatabaseFile)
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC()
	err = db.RecordEvents(context.Background(), []events.Event{
		{
			ID:        "evt_cli_1",
			Type:      events.EventCreated,
			Path:      "/test/project/file1.go",
			Size:      256,
			ModTime:   now,
			IsDir:     false,
			Source:    "fs_poll",
			CreatedAt: now,
		},
		{
			ID:        "evt_cli_2",
			Type:      events.EventModified,
			Path:      "/test/project/sub/file2.go",
			Size:      512,
			ModTime:   now.Add(time.Second),
			IsDir:     false,
			Source:    "fs_poll",
			CreatedAt: now.Add(time.Second),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	code = cli.Run([]string{"events", "--limit", "10"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected success querying events, got %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "file1.go") || !strings.Contains(out, "file2.go") {
		t.Errorf("missing events in output: %s", out)
	}
	if !strings.Contains(out, "showing 2") {
		t.Errorf("expected 'showing 2' in header, got: %s", out)
	}

	// 4. Path prefix filter
	stdout.Reset()
	code = cli.Run([]string{"events", "--path", "/test/project/sub"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected success filtering events, got %d", code)
	}
	out = stdout.String()
	if !strings.Contains(out, "file2.go") || strings.Contains(out, "file1.go") {
		t.Errorf("path filtering failed: %s", out)
	}
}

func TestRun_Git_Commands(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "lifelog_data")
	projectDir := filepath.Join(tempDir, "git_project")

	t.Setenv(paths.EnvDataDir, dataDir)

	var stdout, stderr bytes.Buffer

	// 1. git help
	code := cli.Run([]string{"git", "help"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected git help code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "LIFELOG Git Memory Engine") {
		t.Errorf("missing git help header: %s", stdout.String())
	}

	// 2. git unknown subcommand
	stderr.Reset()
	code = cli.Run([]string{"git", "unknown_subcmd"}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected exit code 2 on unknown git subcommand, got %d", code)
	}

	// 3. repos on empty database
	stdout.Reset()
	code = cli.Run([]string{"repos"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected repos code 0 on empty DB, got %d", code)
	}
	if !strings.Contains(stdout.String(), "No Git repositories registered yet") {
		t.Errorf("expected empty repos notice, got: %s", stdout.String())
	}

	// 4. git scan on non-git directory
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	code = cli.Run([]string{"git", "scan", projectDir}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected error scanning non-git directory, got %d", code)
	}

	// 5. Initialize git repo and scan
	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", projectDir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.name", "Tester")
	runGit("config", "user.email", "tester@test.local")

	f := filepath.Join(projectDir, "test.txt")
	if err := os.WriteFile(f, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "test.txt")
	runGit("commit", "-m", "first commit")

	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"git", "scan", projectDir}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected scan success, got code %d (stderr: %s)", code, stderr.String())
	}
	scanOut := stdout.String()
	if !strings.Contains(scanOut, "LIFELOG Git Scan") {
		t.Errorf("missing scan header: %s", scanOut)
	}
	if !strings.Contains(scanOut, "New Commits:    1") {
		t.Errorf("expected 1 new commit, got: %s", scanOut)
	}

	// 6. repos lists registered repository
	stdout.Reset()
	code = cli.Run([]string{"repos"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected repos code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "showing 1") {
		t.Errorf("expected 1 repo listed, got: %s", stdout.String())
	}

	// 7. git status
	stdout.Reset()
	code = cli.Run([]string{"git", "status", projectDir}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected git status code 0, got %d", code)
	}
	statusOut := stdout.String()
	if !strings.Contains(statusOut, "first commit") {
		t.Errorf("expected commit subject in status output: %s", statusOut)
	}

	// 8. git log
	stdout.Reset()
	code = cli.Run([]string{"git", "log", projectDir, "--limit", "10"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected git log code 0, got %d", code)
	}
	logOut := stdout.String()
	if !strings.Contains(logOut, "first commit") || !strings.Contains(logOut, "Tester") {
		t.Errorf("expected commit in git log output: %s", logOut)
	}

	// 9. git branches
	stdout.Reset()
	code = cli.Run([]string{"git", "branches", projectDir}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected git branches code 0, got %d", code)
	}
	branchesOut := stdout.String()
	if !strings.Contains(branchesOut, "LIFELOG Git Branches") {
		t.Errorf("missing branches output: %s", branchesOut)
	}
}

func TestRun_TerminalLifecycle(t *testing.T) {
	tempDataDir := t.TempDir()
	t.Setenv("LIFELOG_DATA_DIR", tempDataDir)

	var stdout, stderr bytes.Buffer

	// 1. Initialize storage
	code := cli.Run([]string{"init"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected init code 0, got %d", code)
	}

	// 2. Record a successful command
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "record", "--", "echo", "lifelog terminal test"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected terminal record code 0, got %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "lifelog terminal test") {
		t.Errorf("expected command stdout, got: %s", stdout.String())
	}

	// 3. Record a non-zero exit command
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "record", "--", "sh", "-c", "exit 42"}, &stdout, &stderr)
	if code != 42 {
		t.Fatalf("expected terminal record return child exit code 42, got %d", code)
	}

	// 4. Record a command with sensitive flags and verify redaction
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "record", "--", "echo", "--password=supersecret", "safe_arg"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected terminal record code 0, got %d", code)
	}

	// 5. Query terminal history
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "history"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected terminal history code 0, got %d", code)
	}
	historyOut := stdout.String()
	if !strings.Contains(historyOut, "showing 3") {
		t.Errorf("expected 3 records in history, got: %s", historyOut)
	}
	if !strings.Contains(historyOut, "ERR (42)") {
		t.Errorf("expected ERR (42) in history output, got: %s", historyOut)
	}
	if strings.Contains(historyOut, "supersecret") {
		t.Errorf("secret leaked in history output: %s", historyOut)
	}
	if !strings.Contains(historyOut, "--password=[REDACTED]") {
		t.Errorf("expected redacted flag in history output: %s", historyOut)
	}

	// Extract a record ID from history output
	lines := strings.Split(historyOut, "\n")
	var recordID string
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.HasPrefix(fields[0], "term_") {
			recordID = fields[0]
			break
		}
	}
	if recordID == "" {
		t.Fatalf("could not extract record ID from history output: %s", historyOut)
	}

	// 6. Inspect record
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "inspect", recordID}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected inspect code 0, got %d (stderr: %s)", code, stderr.String())
	}
	inspectOut := stdout.String()
	if !strings.Contains(inspectOut, recordID) {
		t.Errorf("inspect output missing record ID: %s", inspectOut)
	}
	if strings.Contains(inspectOut, "supersecret") {
		t.Errorf("secret leaked in inspect output: %s", inspectOut)
	}

	// 7. Delete record
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "delete", recordID}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected delete code 0, got %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Deleted terminal record") {
		t.Errorf("expected delete confirmation: %s", stdout.String())
	}

	// 8. Clear without --confirm should fail
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "clear"}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected clear without --confirm to return code %d, got %d", cli.ExitUsageError, code)
	}

	// 9. Clear with --confirm should succeed
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "clear", "--confirm"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected clear --confirm code 0, got %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Successfully cleared") {
		t.Errorf("expected clear confirmation, got: %s", stdout.String())
	}

	// 10. History should now be empty
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "history"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected history code 0 on empty, got %d", code)
	}
	if !strings.Contains(stdout.String(), "No recorded terminal commands found") {
		t.Errorf("expected empty state message, got: %s", stdout.String())
	}
}

func TestRun_SearchAndTimeline(t *testing.T) {
	tempDataDir := t.TempDir()
	t.Setenv("LIFELOG_DATA_DIR", tempDataDir)

	var stdout, stderr bytes.Buffer

	// 1. Initialize storage
	code := cli.Run([]string{"init"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("init failed: %d", code)
	}

	// 2. Record terminal command
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "record", "--", "echo", "build-auth-module"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("record failed: %d", code)
	}

	// 3. Search across all domains
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"search", "build-auth-module"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("search failed: %d (stderr: %s)", code, stderr.String())
	}
	searchOut := stdout.String()
	if !strings.Contains(searchOut, "LIFELOG Search Results") {
		t.Errorf("missing search header: %s", searchOut)
	}
	if !strings.Contains(searchOut, "build-auth-module") {
		t.Errorf("missing match in search output: %s", searchOut)
	}

	// 4. Search with domain filter
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"search", "--domain", "terminal", "build-auth-module"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("search domain terminal failed: %d", code)
	}
	if !strings.Contains(stdout.String(), "terminal") {
		t.Errorf("expected terminal domain output: %s", stdout.String())
	}

	// 5. Search with empty result domain
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"search", "--domain", "filesystem", "build-auth-module"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("search domain filesystem failed: %d", code)
	}
	if !strings.Contains(stdout.String(), "No matching records found") {
		t.Errorf("expected empty state message, got: %s", stdout.String())
	}

	// 6. Search with invalid domain
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"search", "--domain", "invalid_domain", "query"}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected ExitUsageError for invalid domain, got: %d", code)
	}

	// 7. Timeline command
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"timeline", "--limit", "10"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("timeline failed: %d (stderr: %s)", code, stderr.String())
	}
	timelineOut := stdout.String()
	if !strings.Contains(timelineOut, "LIFELOG Unified Activity Timeline") {
		t.Errorf("missing timeline header: %s", timelineOut)
	}
	if !strings.Contains(timelineOut, "build-auth-module") {
		t.Errorf("missing entry in timeline output: %s", timelineOut)
	}
}

func TestRun_Resume(t *testing.T) {
	tempDataDir := t.TempDir()
	t.Setenv(paths.EnvDataDir, tempDataDir)

	var stdout, stderr bytes.Buffer

	// 1. Initialize
	code := cli.Run([]string{"init"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("init failed: %d (stderr: %s)", code, stderr.String())
	}

	projectDir := t.TempDir()

	// 2. Resume on empty directory (should succeed with informative empty state)
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"resume", projectDir}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("resume failed on empty project: %d (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "LIFELOG PROJECT RESUMPTION REPORT") {
		t.Errorf("missing header in resume output: %s", out)
	}
	if !strings.Contains(out, "No recorded activity") {
		t.Errorf("expected no recorded activity notice in output: %s", out)
	}

	// 3. Record a terminal command in the project directory
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "record", "--", "echo", "resumption-test-command"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("terminal record failed: %d (stderr: %s)", code, stderr.String())
	}

	// 4. Resume after recording activity
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"resume", projectDir, "--limit", "10", "--gap", "15m", "--window", "24h"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("resume failed: %d (stderr: %s)", code, stderr.String())
	}
	out = stdout.String()
	if !strings.Contains(out, "resumption-test-command") {
		t.Errorf("expected command in resume output: %s", out)
	}
	if !strings.Contains(out, "session-001") {
		t.Errorf("expected synthesized session in resume output: %s", out)
	}
	if !strings.Contains(out, "1. OBSERVED ACTIVITY") {
		t.Errorf("expected Observed section: %s", out)
	}
	if !strings.Contains(out, "2. DERIVED ACTIVITY SESSIONS") {
		t.Errorf("expected Derived section: %s", out)
	}
	if !strings.Contains(out, "3. UNAVAILABLE CONTEXT & PRIVACY BOUNDARIES") {
		t.Errorf("expected Unavailable section: %s", out)
	}

	// 5. Test validation errors
	// Invalid limit
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"resume", projectDir, "--limit", "0"}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected ExitUsageError for limit 0, got: %d", code)
	}

	// Invalid gap
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"resume", projectDir, "--gap", "notaduration"}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected ExitUsageError for bad gap, got: %d", code)
	}

	// Too many arguments
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"resume", projectDir, "extra_arg"}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected ExitUsageError for extra args, got: %d", code)
	}
}

func TestRun_Export(t *testing.T) {
	tempDataDir := t.TempDir()
	t.Setenv(paths.EnvDataDir, tempDataDir)

	var stdout, stderr bytes.Buffer

	// 1. Initialize
	code := cli.Run([]string{"init"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("init failed: %d (stderr: %s)", code, stderr.String())
	}

	projectDir := t.TempDir()

	// 2. Export Markdown to stdout
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"export", projectDir, "--format", "markdown"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("export markdown failed: %d (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "# LIFELOG Project Resumption Report") {
		t.Errorf("missing title in export markdown output: %s", out)
	}
	if !strings.Contains(out, "## 1. Observed Activity") {
		t.Errorf("missing Observed section in export markdown output: %s", out)
	}

	// 3. Export JSON to stdout
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"export", projectDir, "--format", "json"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("export json failed: %d (stderr: %s)", code, stderr.String())
	}
	var env map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshaling export json output failed: %v", err)
	}
	if env["generator"] != "LIFELOG" {
		t.Errorf("got generator %v, want LIFELOG", env["generator"])
	}

	// 4. Export to file
	outFile := filepath.Join(projectDir, "export-test.md")
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"export", projectDir, "--output", outFile}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("export to file failed: %d (stderr: %s)", code, stderr.String())
	}
	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("expected export file to exist: %v", err)
	}

	// 5. Export to existing file without force -> should fail
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"export", projectDir, "--output", outFile}, &stdout, &stderr)
	if code != cli.ExitGeneralErr {
		t.Fatalf("expected failure overwriting existing file without force, got: %d", code)
	}

	// 6. Export to existing file with force -> should succeed
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"export", projectDir, "--output", outFile, "--force"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("export with --force failed: %d (stderr: %s)", code, stderr.String())
	}

	// 7. Export to protected dataDir path -> should fail
	protectedDest := filepath.Join(tempDataDir, "protected_export.md")
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"export", projectDir, "--output", protectedDest, "--force"}, &stdout, &stderr)
	if code != cli.ExitGeneralErr {
		t.Fatalf("expected failure exporting into protected dataDir, got: %d", code)
	}

	// 8. Invalid format validation
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"export", projectDir, "--format", "invalid_fmt"}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected ExitUsageError for bad format, got: %d", code)
	}
}

func TestRun_Checkpoint(t *testing.T) {
	tempDataDir := t.TempDir()
	t.Setenv(paths.EnvDataDir, tempDataDir)

	var stdout, stderr bytes.Buffer

	// 1. Initialize
	code := cli.Run([]string{"init"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("init failed: %d (stderr: %s)", code, stderr.String())
	}

	projectDir := t.TempDir()

	// 2. Checkpoint create without label -> usage error
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"checkpoint", "create", projectDir}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected ExitUsageError for checkpoint create without label, got: %d", code)
	}

	// 3. Checkpoint create with label -> success
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"checkpoint", "create", projectDir, "--label", "Sprint 1 checkpoint"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("checkpoint create failed: %d (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Checkpoint created successfully") {
		t.Errorf("missing success message in create output: %s", out)
	}

	// Extract checkpoint ID from output
	var chkID string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "ID:") {
			chkID = strings.TrimSpace(strings.TrimPrefix(line, "ID:"))
			break
		}
	}
	if chkID == "" {
		t.Fatalf("failed to extract checkpoint ID from output: %s", out)
	}

	// 4. Checkpoint list
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"checkpoint", "list", projectDir}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("checkpoint list failed: %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), chkID) {
		t.Errorf("missing checkpoint ID in list output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Sprint 1 checkpoint") {
		t.Errorf("missing label in list output: %s", stdout.String())
	}

	// 5. Checkpoint show (text)
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"checkpoint", "show", chkID}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("checkpoint show failed: %d (stderr: %s)", code, stderr.String())
	}
	showOut := stdout.String()
	if !strings.Contains(showOut, "LIFELOG RESUME CHECKPOINT") {
		t.Errorf("missing checkpoint header in show output: %s", showOut)
	}
	if !strings.Contains(showOut, "Historical snapshot") {
		t.Errorf("missing historical snapshot status: %s", showOut)
	}

	// 6. Checkpoint show (json)
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"checkpoint", "show", chkID, "--format", "json"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("checkpoint show json failed: %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "schema_version") {
		t.Errorf("missing schema_version in json show output: %s", stdout.String())
	}

	// 7. Checkpoint delete without --confirm -> usage error
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"checkpoint", "delete", chkID}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected ExitUsageError for delete without --confirm, got: %d", code)
	}

	// 8. Checkpoint delete with --confirm -> success
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"checkpoint", "delete", chkID, "--confirm"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("checkpoint delete failed: %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "deleted successfully") {
		t.Errorf("missing deleted message: %s", stdout.String())
	}

	// 9. Checkpoint show deleted -> error
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"checkpoint", "show", chkID}, &stdout, &stderr)
	if code != cli.ExitGeneralErr {
		t.Fatalf("expected error showing deleted checkpoint, got: %d", code)
	}
}

func TestRun_Package(t *testing.T) {
	tempDataDir := t.TempDir()
	t.Setenv(paths.EnvDataDir, tempDataDir)

	var stdout, stderr bytes.Buffer

	// 1. Initialize
	code := cli.Run([]string{"init"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("init failed: %d (stderr: %s)", code, stderr.String())
	}

	projectDir := t.TempDir()

	// 2. Package markdown to stdout
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"package", projectDir}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("package markdown failed: %d (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "# LIFELOG Project Context Package") {
		t.Errorf("missing package header: %s", out)
	}
	if !strings.Contains(out, "Data Domain Coverage") {
		t.Errorf("missing domain coverage section: %s", out)
	}

	// 3. Package JSON to stdout
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"package", projectDir, "--format", "json"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("package json failed: %d (stderr: %s)", code, stderr.String())
	}
	var pkg map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &pkg); err != nil {
		t.Fatalf("unmarshaling package json failed: %v", err)
	}
	if pkg["package_version"] != "1.0.0" {
		t.Errorf("got package_version %v, want 1.0.0", pkg["package_version"])
	}
	if pkg["generator"] != "LIFELOG" {
		t.Errorf("got generator %v, want LIFELOG", pkg["generator"])
	}
	if pkg["display_name"] != filepath.Base(projectDir) {
		t.Errorf("got display_name %v, want %s", pkg["display_name"], filepath.Base(projectDir))
	}

	// 4. Package to explicit file
	pkgFile := filepath.Join(projectDir, "context-pkg.md")
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"package", projectDir, "--output", pkgFile}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("package to file failed: %d (stderr: %s)", code, stderr.String())
	}
	info, err := os.Stat(pkgFile)
	if err != nil {
		t.Fatalf("package file missing: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 file permissions, got %04o", info.Mode().Perm())
	}

	// 5. Overwrite protection without --force
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"package", projectDir, "--output", pkgFile}, &stdout, &stderr)
	if code != cli.ExitGeneralErr {
		t.Fatalf("expected ExitGeneralErr when overwriting without --force, got: %d", code)
	}

	// 6. Overwrite with --force
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"package", projectDir, "--output", pkgFile, "--force"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("package with --force failed: %d (stderr: %s)", code, stderr.String())
	}

	// 7. Protected data directory rejection
	protectedDest := filepath.Join(tempDataDir, "forbidden.md")
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"package", projectDir, "--output", protectedDest, "--force"}, &stdout, &stderr)
	if code != cli.ExitGeneralErr {
		t.Fatalf("expected failure exporting package into protected data dir, got: %d", code)
	}

	// 8. Rejection of symlink into protected data directory
	outsideDir := t.TempDir()
	symlinkPath := filepath.Join(outsideDir, "symlink-to-db.md")
	_ = os.Symlink(filepath.Join(tempDataDir, "lifelog.db"), symlinkPath)
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"package", projectDir, "--output", symlinkPath, "--force"}, &stdout, &stderr)
	if code != cli.ExitGeneralErr {
		t.Fatalf("expected failure targeting symlink pointing to database, got: %d", code)
	}

	// 9. Flag validation
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"package", projectDir, "--format", "xml"}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected ExitUsageError for bad format, got: %d", code)
	}

	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"package", projectDir, "--limit", "0"}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected ExitUsageError for limit 0, got: %d", code)
	}
}

func TestRun_Projects(t *testing.T) {
	tempDataDir := t.TempDir()
	t.Setenv(paths.EnvDataDir, tempDataDir)

	var stdout, stderr bytes.Buffer

	// 1. Initialize
	code := cli.Run([]string{"init"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("init failed: %d (stderr: %s)", code, stderr.String())
	}

	// 2. Empty state human output
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"projects"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("projects on empty DB failed: %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No projects currently discovered") {
		t.Errorf("missing empty state message: %s", stdout.String())
	}

	// 3. Empty state JSON output
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"projects", "--json"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("projects --json on empty DB failed: %d (stderr: %s)", code, stderr.String())
	}
	var emptyArr []interface{}
	if err := json.Unmarshal(stdout.Bytes(), &emptyArr); err != nil {
		t.Fatalf("expected valid JSON empty array, got error: %v (output: %s)", err, stdout.String())
	}
	if len(emptyArr) != 0 {
		t.Errorf("expected 0 items in empty JSON array, got %d", len(emptyArr))
	}

	// 4. Create projects with activity
	projADir := filepath.Join(t.TempDir(), "alpha-app")
	projBDir := filepath.Join(t.TempDir(), "beta-service")
	_ = os.MkdirAll(projADir, 0755)
	_ = os.MkdirAll(projBDir, 0755)

	// Create checkpoint in Alpha
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"checkpoint", "create", projADir, "--label", "Initial Alpha checkpoint"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("checkpoint in Alpha failed: %d (stderr: %s)", code, stderr.String())
	}

	// Create checkpoint in Beta
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"checkpoint", "create", projBDir, "--label", "Initial Beta release"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("checkpoint in Beta failed: %d (stderr: %s)", code, stderr.String())
	}

	// 5. Human projects overview
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"projects"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("projects failed: %d (stderr: %s)", code, stderr.String())
	}
	pOut := stdout.String()
	if !strings.Contains(pOut, "alpha-app") || !strings.Contains(pOut, "beta-service") {
		t.Errorf("missing project names in overview output: %s", pOut)
	}
	if !strings.Contains(pOut, "Discovered Projects: 2") {
		t.Errorf("missing project count 2 in overview: %s", pOut)
	}

	// 6. JSON projects overview
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"projects", "--json"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("projects --json failed: %d (stderr: %s)", code, stderr.String())
	}
	var projList []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &projList); err != nil {
		t.Fatalf("unmarshaling projects json failed: %v", err)
	}
	if len(projList) != 2 {
		t.Fatalf("expected 2 projects in json list, got %d", len(projList))
	}

	// 7. Test sorting: --sort name
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"projects", "--sort", "name", "--json"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("projects --sort name failed: %d", code)
	}
	var nameSorted []map[string]interface{}
	_ = json.Unmarshal(stdout.Bytes(), &nameSorted)
	if nameSorted[0]["display_name"] != "alpha-app" || nameSorted[1]["display_name"] != "beta-service" {
		t.Errorf("incorrect name sorting: %v, %v", nameSorted[0]["display_name"], nameSorted[1]["display_name"])
	}

	// 8. Test invalid sort option -> usage error
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"projects", "--sort", "invalid_sort"}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected ExitUsageError for bad sort mode, got: %d", code)
	}

	// 9. Test limit
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"projects", "--limit", "1", "--json"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("projects with limit failed: %d", code)
	}
	var limList []map[string]interface{}
	_ = json.Unmarshal(stdout.Bytes(), &limList)
	if len(limList) != 1 {
		t.Errorf("expected 1 project with limit=1, got %d", len(limList))
	}
}
