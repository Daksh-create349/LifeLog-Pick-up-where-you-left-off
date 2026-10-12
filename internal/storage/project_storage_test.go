package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/git"
	"github.com/lifelog/lifelog/internal/storage"
	"github.com/lifelog/lifelog/internal/terminal"
)

func TestDiscoverProjects_Empty(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	projects, err := db.DiscoverProjects(ctx, storage.ProjectOverviewOptions{SortMode: storage.SortRecent})
	if err != nil {
		t.Fatalf("DiscoverProjects error: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(projects))
	}
}

func TestDiscoverProjects_AllDomainsAndAttribution(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create project A (Git repo on disk)
	projADir := filepath.Join(tempDir, "project-alpha")
	if err := os.MkdirAll(projADir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create project B (Nested Git repo within project A)
	projBDir := filepath.Join(projADir, "nested-beta")
	if err := os.MkdirAll(projBDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create project C (Non-git project with terminal records)
	projCDir := filepath.Join(tempDir, "project-gamma")
	if err := os.MkdirAll(projCDir, 0755); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	timeAlpha := now.Add(-2 * time.Hour)
	timeBeta := now.Add(-1 * time.Hour)
	timeGamma := now.Add(-30 * time.Minute)

	repoIDAlpha, err := db.RegisterOrUpdateGitRepo(ctx, projADir, false)
	if err != nil {
		t.Fatalf("register repo alpha: %v", err)
	}

	repoIDBeta, err := db.RegisterOrUpdateGitRepo(ctx, projBDir, false)
	if err != nil {
		t.Fatalf("register repo beta: %v", err)
	}

	// State for Alpha
	_, err = db.RecordGitState(ctx, repoIDAlpha, &git.RepoState{
		Root:       projADir,
		HeadBranch: "main",
		HeadCommit: &git.Commit{
			Hash:       "aaa111",
			AuthorName: "Alice",
			AuthorTime: timeAlpha,
			Subject:    "Alpha commit",
		},
		RecentCommits: []git.Commit{
			{
				Hash:       "aaa111",
				AuthorName: "Alice",
				AuthorTime: timeAlpha,
				Subject:    "Alpha commit",
			},
		},
		ObservedAt: timeAlpha,
	})
	if err != nil {
		t.Fatalf("record git state alpha: %v", err)
	}

	// State for Beta (nested)
	_, err = db.RecordGitState(ctx, repoIDBeta, &git.RepoState{
		Root:       projBDir,
		HeadBranch: "dev",
		HeadCommit: &git.Commit{
			Hash:       "bbb222",
			AuthorName: "Bob",
			AuthorTime: timeBeta,
			Subject:    "Beta commit",
		},
		RecentCommits: []git.Commit{
			{
				Hash:       "bbb222",
				AuthorName: "Bob",
				AuthorTime: timeBeta,
				Subject:    "Beta commit",
			},
		},
		ObservedAt: timeBeta,
	})
	if err != nil {
		t.Fatalf("record git state beta: %v", err)
	}

	// Filesystem event inside nested-beta
	err = db.RecordEvents(ctx, []events.Event{
		{
			ID:        "evt_beta",
			Type:      events.EventCreated,
			Path:      filepath.Join(projBDir, "beta.go"),
			Size:      100,
			ModTime:   timeBeta,
			Source:    "test",
			CreatedAt: timeBeta,
		},
	})
	if err != nil {
		t.Fatalf("record event beta: %v", err)
	}

	// Terminal command in Gamma
	exitCode := 0
	err = db.RecordTerminalExecution(ctx, &terminal.Record{
		ID:               "term_gamma",
		WorkingDir:       projCDir,
		Executable:       "npm",
		SanitizedCommand: "npm test",
		StartTime:        timeGamma,
		EndTime:          timeGamma.Add(time.Second),
		DurationMs:       1000,
		ExitCode:         &exitCode,
		CollectionMode:   "explicit",
		CreatedAt:        timeGamma,
	})
	if err != nil {
		t.Fatalf("record terminal gamma: %v", err)
	}

	// Discover projects
	projects, err := db.DiscoverProjects(ctx, storage.ProjectOverviewOptions{SortMode: storage.SortRecent})
	if err != nil {
		t.Fatalf("DiscoverProjects failed: %v", err)
	}

	if len(projects) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(projects))
	}

	// Because of SortRecent: Gamma is newest (30m ago), then Beta (1h ago), then Alpha (2h ago)
	if projects[0].DisplayName != "project-gamma" {
		t.Errorf("expected 1st project 'project-gamma', got %q", projects[0].DisplayName)
	}
	if projects[1].DisplayName != "nested-beta" {
		t.Errorf("expected 2nd project 'nested-beta', got %q", projects[1].DisplayName)
	}
	if projects[2].DisplayName != "project-alpha" {
		t.Errorf("expected 3rd project 'project-alpha', got %q", projects[2].DisplayName)
	}

	// Verify nested Git isolation: beta's event must attribute to beta, not alpha!
	for _, p := range projects {
		if p.DisplayName == "nested-beta" {
			if p.DomainCounts["filesystem"] != 1 {
				t.Errorf("nested-beta expected 1 filesystem event, got %d", p.DomainCounts["filesystem"])
			}
			if p.DomainCounts["git"] < 1 {
				t.Errorf("nested-beta expected >=1 git event, got %d", p.DomainCounts["git"])
			}
		} else if p.DisplayName == "project-alpha" {
			if p.DomainCounts["filesystem"] != 0 {
				t.Errorf("project-alpha should have 0 filesystem events (nested was isolated), got %d", p.DomainCounts["filesystem"])
			}
		}
	}

	// Test SortName
	projectsByName, err := db.DiscoverProjects(ctx, storage.ProjectOverviewOptions{SortMode: storage.SortName})
	if err != nil {
		t.Fatalf("DiscoverProjects by name failed: %v", err)
	}
	// "nested-beta" < "project-alpha" < "project-gamma"
	if projectsByName[0].DisplayName != "nested-beta" ||
		projectsByName[1].DisplayName != "project-alpha" ||
		projectsByName[2].DisplayName != "project-gamma" {
		t.Errorf("unexpected name sorting order: %s, %s, %s",
			projectsByName[0].DisplayName, projectsByName[1].DisplayName, projectsByName[2].DisplayName)
	}
}

func TestDiscoverProjects_MissingDirectoryAndCheckpoints(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Register a project that does not exist on disk
	deletedPath := filepath.Join(tempDir, "deleted-proj")
	err = db.CreateCheckpoint(ctx, &storage.CheckpointRecord{
		ID:            "chk_1",
		ProjectPath:   deletedPath,
		Label:         "Deleted Project Milestone",
		CreatedAt:     time.Now().UTC(),
		FormatVersion: "1.0.0",
		Payload:       "{}",
	})
	if err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}

	projects, err := db.DiscoverProjects(ctx, storage.ProjectOverviewOptions{SortMode: storage.SortRecent})
	if err != nil {
		t.Fatalf("DiscoverProjects failed: %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}

	p := projects[0]
	if p.DisplayName != "deleted-proj" {
		t.Errorf("expected display name 'deleted-proj', got %s", p.DisplayName)
	}
	if p.ExistsOnDisk {
		t.Errorf("expected ExistsOnDisk = false, got true")
	}
	if !p.HasCheckpoint || p.CheckpointCount != 1 {
		t.Errorf("expected 1 checkpoint, got %d (has=%v)", p.CheckpointCount, p.HasCheckpoint)
	}
}

func TestDiscoverProjects_IdenticalNamesAndSorting(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Two unrelated projects with identical folder name "api"
	dir1 := filepath.Join(tempDir, "userA", "api")
	dir2 := filepath.Join(tempDir, "userB", "api")
	_ = os.MkdirAll(dir1, 0755)
	_ = os.MkdirAll(dir2, 0755)

	_, err = db.RegisterOrUpdateGitRepo(ctx, dir1, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.RegisterOrUpdateGitRepo(ctx, dir2, false)
	if err != nil {
		t.Fatal(err)
	}

	projects, err := db.DiscoverProjects(ctx, storage.ProjectOverviewOptions{SortMode: storage.SortName})
	if err != nil {
		t.Fatalf("DiscoverProjects error: %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("expected 2 distinct projects despite matching display names, got %d", len(projects))
	}
	if projects[0].DisplayName != "api" || projects[1].DisplayName != "api" {
		t.Errorf("both projects should have display name 'api'")
	}
	if projects[0].CanonicalPath == projects[1].CanonicalPath {
		t.Errorf("projects should have distinct canonical paths")
	}
}

func TestDiscoverProjects_WindowAndLimits(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	projDir := filepath.Join(tempDir, "app")
	_ = os.MkdirAll(projDir, 0755)

	now := time.Now().UTC()
	oldTime := now.Add(-10 * 24 * time.Hour) // 10 days ago
	recentTime := now.Add(-1 * time.Hour)    // 1 hour ago

	err = db.RecordEvents(ctx, []events.Event{
		{
			ID:        "evt_old",
			Type:      events.EventCreated,
			Path:      filepath.Join(projDir, "old.txt"),
			Size:      10,
			ModTime:   oldTime,
			Source:    "test",
			CreatedAt: oldTime,
		},
		{
			ID:        "evt_recent",
			Type:      events.EventCreated,
			Path:      filepath.Join(projDir, "recent.txt"),
			Size:      20,
			ModTime:   recentTime,
			Source:    "test",
			CreatedAt: recentTime,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Without window (lifetime)
	allProjects, err := db.DiscoverProjects(ctx, storage.ProjectOverviewOptions{SortMode: storage.SortActivity})
	if err != nil {
		t.Fatal(err)
	}
	if len(allProjects) != 1 || allProjects[0].TotalEvents != 2 {
		t.Errorf("expected 2 total events lifetime, got %d", allProjects[0].TotalEvents)
	}

	// 2. With window: since 2 days ago
	sinceTime := now.Add(-2 * 24 * time.Hour)
	windowProjects, err := db.DiscoverProjects(ctx, storage.ProjectOverviewOptions{
		SortMode: storage.SortActivity,
		Since:    &sinceTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(windowProjects) != 1 || windowProjects[0].TotalEvents != 1 {
		t.Errorf("expected 1 event within window, got %d", windowProjects[0].TotalEvents)
	}

	// 3. Test limit
	limitedProjects, err := db.DiscoverProjects(ctx, storage.ProjectOverviewOptions{
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(limitedProjects) > 1 {
		t.Errorf("expected <= 1 project with limit=1, got %d", len(limitedProjects))
	}
}
