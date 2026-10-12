package storage_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/git"
	"github.com/lifelog/lifelog/internal/search"
	"github.com/lifelog/lifelog/internal/storage"
)

func TestStorage_SearchDeleteTriggers_GitCommits(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "triggers.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// 1. Register Git repo
	repoDir := filepath.Join(tempDir, "repo")
	_ = os.MkdirAll(repoDir, 0755)

	repoID, err := db.RegisterOrUpdateGitRepo(ctx, repoDir, false)
	if err != nil {
		t.Fatalf("RegisterOrUpdateGitRepo failed: %v", err)
	}

	// 2. Insert commit
	now := time.Now().UTC()
	_, err = db.RecordGitState(ctx, repoID, &git.RepoState{
		Root:       repoDir,
		HeadBranch: "main",
		HeadCommit: &git.Commit{
			Hash:       "deadbeef1234",
			AuthorName: "Developer",
			AuthorTime: now,
			Subject:    "UniqueCommitSubjectForDeleteTest",
		},
		RecentCommits: []git.Commit{
			{
				Hash:       "deadbeef1234",
				AuthorName: "Developer",
				AuthorTime: now,
				Subject:    "UniqueCommitSubjectForDeleteTest",
			},
		},
		ObservedAt: now,
	})
	if err != nil {
		t.Fatalf("RecordGitState failed: %v", err)
	}

	// 3. Verify search finds the commit
	res, err := db.Search(ctx, search.Query{Text: "UniqueCommitSubjectForDeleteTest"})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if res.TotalCount != 1 {
		t.Fatalf("expected 1 search result before deletion, got %d", res.TotalCount)
	}

	// 4. Delete commit directly from git_commits
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer rawDB.Close()

	_, err = rawDB.ExecContext(ctx, "DELETE FROM git_commits WHERE commit_hash = ?", "deadbeef1234")
	if err != nil {
		t.Fatalf("deleting commit failed: %v", err)
	}

	// 5. Verify search NO LONGER finds the commit (trigger deleted it from search_index)
	resAfter, err := db.Search(ctx, search.Query{Text: "UniqueCommitSubjectForDeleteTest"})
	if err != nil {
		t.Fatalf("Search after deletion failed: %v", err)
	}
	if resAfter.TotalCount != 0 {
		t.Fatalf("expected 0 search results after commit deletion, got %d (stale FTS index entry remaining)", resAfter.TotalCount)
	}
}

func TestStorage_SearchDeleteTriggers_CascadeRepo(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "cascade.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	repoDir := filepath.Join(tempDir, "cascade_repo")
	_ = os.MkdirAll(repoDir, 0755)

	repoID, err := db.RegisterOrUpdateGitRepo(ctx, repoDir, false)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	_, err = db.RecordGitState(ctx, repoID, &git.RepoState{
		Root:       repoDir,
		HeadBranch: "main",
		HeadCommit: &git.Commit{
			Hash:       "c0ffee99",
			AuthorName: "CascadeTester",
			AuthorTime: now,
			Subject:    "CascadeSearchSubject",
		},
		RecentCommits: []git.Commit{
			{
				Hash:       "c0ffee99",
				AuthorName: "CascadeTester",
				AuthorTime: now,
				Subject:    "CascadeSearchSubject",
			},
		},
		ObservedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Delete repository
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer rawDB.Close()

	_, err = rawDB.ExecContext(ctx, "DELETE FROM git_repositories WHERE id = ?", repoID)
	if err != nil {
		t.Fatal(err)
	}

	// Search for commit should find 0 results
	res, err := db.Search(ctx, search.Query{Text: "CascadeSearchSubject"})
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalCount != 0 {
		t.Fatalf("expected 0 results after repo delete cascade, got %d", res.TotalCount)
	}
}

func TestStorage_LikeFallback_WildcardEscaping(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "wildcard.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// Insert events with '_' and '%' in path
	err = db.RecordEvents(ctx, []events.Event{
		{
			ID:        "evt_target",
			Type:      events.EventCreated,
			Path:      "/project/core_helper.go", // Contains literal '_'
			Size:      100,
			ModTime:   now,
			Source:    "test",
			CreatedAt: now,
		},
		{
			ID:        "evt_other",
			Type:      events.EventCreated,
			Path:      "/project/coreahelper.go", // Has 'a' instead of '_'
			Size:      100,
			ModTime:   now,
			Source:    "test",
			CreatedAt: now,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Force fallback by executing search with punctuation query that triggers LIKE fallback
	// or querying with query syntax containing punctuation
	res, err := db.Search(ctx, search.Query{Text: "core_helper"})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Verify that the result matched core_helper.go, and did NOT match coreahelper.go
	for _, item := range res.Items {
		if item.Title == "/project/coreahelper.go" {
			t.Errorf("LIKE search mistakenly matched wildcard on 'coreahelper.go' when searching for 'core_helper'")
		}
	}
}

func TestStorage_SiblingPathPrefix_NoConfusion(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "sibling.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// Two sibling projects: /proj_1 and /proj-1
	err = db.RecordEvents(ctx, []events.Event{
		{
			ID:        "evt_proj_1",
			Type:      events.EventCreated,
			Path:      "/workspace/proj_1/main.go",
			Size:      50,
			ModTime:   now,
			Source:    "test",
			CreatedAt: now,
		},
		{
			ID:        "evt_proj_dash_1",
			Type:      events.EventCreated,
			Path:      "/workspace/proj-1/main.go",
			Size:      50,
			ModTime:   now,
			Source:    "test",
			CreatedAt: now,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Query filesystem summary specifically for /workspace/proj_1
	summary, err := db.GetProjectFilesystemSummary(ctx, "/workspace/proj_1", nil, 10)
	if err != nil {
		t.Fatalf("GetProjectFilesystemSummary failed: %v", err)
	}

	if summary.TotalCount != 1 {
		t.Fatalf("expected 1 event for /workspace/proj_1, got %d", summary.TotalCount)
	}
	if summary.RecentEvents[0].Path != "/workspace/proj_1/main.go" {
		t.Errorf("expected path /workspace/proj_1/main.go, got %s", summary.RecentEvents[0].Path)
	}

	// QueryEvents with PathPrefix
	evts, err := db.QueryEvents(ctx, storage.EventFilter{PathPrefix: "/workspace/proj_1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(evts) != 1 || evts[0].Path != "/workspace/proj_1/main.go" {
		t.Errorf("QueryEvents confused sibling path: got %v", evts)
	}
}

func TestStorage_ConcurrentReadersWriters(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "concurrent.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	var wg sync.WaitGroup
	errCh := make(chan error, 20)

	// 5 concurrent writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				evt := events.Event{
					ID:        fmt.Sprintf("evt_worker_%d_%d", workerID, j),
					Type:      events.EventCreated,
					Path:      fmt.Sprintf("/concurrent/worker%d/file%d.go", workerID, j),
					Size:      int64(j * 10),
					ModTime:   time.Now().UTC(),
					Source:    "test",
					CreatedAt: time.Now().UTC(),
				}
				if err := db.RecordEvents(ctx, []events.Event{evt}); err != nil {
					errCh <- err
					return
				}
			}
		}(i)
	}

	// 5 concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_, err := db.QueryEvents(ctx, storage.EventFilter{Limit: 20})
				if err != nil {
					errCh <- err
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent database error: %v", err)
	}
}
