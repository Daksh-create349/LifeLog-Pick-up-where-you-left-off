package resumption_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/git"
	"github.com/lifelog/lifelog/internal/resumption"
	"github.com/lifelog/lifelog/internal/storage"
	"github.com/lifelog/lifelog/internal/terminal"
)

func setupTestDB(t *testing.T) (*storage.DB, string) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, tempDir
}

func TestBuildResumptionReport_EmptyHistory(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()

	targetDir := t.TempDir()
	report, err := resumption.BuildResumptionReport(ctx, db, targetDir, resumption.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Observed.Filesystem.TotalEvents != 0 {
		t.Errorf("expected 0 fs events, got %d", report.Observed.Filesystem.TotalEvents)
	}
	if report.Observed.Git.IsRegistered {
		t.Errorf("expected git not registered")
	}
	if report.Observed.Terminal.TotalCommands != 0 {
		t.Errorf("expected 0 terminal commands, got %d", report.Observed.Terminal.TotalCommands)
	}
	if len(report.Derived.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(report.Derived.Sessions))
	}
	if report.LatestActivityTime != nil {
		t.Errorf("expected nil latest activity time, got %v", report.LatestActivityTime)
	}

	var buf bytes.Buffer
	resumption.FormatReport(&buf, report)
	out := buf.String()

	if !strings.Contains(out, "LIFELOG PROJECT RESUMPTION REPORT") {
		t.Errorf("expected header in output")
	}
	if !strings.Contains(out, "No recorded activity") {
		t.Errorf("expected no recorded activity in output")
	}
}

func TestBuildResumptionReport_CombinedDomainsAndAssociation(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()

	projectDir := t.TempDir()
	unrelatedDir := t.TempDir()

	baseTime := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

	// 1. Filesystem Events in projectDir
	fsEvents := []events.Event{
		{
			ID:        "fs-1",
			Type:      events.EventCreated,
			Path:      filepath.Join(projectDir, "main.go"),
			Size:      120,
			ModTime:   baseTime,
			Source:    "watch",
			CreatedAt: baseTime,
		},
		{
			ID:        "fs-2",
			Type:      events.EventModified,
			Path:      filepath.Join(projectDir, "main.go"),
			Size:      250,
			ModTime:   baseTime.Add(5 * time.Minute),
			Source:    "watch",
			CreatedAt: baseTime.Add(5 * time.Minute),
		},
		// Unrelated event in unrelatedDir
		{
			ID:        "fs-3",
			Type:      events.EventModified,
			Path:      filepath.Join(unrelatedDir, "other.txt"),
			Size:      50,
			ModTime:   baseTime.Add(6 * time.Minute),
			Source:    "watch",
			CreatedAt: baseTime.Add(6 * time.Minute),
		},
	}
	if err := db.RecordEvents(ctx, fsEvents); err != nil {
		t.Fatalf("failed recording fs events: %v", err)
	}

	// 2. Git repo registered for projectDir
	repoID, err := db.RegisterOrUpdateGitRepo(ctx, projectDir, false)
	if err != nil {
		t.Fatalf("failed registering git repo: %v", err)
	}

	// Git snapshot and commit
	commitObj := git.Commit{
		Hash:       "commit1234567890abcdef",
		AuthorName: "Alice Dev",
		AuthorTime: baseTime.Add(10 * time.Minute),
		Subject:    "Implement resumption engine",
	}
	gitState := git.RepoState{
		Root:          projectDir,
		HeadBranch:    "feature-resume",
		HeadCommit:    &commitObj,
		RecentCommits: []git.Commit{commitObj},
		WorkingTree: []git.FileChange{
			{Path: "main.go", Status: git.StatusModified},
			{Path: "README.md", Status: git.StatusUntracked},
		},
		ObservedAt: baseTime.Add(12 * time.Minute),
	}
	if _, err := db.RecordGitState(ctx, repoID, &gitState); err != nil {
		t.Fatalf("failed recording git state: %v", err)
	}

	// 3. Terminal commands: 1 safe, 1 omitted
	exitZero := 0
	termRecords := []terminal.Record{
		{
			ID:               "term-1",
			WorkingDir:       projectDir,
			Executable:       "go",
			SanitizedArgs:    []string{"test", "./..."},
			SanitizedCommand: "go test ./...",
			StartTime:        baseTime.Add(15 * time.Minute),
			EndTime:          baseTime.Add(15*time.Minute + 500*time.Millisecond),
			DurationMs:       500,
			ExitCode:         &exitZero,
			CollectionMode:   "direct",
			CreatedAt:        baseTime.Add(15 * time.Minute),
		},
		{
			ID:               "term-2",
			WorkingDir:       projectDir,
			Executable:       "aws",
			SanitizedArgs:    []string{},
			SanitizedCommand: "[DETAILS OMITTED]",
			StartTime:        baseTime.Add(20 * time.Minute),
			EndTime:          baseTime.Add(20*time.Minute + 100*time.Millisecond),
			DurationMs:       100,
			ExitCode:         &exitZero,
			CollectionMode:   "direct",
			IsOmitted:        true,
			OmissionReason:   "sensitive cloud CLI executable: aws",
			CreatedAt:        baseTime.Add(20 * time.Minute),
		},
		// Unrelated command in unrelatedDir
		{
			ID:               "term-3",
			WorkingDir:       unrelatedDir,
			Executable:       "ls",
			SanitizedArgs:    []string{"-la"},
			SanitizedCommand: "ls -la",
			StartTime:        baseTime.Add(25 * time.Minute),
			EndTime:          baseTime.Add(25*time.Minute + 50*time.Millisecond),
			DurationMs:       50,
			ExitCode:         &exitZero,
			CollectionMode:   "direct",
			CreatedAt:        baseTime.Add(25 * time.Minute),
		},
	}
	for _, tr := range termRecords {
		trCopy := tr
		if err := db.RecordTerminalExecution(ctx, &trCopy); err != nil {
			t.Fatalf("failed recording terminal execution: %v", err)
		}
	}

	// 4. Build resumption report for projectDir
	report, err := resumption.BuildResumptionReport(ctx, db, projectDir, resumption.Options{
		Limit:         20,
		InactivityGap: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("unexpected error building report: %v", err)
	}

	// Verify Filesystem: only project events (2), NOT unrelatedDir event
	if report.Observed.Filesystem.TotalEvents != 2 {
		t.Errorf("expected 2 project fs events, got %d", report.Observed.Filesystem.TotalEvents)
	}
	if report.Observed.Filesystem.TypeCounts[events.EventCreated] != 1 || report.Observed.Filesystem.TypeCounts[events.EventModified] != 1 {
		t.Errorf("unexpected fs type counts: %+v", report.Observed.Filesystem.TypeCounts)
	}

	// Verify Git
	if !report.Observed.Git.IsRegistered {
		t.Errorf("expected registered git repo")
	}
	if report.Observed.Git.CurrentBranch != "feature-resume" {
		t.Errorf("expected branch feature-resume, got %q", report.Observed.Git.CurrentBranch)
	}
	if len(report.Observed.Git.WorkingTree) != 2 {
		t.Errorf("expected 2 working tree changes, got %d", len(report.Observed.Git.WorkingTree))
	}
	if len(report.Observed.Git.RecentCommits) != 1 {
		t.Errorf("expected 1 recent commit, got %d", len(report.Observed.Git.RecentCommits))
	}

	// Verify Terminal: only project records (2), NOT unrelatedDir command
	if report.Observed.Terminal.TotalCommands != 2 {
		t.Errorf("expected 2 project terminal commands, got %d", report.Observed.Terminal.TotalCommands)
	}

	// Verify Session Clustering: all events are within 20 mins of each other -> 1 single session
	if len(report.Derived.Sessions) != 1 {
		t.Fatalf("expected 1 unified session, got %d", len(report.Derived.Sessions))
	}
	s := report.Derived.Sessions[0]
	if s.DomainCounts["filesystem"] != 2 || s.DomainCounts["git"] != 1 || s.DomainCounts["terminal"] != 2 {
		t.Errorf("unexpected session domain counts: %+v", s.DomainCounts)
	}

	// Verify latest activity time
	expectedLatest := baseTime.Add(20 * time.Minute)
	if report.LatestActivityTime == nil || !report.LatestActivityTime.Equal(expectedLatest) {
		t.Errorf("expected latest time %v, got %v", expectedLatest, report.LatestActivityTime)
	}

	// Verify CLI formatting
	var buf bytes.Buffer
	resumption.FormatReport(&buf, report)
	output := buf.String()

	if !strings.Contains(output, "feature-resume") {
		t.Errorf("output missing branch name")
	}
	if !strings.Contains(output, "Implement resumption engine") {
		t.Errorf("output missing commit subject")
	}
	if !strings.Contains(output, "go test ./...") {
		t.Errorf("output missing terminal command")
	}
	// Omitted command must remain omitted
	if !strings.Contains(output, "[DETAILS OMITTED]") {
		t.Errorf("output did not preserve [DETAILS OMITTED] for sensitive command")
	}
	// Output must NOT leak unrelated directory
	if strings.Contains(output, unrelatedDir) {
		t.Errorf("output leaked unrelated directory %q", unrelatedDir)
	}
}

func TestBuildResumptionReport_NestedRepoSubpath(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()

	parentDir := t.TempDir()
	nestedDir := filepath.Join(parentDir, "subproject")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("failed creating subproject dir: %v", err)
	}

	// Register parent repo and nested repo
	_, err := db.RegisterOrUpdateGitRepo(ctx, parentDir, false)
	if err != nil {
		t.Fatalf("failed registering parent repo: %v", err)
	}
	nestedID, err := db.RegisterOrUpdateGitRepo(ctx, nestedDir, false)
	if err != nil {
		t.Fatalf("failed registering nested repo: %v", err)
	}

	// Query resumption for nestedDir -> should pick closest (nestedID)
	report, err := resumption.BuildResumptionReport(ctx, db, nestedDir, resumption.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Observed.Git.RepoID != nestedID {
		t.Errorf("expected nested repo ID %q, got %q", nestedID, report.Observed.Git.RepoID)
	}
}

func TestBuildResumptionReport_RelativePathResolution(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()

	projectDir := t.TempDir()
	now := time.Now().UTC()

	// Register Git repo
	repoID, err := db.RegisterOrUpdateGitRepo(ctx, projectDir, false)
	if err != nil {
		t.Fatalf("failed registering repo: %v", err)
	}

	// Record an FS event
	if err := db.RecordEvents(ctx, []events.Event{
		{
			ID:        "evt-rel-1",
			Type:      events.EventCreated,
			Path:      filepath.Join(projectDir, "sample.go"),
			Size:      100,
			ModTime:   now,
			Source:    "watch",
			CreatedAt: now,
		},
	}); err != nil {
		t.Fatalf("recording fs events: %v", err)
	}

	// Record a terminal execution
	exitZero := 0
	termRecord := terminal.Record{
		ID:               "term-rel-1",
		WorkingDir:       projectDir,
		Executable:       "go",
		SanitizedArgs:    []string{"build"},
		SanitizedCommand: "go build",
		StartTime:        now,
		EndTime:          now.Add(100 * time.Millisecond),
		DurationMs:       100,
		ExitCode:         &exitZero,
		CollectionMode:   "direct",
		CreatedAt:        now,
	}
	if err := db.RecordTerminalExecution(ctx, &termRecord); err != nil {
		t.Fatalf("recording terminal: %v", err)
	}

	// Temporarily switch working directory to projectDir
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting wd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("changing wd: %v", err)
	}

	// Query with relative path "."
	report, err := resumption.BuildResumptionReport(ctx, db, ".", resumption.Options{})
	if err != nil {
		t.Fatalf("BuildResumptionReport failed: %v", err)
	}

	if !report.Observed.Git.IsRegistered || report.Observed.Git.RepoID != repoID {
		t.Errorf("expected git repo %s to be registered in report, got %+v", repoID, report.Observed.Git)
	}
	if report.Observed.Filesystem.TotalEvents != 1 {
		t.Errorf("expected 1 filesystem event, got %d", report.Observed.Filesystem.TotalEvents)
	}
	if report.Observed.Terminal.TotalCommands != 1 {
		t.Errorf("expected 1 terminal command, got %d", report.Observed.Terminal.TotalCommands)
	}
}
