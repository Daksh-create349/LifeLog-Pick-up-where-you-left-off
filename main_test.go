package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/cli"
	"github.com/lifelog/lifelog/internal/collector"
	"github.com/lifelog/lifelog/internal/config"
	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/paths"
	"github.com/lifelog/lifelog/internal/storage"
)

func TestMainIntegration(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("LIFELOG_DATA_DIR", tempDir)

	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"--version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), cli.AppName) {
		t.Fatalf("expected %s in output, got %s", cli.AppName, stdout.String())
	}

	stdout.Reset()
	code = cli.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected init 0, got %d", code)
	}

	stdout.Reset()
	code = cli.Run([]string{"status"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected status 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Healthy") {
		t.Fatalf("expected 'Healthy' in status output, got %s", stdout.String())
	}

	stdout.Reset()
	code = cli.Run([]string{"doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected doctor 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "0 failure(s)") {
		t.Fatalf("expected 0 failures in doctor output, got %s", stdout.String())
	}

	stdout.Reset()
	code = cli.Run([]string{"events"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected events 0 on empty DB, got %d", code)
	}
	if !strings.Contains(stdout.String(), "No filesystem events recorded yet") {
		t.Fatalf("expected empty events notice, got %s", stdout.String())
	}
}

func TestMainWatchAndQueryIntegration(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "lifelog_data")
	projectDir := filepath.Join(tempDir, "sample_project")

	t.Setenv("LIFELOG_DATA_DIR", dataDir)

	p, err := paths.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}

	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	eventCount := 0

	poller, err := collector.NewFSPoller(collector.PollerConfig{
		Root:     projectDir,
		Interval: 25 * time.Millisecond,
		DataDir:  p.DataDir,
		OnEvents: func(batch []events.Event) error {
			if err := db.RecordEvents(context.Background(), batch); err != nil {
				return err
			}
			mu.Lock()
			eventCount += len(batch)
			mu.Unlock()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = poller.Start(ctx)
	}()

	time.Sleep(40 * time.Millisecond)

	// Create a file in monitored project
	docPath := filepath.Join(projectDir, "notes.md")
	if err := os.WriteFile(docPath, []byte("# Project Notes\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Wait for event to be captured and persisted
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		c := eventCount
		mu.Unlock()
		if c >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	wg.Wait()

	// Query via CLI
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"events", "--limit", "10"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Fatalf("expected CLI events exit code %d, got %d", cli.ExitSuccess, code)
	}

	out := stdout.String()
	if !strings.Contains(out, "notes.md") {
		t.Fatalf("expected 'notes.md' in events CLI output, got: %s", out)
	}
	if !strings.Contains(out, "CREATED") && !strings.Contains(out, "created") {
		t.Fatalf("expected created event type in CLI output, got: %s", out)
	}
}

func TestMainGitIntegration(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "lifelog_data")
	projectDir := filepath.Join(tempDir, "main_git_project")

	t.Setenv("LIFELOG_DATA_DIR", dataDir)

	var stdout, stderr bytes.Buffer

	// 1. Initialize LIFELOG
	code := cli.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init failed: %d (stderr: %s)", code, stderr.String())
	}

	// 2. Create git repo with multiple commits
	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", projectDir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, string(out))
		}
	}
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatal(err)
	}
	runGit("init")
	runGit("config", "user.name", "Main Tester")
	runGit("config", "user.email", "maintester@lifelog.local")

	f1 := filepath.Join(projectDir, "file1.txt")
	if err := os.WriteFile(f1, []byte("v1"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "file1.txt")
	runGit("commit", "-m", "commit 1: file1")

	f2 := filepath.Join(projectDir, "file2.txt")
	if err := os.WriteFile(f2, []byte("v2"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "file2.txt")
	runGit("commit", "-m", "commit 2: file2")

	// 3. Register and scan via CLI
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"git", "scan", projectDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("git scan failed: %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "New Commits:    2") {
		t.Errorf("expected 2 new commits in scan output: %s", stdout.String())
	}

	// 4. Query commits
	stdout.Reset()
	code = cli.Run([]string{"git", "log", projectDir, "--limit", "10"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("git log failed: %d", code)
	}
	logOut := stdout.String()
	if !strings.Contains(logOut, "commit 2: file2") || !strings.Contains(logOut, "commit 1: file1") {
		t.Errorf("expected commits in log output: %s", logOut)
	}

	// 5. Create a branch, commit, and switch
	runGit("checkout", "-b", "feature-x")
	f3 := filepath.Join(projectDir, "feature.txt")
	if err := os.WriteFile(f3, []byte("feat"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "feature.txt")
	runGit("commit", "-m", "commit 3: feature-x")

	stdout.Reset()
	code = cli.Run([]string{"git", "scan", projectDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("rescan on branch failed: %d", code)
	}
	if !strings.Contains(stdout.String(), "feature-x") {
		t.Errorf("expected branch feature-x in scan: %s", stdout.String())
	}

	// 6. Working tree changes: staged, unstaged, untracked, deleted
	// Staged file
	stagedFile := filepath.Join(projectDir, "staged.txt")
	if err := os.WriteFile(stagedFile, []byte("staged"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "staged.txt")

	// Unstaged modified file
	if err := os.WriteFile(f1, []byte("v1 modified"), 0600); err != nil {
		t.Fatal(err)
	}

	// Untracked file
	untrackedFile := filepath.Join(projectDir, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("untracked"), 0600); err != nil {
		t.Fatal(err)
	}

	// Deleted file
	if err := os.Remove(f2); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	code = cli.Run([]string{"git", "scan", projectDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("scan with working tree changes failed: %d", code)
	}

	// 7. Check status via CLI
	stdout.Reset()
	code = cli.Run([]string{"git", "status", projectDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("git status failed: %d", code)
	}
	statusOut := stdout.String()
	if !strings.Contains(statusOut, "staged.txt") || !strings.Contains(statusOut, "staged") {
		t.Errorf("missing staged status: %s", statusOut)
	}
	if !strings.Contains(statusOut, "file1.txt") || !strings.Contains(statusOut, "modified") {
		t.Errorf("missing modified status: %s", statusOut)
	}
	if !strings.Contains(statusOut, "untracked.txt") || !strings.Contains(statusOut, "untracked") {
		t.Errorf("missing untracked status: %s", statusOut)
	}
	if !strings.Contains(statusOut, "file2.txt") || !strings.Contains(statusOut, "deleted") {
		t.Errorf("missing deleted status: %s", statusOut)
	}

	// 8. Rescan without changes: verify duplicate commits/snapshots are not created
	stdout.Reset()
	code = cli.Run([]string{"git", "scan", projectDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("rescan without changes failed: %d", code)
	}
	if !strings.Contains(stdout.String(), "New Commits:    0") {
		t.Errorf("expected 0 new commits on rescan, got: %s", stdout.String())
	}

	// 9. Verify doctor and other commands continue to pass
	stdout.Reset()
	code = cli.Run([]string{"doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor failed: %d", code)
	}
	if !strings.Contains(stdout.String(), "0 failure(s)") {
		t.Errorf("doctor reported failures: %s", stdout.String())
	}
}

func TestMainTerminalIntegration(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "lifelog_data")
	t.Setenv("LIFELOG_DATA_DIR", dataDir)

	var stdout, stderr bytes.Buffer

	// 1. Initialize
	code := cli.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init failed: %d", code)
	}

	// 2. Record command
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "record", "--", "echo", "integration terminal test"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("terminal record failed: %d", code)
	}
	if !strings.Contains(stdout.String(), "integration terminal test") {
		t.Errorf("missing command output: %s", stdout.String())
	}

	// 3. Record failing command
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "record", "--", "sh", "-c", "exit 7"}, &stdout, &stderr)
	if code != 7 {
		t.Fatalf("expected exit code 7, got %d", code)
	}

	// 4. Record secret command and ensure redaction
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "record", "--", "echo", "--api-key=supersecret123", "ghp_123456789012345678901234567890"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("terminal record secret failed: %d", code)
	}

	// 5. Query history
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "history"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("terminal history failed: %d", code)
	}
	historyOut := stdout.String()
	if !strings.Contains(historyOut, "showing 3") {
		t.Errorf("expected 3 records, got: %s", historyOut)
	}
	if !strings.Contains(historyOut, "ERR (7)") {
		t.Errorf("expected ERR (7), got: %s", historyOut)
	}
	if strings.Contains(historyOut, "supersecret123") || strings.Contains(historyOut, "ghp_123456789012345678901234567890") {
		t.Errorf("secret leaked in history: %s", historyOut)
	}

	// 6. Inspect, delete, clear
	fields := strings.Fields(historyOut)
	var termID string
	for _, f := range fields {
		if strings.HasPrefix(f, "term_") {
			termID = f
			break
		}
	}
	if termID == "" {
		t.Fatalf("could not find termID in: %s", historyOut)
	}

	stdout.Reset()
	code = cli.Run([]string{"terminal", "inspect", termID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("inspect failed: %d", code)
	}

	stdout.Reset()
	code = cli.Run([]string{"terminal", "delete", termID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("delete failed: %d", code)
	}

	stdout.Reset()
	code = cli.Run([]string{"terminal", "clear", "--confirm"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clear failed: %d", code)
	}

	// 7. Verify doctor passes with schema v4
	stdout.Reset()
	code = cli.Run([]string{"doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor failed: %d", code)
	}
	if !strings.Contains(stdout.String(), "0 failure(s)") {
		t.Errorf("doctor reported failures: %s", stdout.String())
	}
}

func TestMainSearchIntegration(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "lifelog_data")
	repoDir := filepath.Join(tempDir, "sample_repo")
	t.Setenv("LIFELOG_DATA_DIR", dataDir)

	var stdout, stderr bytes.Buffer

	// 1. Initialize storage
	code := cli.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init failed: %d", code)
	}

	// 2. Create git repository and commits
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	execGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v, out: %s", args, err, string(out))
		}
	}
	execGit("init", "-b", "main")
	execGit("config", "user.name", "SearchTester")
	execGit("config", "user.email", "search@example.com")
	if err := os.WriteFile(filepath.Join(repoDir, "search_feature.txt"), []byte("search content"), 0644); err != nil {
		t.Fatal(err)
	}
	execGit("add", "search_feature.txt")
	execGit("commit", "-m", "feat: implement unified query engine")

	// 3. Scan Git repo into LIFELOG
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"git", "scan", repoDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("git scan failed: %d", code)
	}

	// 4. Record terminal commands
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "record", "--", "echo", "run-search-indexer"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("terminal record failed: %d", code)
	}

	// 5. Search for Git commit subject
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"search", "unified query engine"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("search git failed: %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "feat: implement unified query engine") {
		t.Errorf("missing git commit in search output: %s", stdout.String())
	}

	// 6. Search for Terminal command
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"search", "--domain", "terminal", "run-search-indexer"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("search terminal failed: %d", code)
	}
	if !strings.Contains(stdout.String(), "run-search-indexer") {
		t.Errorf("missing terminal command in search output: %s", stdout.String())
	}

	// 7. Timeline check
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"timeline", "--limit", "10"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("timeline failed: %d", code)
	}
	if !strings.Contains(stdout.String(), "LIFELOG Unified Activity Timeline") {
		t.Errorf("missing timeline header: %s", stdout.String())
	}

	// 8. Reopen database and verify persistent search
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"search", "query engine"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("search on reopen failed: %d", code)
	}
	if !strings.Contains(stdout.String(), "unified query engine") {
		t.Errorf("expected search results on reopen: %s", stdout.String())
	}

	// 9. Verify doctor passes with schema v5
	stdout.Reset()
	code = cli.Run([]string{"doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor failed: %d", code)
	}
	if !strings.Contains(stdout.String(), "0 failure(s)") {
		t.Errorf("doctor reported failures: %s", stdout.String())
	}
}

func TestMainResumeIntegration(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "lifelog_data")
	projectDir := filepath.Join(tempDir, "resume_project")
	unrelatedDir := filepath.Join(tempDir, "unrelated_project")

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(unrelatedDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("LIFELOG_DATA_DIR", dataDir)

	var stdout, stderr bytes.Buffer

	// 1. Initialize LIFELOG
	code := cli.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init failed: %d", code)
	}

	// 2. Set up Git repo in projectDir
	execGit := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test Runner",
			"GIT_AUTHOR_EMAIL=test@lifelog.local",
			"GIT_COMMITTER_NAME=Test Runner",
			"GIT_COMMITTER_EMAIL=test@lifelog.local",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s failed: %v: %s", strings.Join(args, " "), err, string(out))
		}
	}

	execGit(projectDir, "init")
	execGit(projectDir, "checkout", "-b", "main")
	testFile := filepath.Join(projectDir, "service.go")
	if err := os.WriteFile(testFile, []byte("package service\n"), 0644); err != nil {
		t.Fatal(err)
	}
	execGit(projectDir, "add", "service.go")
	execGit(projectDir, "commit", "-m", "feat: create service skeleton")

	// Create uncommitted change
	dirtyFile := filepath.Join(projectDir, "dirty.txt")
	if err := os.WriteFile(dirtyFile, []byte("uncommitted change"), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Scan Git repo into LIFELOG
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"git", "scan", projectDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("git scan failed: %d", code)
	}

	// 4. Record filesystem event in projectDir
	p, err := paths.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	err = db.RecordEvents(context.Background(), []events.Event{
		{
			ID:        "fs-resume-1",
			Type:      events.EventCreated,
			Path:      testFile,
			Size:      16,
			ModTime:   now,
			Source:    "watch",
			CreatedAt: now,
		},
	})
	if err != nil {
		t.Fatalf("failed recording filesystem event: %v", err)
	}
	_ = db.Close()

	// 5. Record terminal command in projectDir
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
	code = cli.Run([]string{"terminal", "record", "--", "echo", "deploy-service-preview"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("terminal record failed: %d (stderr: %s)", code, stderr.String())
	}

	// 6. Execute lifelog resume for projectDir
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"resume", projectDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("resume failed: %d (stderr: %s)", code, stderr.String())
	}
	report := stdout.String()

	// Check sections and content
	if !strings.Contains(report, "LIFELOG PROJECT RESUMPTION REPORT") {
		t.Errorf("missing report title: %s", report)
	}
	if !strings.Contains(report, "1. OBSERVED ACTIVITY") {
		t.Errorf("missing Observed Activity section: %s", report)
	}
	if !strings.Contains(report, "service.go") {
		t.Errorf("missing service.go in filesystem activity: %s", report)
	}
	if !strings.Contains(report, "feat: create service skeleton") {
		t.Errorf("missing Git commit subject in report: %s", report)
	}
	if !strings.Contains(report, "deploy-service-preview") {
		t.Errorf("missing terminal command in report: %s", report)
	}
	if !strings.Contains(report, "2. DERIVED ACTIVITY SESSIONS") {
		t.Errorf("missing Derived Activity Sessions section: %s", report)
	}
	if !strings.Contains(report, "session-001") {
		t.Errorf("missing synthesized session-001 in report: %s", report)
	}
	if !strings.Contains(report, "Recent recorded activity suggests") {
		t.Errorf("missing evidence-based inference in report: %s", report)
	}
	if !strings.Contains(report, "3. UNAVAILABLE CONTEXT & PRIVACY BOUNDARIES") {
		t.Errorf("missing Unavailable Context section: %s", report)
	}

	// 7. Verify flags: --limit, --gap, --window
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"resume", projectDir, "--limit", "5", "--gap", "10m", "--window", "24h"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("resume with flags failed: %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "deploy-service-preview") {
		t.Errorf("flagged resume missing terminal command: %s", stdout.String())
	}

	// 8. Verify resume on unrelated directory returns empty activity and notices
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"resume", unrelatedDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("resume on unrelated dir failed: %d (stderr: %s)", code, stderr.String())
	}
	unrelatedReport := stdout.String()
	if !strings.Contains(unrelatedReport, "No recorded activity") {
		t.Errorf("expected no recorded activity for unrelated dir: %s", unrelatedReport)
	}
	if strings.Contains(unrelatedReport, "deploy-service-preview") {
		t.Errorf("leaked project activity into unrelated dir: %s", unrelatedReport)
	}
}

func TestMainExportAndCheckpointIntegration(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "lifelog_data")
	projectDir := filepath.Join(tempDir, "export_project")

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("LIFELOG_DATA_DIR", dataDir)

	var stdout, stderr bytes.Buffer

	// 1. Initialize LIFELOG
	code := cli.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init failed: %d", code)
	}

	// 2. Set up Git repo in projectDir
	execGit := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test Runner",
			"GIT_AUTHOR_EMAIL=test@lifelog.local",
			"GIT_COMMITTER_NAME=Test Runner",
			"GIT_COMMITTER_EMAIL=test@lifelog.local",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s failed: %v: %s", strings.Join(args, " "), err, string(out))
		}
	}

	execGit(projectDir, "init")
	execGit(projectDir, "checkout", "-b", "main")
	testFile := filepath.Join(projectDir, "core.go")
	if err := os.WriteFile(testFile, []byte("package core\n"), 0644); err != nil {
		t.Fatal(err)
	}
	execGit(projectDir, "add", "core.go")
	execGit(projectDir, "commit", "-m", "feat: initial core module")

	// 3. Scan Git repo
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"git", "scan", projectDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("git scan failed: %d", code)
	}

	// 4. Record terminal execution
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
	code = cli.Run([]string{"terminal", "record", "--", "echo", "run-core-benchmark"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("terminal record failed: %d", code)
	}

	// 5. Export to Markdown file
	exportFile := filepath.Join(projectDir, "exported-context.md")
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"export", projectDir, "--format", "markdown", "--output", exportFile}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("export markdown failed: %d (stderr: %s)", code, stderr.String())
	}
	mdContent, err := os.ReadFile(exportFile)
	if err != nil {
		t.Fatalf("failed reading export file: %v", err)
	}
	if !strings.Contains(string(mdContent), "# LIFELOG Project Resumption Report") {
		t.Errorf("missing title in exported markdown: %s", string(mdContent))
	}
	if !strings.Contains(string(mdContent), "run-core-benchmark") {
		t.Errorf("missing command in exported markdown: %s", string(mdContent))
	}

	// 6. Export to JSON (stdout)
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"export", projectDir, "--format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("export json failed: %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"schema_version": "1.0.0"`) {
		t.Errorf("missing schema_version in json export: %s", stdout.String())
	}

	// 7. Create named checkpoint
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"checkpoint", "create", projectDir, "--label", "Benchmark baseline"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("checkpoint create failed: %d (stderr: %s)", code, stderr.String())
	}
	chkCreateOut := stdout.String()
	var chkID string
	for _, line := range strings.Split(chkCreateOut, "\n") {
		if strings.HasPrefix(line, "ID:") {
			chkID = strings.TrimSpace(strings.TrimPrefix(line, "ID:"))
			break
		}
	}
	if chkID == "" {
		t.Fatalf("failed to parse checkpoint ID: %s", chkCreateOut)
	}

	// 8. Add subsequent activity
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"terminal", "record", "--", "echo", "post-checkpoint-activity"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("subsequent terminal record failed: %d", code)
	}

	// 9. Show checkpoint and verify immutability
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"checkpoint", "show", chkID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("checkpoint show failed: %d (stderr: %s)", code, stderr.String())
	}
	chkShowOut := stdout.String()
	if !strings.Contains(chkShowOut, "run-core-benchmark") {
		t.Errorf("checkpoint missing initial command: %s", chkShowOut)
	}
	if strings.Contains(chkShowOut, "post-checkpoint-activity") {
		t.Errorf("checkpoint mutated! contains post-checkpoint activity: %s", chkShowOut)
	}

	// 10. Delete checkpoint
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"checkpoint", "delete", chkID, "--confirm"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("checkpoint delete failed: %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "deleted successfully") {
		t.Errorf("missing deletion confirmation: %s", stdout.String())
	}

	// 11. Doctor check (schema v6)
	stdout.Reset()
	code = cli.Run([]string{"doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor failed: %d", code)
	}
	if !strings.Contains(stdout.String(), "0 failure(s)") {
		t.Errorf("doctor reported failures: %s", stdout.String())
	}
}

func TestMain_Prompt9_PackageAndProjectsE2E(t *testing.T) {
	tempDataDir := t.TempDir()
	t.Setenv(paths.EnvDataDir, tempDataDir)

	var stdout, stderr bytes.Buffer

	// 1. Initialize LIFELOG
	code := cli.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init failed: %d (stderr: %s)", code, stderr.String())
	}

	// 2. Set up real project Git repository
	projectDir := filepath.Join(t.TempDir(), "lifelog-e2e-project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	runGitCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = projectDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=TestAuthor",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=TestAuthor",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v (output: %s)", args, err, string(out))
		}
	}

	runGitCmd("init", "-b", "main")
	codeFile := filepath.Join(projectDir, "server.go")
	if err := os.WriteFile(codeFile, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitCmd("add", "server.go")
	runGitCmd("commit", "-m", "feat: initial server implementation")

	// 3. Scan Git repo into LIFELOG
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"git", "scan", projectDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("git scan failed: %d (stderr: %s)", code, stderr.String())
	}

	// 4. Create checkpoint
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"checkpoint", "create", projectDir, "--label", "v0.1-pre-release"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("checkpoint create failed: %d (stderr: %s)", code, stderr.String())
	}

	// 5. Test lifelog projects (Human text)
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"projects"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("projects failed: %d (stderr: %s)", code, stderr.String())
	}
	projectsOut := stdout.String()
	if !strings.Contains(projectsOut, "lifelog-e2e-project") {
		t.Errorf("missing project name in projects output: %s", projectsOut)
	}
	if !strings.Contains(projectsOut, "git (main)") {
		t.Errorf("missing git branch info in projects output: %s", projectsOut)
	}
	if !strings.Contains(projectsOut, "1 saved") {
		t.Errorf("missing checkpoint count in projects output: %s", projectsOut)
	}

	// 6. Test lifelog projects (--json)
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"projects", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("projects --json failed: %d (stderr: %s)", code, stderr.String())
	}
	var projList []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &projList); err != nil {
		t.Fatalf("failed unmarshaling projects json: %v", err)
	}
	if len(projList) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projList))
	}
	p := projList[0]
	if p["display_name"] != "lifelog-e2e-project" {
		t.Errorf("expected display_name lifelog-e2e-project, got %v", p["display_name"])
	}
	if p["is_git_registered"] != true {
		t.Errorf("expected is_git_registered true, got %v", p["is_git_registered"])
	}
	if p["exists_on_disk"] != true {
		t.Errorf("expected exists_on_disk true, got %v", p["exists_on_disk"])
	}

	// 7. Test lifelog package (Markdown to stdout)
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"package", projectDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("package markdown failed: %d (stderr: %s)", code, stderr.String())
	}
	pkgMd := stdout.String()
	if !strings.Contains(pkgMd, "# LIFELOG Project Context Package: lifelog-e2e-project") {
		t.Errorf("missing package title in markdown: %s", pkgMd)
	}
	if !strings.Contains(pkgMd, "Data Domain Coverage") {
		t.Errorf("missing domain coverage section in markdown: %s", pkgMd)
	}
	if !strings.Contains(pkgMd, "feat: initial server implementation") {
		t.Errorf("missing commit subject in package: %s", pkgMd)
	}

	// 8. Test lifelog package (JSON to file)
	pkgJSONPath := filepath.Join(projectDir, "package.json")
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"package", projectDir, "--format", "json", "--output", pkgJSONPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("package json to file failed: %d (stderr: %s)", code, stderr.String())
	}
	fInfo, err := os.Stat(pkgJSONPath)
	if err != nil {
		t.Fatalf("package file missing: %v", err)
	}
	if fInfo.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 permissions, got %04o", fInfo.Mode().Perm())
	}

	data, err := os.ReadFile(pkgJSONPath)
	if err != nil {
		t.Fatal(err)
	}
	var pkgObj map[string]interface{}
	if err := json.Unmarshal(data, &pkgObj); err != nil {
		t.Fatalf("unmarshal package json file failed: %v", err)
	}
	if pkgObj["package_version"] != "1.0.0" {
		t.Errorf("got package_version %v, want 1.0.0", pkgObj["package_version"])
	}
	if pkgObj["generator"] != "LIFELOG" {
		t.Errorf("got generator %v, want LIFELOG", pkgObj["generator"])
	}

	// 9. Protected file rejection
	forbiddenPath := filepath.Join(tempDataDir, "lifelog.db")
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"package", projectDir, "--output", forbiddenPath, "--force"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected error overwriting database file, got: %d", code)
	}

	// 10. Doctor check
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor failed: %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "0 failure(s)") {
		t.Errorf("doctor reported failures: %s", stdout.String())
	}
}

func TestMain_Prompt11_SubprocessBinaryAcceptance(t *testing.T) {
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "lifelog_bin")
	dataDir := filepath.Join(tempDir, "lifelog_data")

	// 1. Build real executable binary
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v (out: %s)", err, string(out))
	}

	runBin := func(args ...string) (string, string, int) {
		var outBuf, errBuf bytes.Buffer
		cmd := exec.Command(binPath, args...)
		cmd.Env = append(os.Environ(), "LIFELOG_DATA_DIR="+dataDir)
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		err := cmd.Run()
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}
		return outBuf.String(), errBuf.String(), exitCode
	}

	// 2. Validate help and version
	stdout, _, code := runBin("--help")
	if code != 0 || !strings.Contains(stdout, "Local-first personal computer memory system") {
		t.Fatalf("--help failed: code=%d, out=%s", code, stdout)
	}

	stdout, _, code = runBin("version")
	if code != 0 || !strings.Contains(stdout, "0.1.0") {
		t.Fatalf("version failed: code=%d, out=%s", code, stdout)
	}

	// 3. Validate invalid command returns usage exit code 2
	_, stderr, code := runBin("bogus-cmd")
	if code != 2 || !strings.Contains(stderr, "unknown command") {
		t.Fatalf("unknown command expected code 2, got %d (stderr: %s)", code, stderr)
	}

	// 4. Validate init and status
	stdout, _, code = runBin("init")
	if code != 0 || !strings.Contains(strings.ToLower(stdout), "storage initialized") {
		t.Fatalf("init failed: code=%d, out=%s", code, stdout)
	}

	stdout, _, code = runBin("status")
	if code != 0 || !strings.Contains(stdout, "Healthy") {
		t.Fatalf("status failed: code=%d, out=%s", code, stdout)
	}

	// 5. Test terminal record with synthetic secret
	syntheticSecret := "SYNTHETIC_TOP_SECRET_PASSWORD123"
	stdout, stderr, code = runBin("terminal", "record", "--", "echo", "--password="+syntheticSecret)
	if code != 0 {
		t.Fatalf("terminal record failed: code=%d (stderr: %s)", code, stderr)
	}
	if strings.Contains(stderr, syntheticSecret) {
		t.Fatalf("secret leaked in recording notice stderr: %s", stderr)
	}
	if !strings.Contains(stderr, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in recording notice: %s", stderr)
	}

	// 6. Verify search does not find secret
	stdout, _, code = runBin("search", syntheticSecret)
	if code != 0 {
		t.Fatalf("search failed: %d", code)
	}
	if !strings.Contains(stdout, "No matching records found") {
		t.Fatalf("expected 0 results searching for secret, got: %s", stdout)
	}

	// 7. Verify secret absent from raw SQLite database
	dbFile := filepath.Join(dataDir, "lifelog.db")
	rawDB, err := sql.Open("sqlite", dbFile)
	if err != nil {
		t.Fatalf("opening raw db failed: %v", err)
	}
	defer rawDB.Close()

	var termCmd, termArgs string
	err = rawDB.QueryRowContext(context.Background(), "SELECT sanitized_command, sanitized_args FROM terminal_records LIMIT 1").Scan(&termCmd, &termArgs)
	if err != nil {
		t.Fatalf("reading terminal record from db failed: %v", err)
	}
	if strings.Contains(termCmd, syntheticSecret) || strings.Contains(termArgs, syntheticSecret) {
		t.Fatalf("CRITICAL PRIVACY HAZARD: secret stored raw in terminal_records! cmd=%s, args=%s", termCmd, termArgs)
	}

	var ftsContent string
	_ = rawDB.QueryRowContext(context.Background(), "SELECT content FROM search_index WHERE content LIKE '%SECRET%' LIMIT 1").Scan(&ftsContent)
	if strings.Contains(ftsContent, syntheticSecret) {
		t.Fatalf("CRITICAL PRIVACY HAZARD: secret stored in search_index! content=%s", ftsContent)
	}

	// 8. Test Checkpoint lifecycle via CLI
	projPath := filepath.Join(tempDir, "my_cool_project")
	_ = os.MkdirAll(projPath, 0755)

	stdout, stderr, code = runBin("checkpoint", "create", projPath, "--label", "Acceptance Checkpoint")
	if code != 0 {
		t.Fatalf("checkpoint create failed: code=%d, err=%s", code, stderr)
	}
	if !strings.Contains(strings.ToLower(stdout), "checkpoint created") {
		t.Fatalf("unexpected checkpoint create output: %s", stdout)
	}

	stdout, _, code = runBin("checkpoint", "list", projPath)
	if code != 0 || !strings.Contains(stdout, "Acceptance Checkpoint") {
		t.Fatalf("checkpoint list failed: code=%d, out=%s", code, stdout)
	}

	// 9. Test export with atomic write and permissions
	exportFile := filepath.Join(tempDir, "export_result.json")
	stdout, stderr, code = runBin("export", projPath, "--format", "json", "--output", exportFile)
	if code != 0 {
		t.Fatalf("export failed: code=%d, err=%s", code, stderr)
	}
	info, err := os.Stat(exportFile)
	if err != nil {
		t.Fatalf("exported file not created: %v", err)
	}
	// Restrictive permissions (0600)
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("export file permission got %04o, want 0600", perm)
	}

	// 10. Reject export into protected LIFELOG_DATA_DIR
	protectedTarget := filepath.Join(dataDir, "tampered.db")
	_, stderr, code = runBin("export", projPath, "--output", protectedTarget, "--force")
	if code != 1 || !strings.Contains(stderr, "protected LIFELOG data directory") {
		t.Fatalf("expected rejection writing into data dir: code=%d, err=%s", code, stderr)
	}

	// 11. Run doctor
	stdout, _, code = runBin("doctor")
	if code != 0 || !strings.Contains(stdout, "0 failure(s)") {
		t.Fatalf("doctor reported failures: code=%d, out=%s", code, stdout)
	}
}
