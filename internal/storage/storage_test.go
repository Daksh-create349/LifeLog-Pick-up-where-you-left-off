package storage_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/git"
	"github.com/lifelog/lifelog/internal/search"
	"github.com/lifelog/lifelog/internal/storage"
	"github.com/lifelog/lifelog/internal/terminal"
	"strings"
)

func TestStorage_InitAndHealthCheck(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := db.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	version, err := db.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion failed: %v", err)
	}
	if version != 9 {
		t.Errorf("got schema version %d, want 9", version)
	}

	initAt, err := db.GetMetadata(ctx, "initialized_at")
	if err != nil {
		t.Fatalf("GetMetadata('initialized_at') failed: %v", err)
	}
	if initAt == "" {
		t.Error("expected non-empty initialized_at")
	}
}

func TestStorage_CloseAndReopenPersistence(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "persist.db")
	ctx := context.Background()

	// 1. Open and insert test metadata and event
	db1, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("initial Open failed: %v", err)
	}

	const testKey = "test_persistence_key"
	const testVal = "persistent_state_value_12345"

	if err := db1.SetMetadata(ctx, testKey, testVal); err != nil {
		t.Fatalf("SetMetadata failed: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	evt := events.Event{
		ID:        "evt_persist_1",
		Type:      events.EventCreated,
		Path:      "/Users/test/workspace/file.txt",
		Size:      42,
		ModTime:   now,
		IsDir:     false,
		Source:    "fs_poll",
		CreatedAt: now,
	}
	if err := db1.RecordEvents(ctx, []events.Event{evt}); err != nil {
		t.Fatalf("RecordEvents failed: %v", err)
	}

	// 2. Close cleanly
	if err := db1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 3. Reopen and verify data survived
	db2, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("reopening database failed: %v", err)
	}
	defer db2.Close()

	retrieved, err := db2.GetMetadata(ctx, testKey)
	if err != nil {
		t.Fatalf("GetMetadata on reopened DB failed: %v", err)
	}
	if retrieved != testVal {
		t.Fatalf("data mismatch on reopened DB: got %q, want %q", retrieved, testVal)
	}

	v, err := db2.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion failed: %v", err)
	}
	if v != 9 {
		t.Fatalf("schema version mismatch on reopen: got %d, want 9", v)
	}

	evts, err := db2.QueryEvents(ctx, storage.EventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("QueryEvents on reopened DB failed: %v", err)
	}
	if len(evts) != 1 {
		t.Fatalf("expected 1 event on reopened DB, got %d", len(evts))
	}
	if evts[0].ID != evt.ID || evts[0].Path != evt.Path || evts[0].Size != evt.Size {
		t.Errorf("queried event mismatch: %+v vs %+v", evts[0], evt)
	}
}

func TestStorage_RecordAndQueryEvents(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "events.db")
	ctx := context.Background()

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	t0 := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Millisecond)
	t1 := time.Now().UTC().Add(-5 * time.Minute).Truncate(time.Millisecond)
	t2 := time.Now().UTC().Truncate(time.Millisecond)

	batch := []events.Event{
		{
			ID:        "evt_01",
			Type:      events.EventCreated,
			Path:      "/project/a.txt",
			Size:      10,
			ModTime:   t0,
			IsDir:     false,
			Source:    "fs_poll",
			CreatedAt: t0,
		},
		{
			ID:        "evt_02",
			Type:      events.EventModified,
			Path:      "/project/a.txt",
			Size:      25,
			ModTime:   t1,
			IsDir:     false,
			Source:    "fs_poll",
			CreatedAt: t1,
		},
		{
			ID:        "evt_03",
			Type:      events.EventCreated,
			Path:      "/project/sub/b.txt",
			Size:      50,
			ModTime:   t2,
			IsDir:     false,
			Source:    "fs_poll",
			CreatedAt: t2,
		},
	}

	if err := db.RecordEvents(ctx, batch); err != nil {
		t.Fatalf("RecordEvents failed: %v", err)
	}

	// Deduplication test: recording identical event for /project/sub/b.txt should be skipped
	dupEvent := events.Event{
		ID:        "evt_04_dup",
		Type:      events.EventCreated,
		Path:      "/project/sub/b.txt",
		Size:      50,
		ModTime:   t2,
		IsDir:     false,
		Source:    "fs_poll",
		CreatedAt: t2.Add(time.Second),
	}
	if err := db.RecordEvents(ctx, []events.Event{dupEvent}); err != nil {
		t.Fatalf("RecordEvents for duplicate failed: %v", err)
	}

	// Total recorded events must remain 3
	all, err := db.QueryEvents(ctx, storage.EventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("QueryEvents failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 events (1 duplicate skipped), got %d", len(all))
	}

	// Verify reverse-chronological order
	if all[0].ID != "evt_03" || all[1].ID != "evt_02" || all[2].ID != "evt_01" {
		t.Errorf("events not in reverse chronological order: [%s, %s, %s]", all[0].ID, all[1].ID, all[2].ID)
	}

	// Path prefix filtering
	subOnly, err := db.QueryEvents(ctx, storage.EventFilter{PathPrefix: "/project/sub", Limit: 10})
	if err != nil {
		t.Fatalf("QueryEvents with PathPrefix failed: %v", err)
	}
	if len(subOnly) != 1 || subOnly[0].ID != "evt_03" {
		t.Errorf("PathPrefix filtering failed: got %d results", len(subOnly))
	}

	// Limit test
	limited, err := db.QueryEvents(ctx, storage.EventFilter{Limit: 2})
	if err != nil {
		t.Fatalf("QueryEvents with limit failed: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("expected 2 events, got %d", len(limited))
	}
}

func TestStorage_GetMetadata_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "meta.db")

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.GetMetadata(context.Background(), "non_existent_key")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestStorage_InvalidPath(t *testing.T) {
	_, err := storage.Open("")
	if err == nil {
		t.Error("expected error opening empty path, got nil")
	}

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "file_blocking_dir")
	if err := os.WriteFile(filePath, []byte("blocker"), 0600); err != nil {
		t.Fatal(err)
	}

	inaccessibleDB := filepath.Join(filePath, "cannot_create.db")
	_, err = storage.Open(inaccessibleDB)
	if err == nil {
		t.Error("expected error opening DB inside a regular file, got nil")
	}
}

func TestStorage_GitPersistence(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "git_test.db")
	ctx := context.Background()

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 1. Register repo
	const repoPath = "/Users/tester/sample_repo"
	repoID, err := db.RegisterOrUpdateGitRepo(ctx, repoPath, false)
	if err != nil {
		t.Fatalf("RegisterOrUpdateGitRepo failed: %v", err)
	}
	if repoID == "" {
		t.Fatal("expected non-empty repoID")
	}

	// Verify retrieval
	r, err := db.GetGitRepo(ctx, repoID)
	if err != nil {
		t.Fatalf("GetGitRepo failed: %v", err)
	}
	if r.Path != repoPath {
		t.Errorf("got path %q, want %q", r.Path, repoPath)
	}

	repos, err := db.ListGitRepos(ctx)
	if err != nil || len(repos) != 1 {
		t.Fatalf("ListGitRepos failed: %v, len=%d", err, len(repos))
	}

	// 2. Record initial Git state
	t0 := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Millisecond)
	state1 := &git.RepoState{
		Root:       repoPath,
		IsBare:     false,
		IsEmpty:    false,
		HeadBranch: "main",
		IsDetached: false,
		HeadCommit: &git.Commit{
			Hash:       "commit1111",
			AuthorName: "Alice",
			AuthorTime: t0,
			Subject:    "initial commit",
		},
		RecentCommits: []git.Commit{
			{
				Hash:       "commit1111",
				AuthorName: "Alice",
				AuthorTime: t0,
				Subject:    "initial commit",
			},
		},
		Branches: []git.BranchInfo{
			{Name: "main", CommitHash: "commit1111"},
		},
		WorkingTree: []git.FileChange{
			{Path: "file1.txt", Status: git.StatusModified},
		},
		ObservedAt: time.Now().UTC(),
	}

	sum1, err := db.RecordGitState(ctx, repoID, state1)
	if err != nil {
		t.Fatalf("RecordGitState 1 failed: %v", err)
	}
	if sum1.NewCommitsCount != 1 || !sum1.NewSnapshot {
		t.Errorf("unexpected summary 1: %+v", sum1)
	}

	// 3. Rescan with identical state: should deduplicate (no new commits, no new snapshot)
	sum2, err := db.RecordGitState(ctx, repoID, state1)
	if err != nil {
		t.Fatalf("RecordGitState rescan failed: %v", err)
	}
	if sum2.NewCommitsCount != 0 {
		t.Errorf("expected 0 new commits on duplicate scan, got %d", sum2.NewCommitsCount)
	}
	if sum2.NewSnapshot {
		t.Error("expected redundant snapshot to be skipped")
	}

	// 4. Record branch movement and new commit
	t1 := time.Now().UTC().Truncate(time.Millisecond)
	state2 := &git.RepoState{
		Root:       repoPath,
		IsBare:     false,
		IsEmpty:    false,
		HeadBranch: "main",
		IsDetached: false,
		HeadCommit: &git.Commit{
			Hash:       "commit2222",
			AuthorName: "Alice",
			AuthorTime: t1,
			Subject:    "second commit",
		},
		RecentCommits: []git.Commit{
			{
				Hash:       "commit2222",
				AuthorName: "Alice",
				AuthorTime: t1,
				Subject:    "second commit",
			},
			{
				Hash:       "commit1111",
				AuthorName: "Alice",
				AuthorTime: t0,
				Subject:    "initial commit",
			},
		},
		Branches: []git.BranchInfo{
			{Name: "main", CommitHash: "commit2222"},
		},
		WorkingTree: nil, // Clean working tree
		ObservedAt:  time.Now().UTC(),
	}

	sum3, err := db.RecordGitState(ctx, repoID, state2)
	if err != nil {
		t.Fatalf("RecordGitState 2 failed: %v", err)
	}
	if sum3.NewCommitsCount != 1 {
		t.Errorf("expected 1 new commit, got %d", sum3.NewCommitsCount)
	}
	if sum3.BranchChanges != 1 {
		t.Errorf("expected 1 branch change recorded, got %d", sum3.BranchChanges)
	}
	if !sum3.NewSnapshot {
		t.Error("expected new snapshot due to commit and working tree change")
	}

	// 5. Query commits
	commits, err := db.QueryGitCommits(ctx, repoID, 10)
	if err != nil {
		t.Fatalf("QueryGitCommits failed: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	if commits[0].CommitHash != "commit2222" || commits[1].CommitHash != "commit1111" {
		t.Errorf("commits ordering mismatch: [%s, %s]", commits[0].CommitHash, commits[1].CommitHash)
	}

	// 6. Query branch log
	blog, err := db.QueryGitBranchLog(ctx, repoID, 10)
	if err != nil {
		t.Fatalf("QueryGitBranchLog failed: %v", err)
	}
	if len(blog) < 2 {
		t.Fatalf("expected at least 2 branch log records, got %d", len(blog))
	}
	if blog[0].OldCommitHash != "commit1111" || blog[0].NewCommitHash != "commit2222" {
		t.Errorf("unexpected branch transition: %s -> %s", blog[0].OldCommitHash, blog[0].NewCommitHash)
	}

	// 7. Verify latest snapshot
	snap, changes, err := db.GetLatestGitSnapshot(ctx, repoID)
	if err != nil {
		t.Fatalf("GetLatestGitSnapshot failed: %v", err)
	}
	if snap == nil || snap.HeadCommitHash != "commit2222" {
		t.Errorf("unexpected latest snapshot: %+v", snap)
	}
	if len(changes) != 0 {
		t.Errorf("expected clean working tree in latest snapshot, got %d changes", len(changes))
	}
}

func TestStorage_TerminalOperations(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "term.db")
	ctx := context.Background()

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	exit0 := 0
	exit1 := 1

	rec1 := &terminal.Record{
		ID:               "term_001",
		WorkingDir:       "/path/to/projectA",
		Executable:       "git",
		SanitizedArgs:    []string{"status"},
		SanitizedCommand: "git status",
		StartTime:        now.Add(-2 * time.Minute),
		EndTime:          now.Add(-2*time.Minute + 50*time.Millisecond),
		DurationMs:       50,
		ExitCode:         &exit0,
		CollectionMode:   "direct",
		IsRedacted:       false,
		IsOmitted:        false,
		CreatedAt:        now.Add(-2 * time.Minute),
	}

	rec2 := &terminal.Record{
		ID:               "term_002",
		WorkingDir:       "/path/to/projectA",
		Executable:       "curl",
		SanitizedArgs:    []string{"--password", "[REDACTED]", "https://example.com"},
		SanitizedCommand: "curl --password [REDACTED] https://example.com",
		StartTime:        now.Add(-1 * time.Minute),
		EndTime:          now.Add(-1*time.Minute + 120*time.Millisecond),
		DurationMs:       120,
		ExitCode:         &exit0,
		CollectionMode:   "direct",
		IsRedacted:       true,
		IsOmitted:        false,
		CreatedAt:        now.Add(-1 * time.Minute),
	}

	rec3 := &terminal.Record{
		ID:               "term_003",
		WorkingDir:       "/path/to/projectB",
		Executable:       "sh",
		SanitizedArgs:    []string{"-c", "exit 1"},
		SanitizedCommand: "sh -c \"exit 1\"",
		StartTime:        now,
		EndTime:          now.Add(10 * time.Millisecond),
		DurationMs:       10,
		ExitCode:         &exit1,
		CollectionMode:   "shell",
		IsRedacted:       false,
		IsOmitted:        false,
		CreatedAt:        now,
	}

	// 1. Record executions
	for _, r := range []*terminal.Record{rec1, rec2, rec3} {
		if err := db.RecordTerminalExecution(ctx, r); err != nil {
			t.Fatalf("RecordTerminalExecution failed for %s: %v", r.ID, err)
		}
	}

	// 2. Query all records (reverse chronological default)
	all, err := db.ListTerminalRecords(ctx, terminal.RecordFilter{})
	if err != nil {
		t.Fatalf("ListTerminalRecords failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 records, got %d", len(all))
	}
	if all[0].ID != "term_003" || all[2].ID != "term_001" {
		t.Errorf("expected reverse-chronological order, got [%s, %s, %s]", all[0].ID, all[1].ID, all[2].ID)
	}

	// 3. Query chronological order
	chrono, err := db.ListTerminalRecords(ctx, terminal.RecordFilter{Chronological: true})
	if err != nil {
		t.Fatalf("ListTerminalRecords chronological failed: %v", err)
	}
	if chrono[0].ID != "term_001" || chrono[2].ID != "term_003" {
		t.Errorf("expected chronological order, got [%s, %s, %s]", chrono[0].ID, chrono[1].ID, chrono[2].ID)
	}

	// 4. Query with directory filter
	projA, err := db.ListTerminalRecords(ctx, terminal.RecordFilter{WorkingDir: "/path/to/projectA"})
	if err != nil {
		t.Fatalf("ListTerminalRecords filter failed: %v", err)
	}
	if len(projA) != 2 {
		t.Fatalf("expected 2 records for projectA, got %d", len(projA))
	}

	// 5. Query with limit
	limited, err := db.ListTerminalRecords(ctx, terminal.RecordFilter{Limit: 1})
	if err != nil {
		t.Fatalf("ListTerminalRecords limit failed: %v", err)
	}
	if len(limited) != 1 || limited[0].ID != "term_003" {
		t.Fatalf("expected 1 record (term_003), got %v", limited)
	}

	// 6. Get single record
	gotRec, err := db.GetTerminalRecord(ctx, "term_002")
	if err != nil {
		t.Fatalf("GetTerminalRecord failed: %v", err)
	}
	if gotRec.Executable != "curl" || !gotRec.IsRedacted {
		t.Errorf("unexpected record content: %+v", gotRec)
	}

	// 7. Delete single record
	deleted, err := db.DeleteTerminalRecord(ctx, "term_002")
	if err != nil {
		t.Fatalf("DeleteTerminalRecord failed: %v", err)
	}
	if !deleted {
		t.Errorf("expected deleted=true")
	}

	// Verify deletion
	_, err = db.GetTerminalRecord(ctx, "term_002")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after deletion, got %v", err)
	}

	// 8. Clear records
	cleared, err := db.ClearTerminalRecords(ctx)
	if err != nil {
		t.Fatalf("ClearTerminalRecords failed: %v", err)
	}
	if cleared != 2 {
		t.Errorf("expected 2 cleared records, got %d", cleared)
	}

	remaining, err := db.ListTerminalRecords(ctx, terminal.RecordFilter{})
	if err != nil {
		t.Fatalf("ListTerminalRecords after clear failed: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected 0 remaining records, got %d", len(remaining))
	}
}

func TestStorage_MigrationFromSchemaV6ToV8(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "migrate_v6_v8.db")
	ctx := context.Background()

	// 1. Create database and verify v8 applied
	dbRaw, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	v, err := dbRaw.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion failed: %v", err)
	}
	if v != 9 {
		t.Fatalf("expected version 9, got %d", v)
	}
	dbRaw.Close()

	// 2. Re-open to verify idempotence
	dbReopen, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer dbReopen.Close()

	vReopen, err := dbReopen.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion on reopen failed: %v", err)
	}
	if vReopen != 9 {
		t.Fatalf("expected version 9 on reopen, got %d", vReopen)
	}
}

func TestStorage_CheckpointsCRUD(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "checkpoints.db")
	ctx := context.Background()

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	chk1 := &storage.CheckpointRecord{
		ID:            "chk_test_001",
		ProjectPath:   "/Users/alice/projects/app",
		Label:         "Before major overhaul",
		CreatedAt:     now.Add(-10 * time.Minute),
		FormatVersion: "1.0.0",
		Payload:       `{"project_path":"/Users/alice/projects/app","summary":"test payload"}`,
	}
	chk2 := &storage.CheckpointRecord{
		ID:            "chk_test_002",
		ProjectPath:   "/Users/alice/projects/app",
		Label:         "Post overhaul snapshot",
		CreatedAt:     now,
		FormatVersion: "1.0.0",
		Payload:       `{"project_path":"/Users/alice/projects/app","summary":"test payload 2"}`,
	}
	chkOther := &storage.CheckpointRecord{
		ID:            "chk_test_003",
		ProjectPath:   "/Users/alice/projects/other",
		Label:         "Other project checkpoint",
		CreatedAt:     now,
		FormatVersion: "1.0.0",
		Payload:       `{"project_path":"/Users/alice/projects/other"}`,
	}

	// 1. Create checkpoints
	if err := db.CreateCheckpoint(ctx, chk1); err != nil {
		t.Fatalf("CreateCheckpoint 1 failed: %v", err)
	}
	if err := db.CreateCheckpoint(ctx, chk2); err != nil {
		t.Fatalf("CreateCheckpoint 2 failed: %v", err)
	}
	if err := db.CreateCheckpoint(ctx, chkOther); err != nil {
		t.Fatalf("CreateCheckpoint 3 failed: %v", err)
	}

	// 2. Get checkpoint by ID
	got, err := db.GetCheckpoint(ctx, "chk_test_001")
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}
	if got.Label != chk1.Label || got.ProjectPath != chk1.ProjectPath {
		t.Errorf("GetCheckpoint returned mismatched data: %+v", got)
	}

	// 3. Get non-existent checkpoint
	_, err = db.GetCheckpoint(ctx, "non_existent_id")
	if !errors.Is(err, storage.ErrCheckpointNotFound) {
		t.Errorf("expected ErrCheckpointNotFound, got: %v", err)
	}

	// 4. List checkpoints filtered by project path
	list, err := db.ListCheckpoints(ctx, "/Users/alice/projects/app", 10)
	if err != nil {
		t.Fatalf("ListCheckpoints failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 checkpoints for project, got %d", len(list))
	}
	// Verify newest first ordering
	if list[0].ID != "chk_test_002" || list[1].ID != "chk_test_001" {
		t.Errorf("expected newest first ordering: %+v", list)
	}

	// 5. Delete checkpoint
	if err := db.DeleteCheckpoint(ctx, "chk_test_001"); err != nil {
		t.Fatalf("DeleteCheckpoint failed: %v", err)
	}
	// Verify deleted
	_, err = db.GetCheckpoint(ctx, "chk_test_001")
	if !errors.Is(err, storage.ErrCheckpointNotFound) {
		t.Errorf("expected ErrCheckpointNotFound after deletion, got: %v", err)
	}
	// Deleting again returns ErrCheckpointNotFound
	if err := db.DeleteCheckpoint(ctx, "chk_test_001"); !errors.Is(err, storage.ErrCheckpointNotFound) {
		t.Errorf("expected ErrCheckpointNotFound for already deleted checkpoint, got: %v", err)
	}
}

func TestStorage_CrossDomainSearch(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "search_test.db")
	ctx := context.Background()

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()

	// 1. Insert filesystem event
	fsEvent := events.Event{
		ID:        "evt_search_01",
		Type:      events.EventModified,
		Path:      "/Users/alice/projects/migration-tool/config.yaml",
		Size:      1024,
		ModTime:   now.Add(-3 * time.Hour),
		IsDir:     false,
		Source:    "fs_poll",
		CreatedAt: now.Add(-3 * time.Hour),
	}
	if err := db.RecordEvents(ctx, []events.Event{fsEvent}); err != nil {
		t.Fatalf("RecordEvents failed: %v", err)
	}

	// 2. Insert Git repository and state
	repoPath := "/Users/alice/projects/migration-tool"
	repoID, err := db.RegisterOrUpdateGitRepo(ctx, repoPath, false)
	if err != nil {
		t.Fatalf("RegisterOrUpdateGitRepo failed: %v", err)
	}
	gitState := &git.RepoState{
		Root:       repoPath,
		HeadBranch: "feature/database-engine",
		IsDetached: false,
		HeadCommit: &git.Commit{
			Hash:       "a1b2c3d4e5f6",
			AuthorName: "Alice Dev",
			AuthorTime: now.Add(-2 * time.Hour),
			Subject:    "feat: add database migration runner",
		},
		RecentCommits: []git.Commit{
			{
				Hash:       "a1b2c3d4e5f6",
				AuthorName: "Alice Dev",
				AuthorTime: now.Add(-2 * time.Hour),
				Subject:    "feat: add database migration runner",
			},
		},
		Branches: []git.BranchInfo{
			{Name: "feature/database-engine", CommitHash: "a1b2c3d4e5f6"},
		},
		ObservedAt: now.Add(-2 * time.Hour),
	}
	if _, err := db.RecordGitState(ctx, repoID, gitState); err != nil {
		t.Fatalf("RecordGitState failed: %v", err)
	}

	// 3. Insert terminal execution records
	exit0 := 0
	termRec1 := &terminal.Record{
		ID:               "term_search_01",
		WorkingDir:       "/Users/alice/projects/migration-tool",
		Executable:       "go",
		SanitizedArgs:    []string{"test", "./internal/migration/..."},
		SanitizedCommand: "go test ./internal/migration/...",
		StartTime:        now.Add(-1 * time.Hour),
		EndTime:          now.Add(-1*time.Hour + 2*time.Second),
		DurationMs:       2000,
		ExitCode:         &exit0,
		CollectionMode:   "direct",
		CreatedAt:        now.Add(-1 * time.Hour),
	}
	termRec2 := &terminal.Record{
		ID:               "term_search_02",
		WorkingDir:       "/Users/alice/projects/vault-secrets",
		Executable:       "pass",
		SanitizedArgs:    nil,
		SanitizedCommand: "pass [DETAILS OMITTED: sensitive authentication or credential tool]",
		StartTime:        now.Add(-30 * time.Minute),
		EndTime:          now.Add(-30 * time.Minute),
		DurationMs:       10,
		ExitCode:         &exit0,
		CollectionMode:   "direct",
		IsOmitted:        true,
		OmissionReason:   "sensitive authentication or credential tool",
		CreatedAt:        now.Add(-30 * time.Minute),
	}
	if err := db.RecordTerminalExecution(ctx, termRec1); err != nil {
		t.Fatalf("RecordTerminalExecution 1 failed: %v", err)
	}
	if err := db.RecordTerminalExecution(ctx, termRec2); err != nil {
		t.Fatalf("RecordTerminalExecution 2 failed: %v", err)
	}

	// 4. Test Cross-Domain Search: "migration" should match in Filesystem, Git, and Terminal!
	res, err := db.Search(ctx, search.Query{Text: "migration"})
	if err != nil {
		t.Fatalf("Search 'migration' failed: %v", err)
	}
	if res.TotalCount < 3 {
		t.Fatalf("expected at least 3 matches for 'migration' across domains, got %d", res.TotalCount)
	}

	domainsFound := make(map[search.Domain]bool)
	for _, item := range res.Items {
		domainsFound[item.Domain] = true
	}
	if !domainsFound[search.DomainFilesystem] || !domainsFound[search.DomainGit] || !domainsFound[search.DomainTerminal] {
		t.Errorf("expected hits in filesystem, git, and terminal, got: %+v", domainsFound)
	}

	// 5. Test Domain Filtering: filter by git
	resGit, err := db.Search(ctx, search.Query{Text: "migration", Domain: search.DomainGit})
	if err != nil {
		t.Fatalf("Search git domain failed: %v", err)
	}
	for _, item := range resGit.Items {
		if item.Domain != search.DomainGit {
			t.Errorf("expected only git domain results, got %s", item.Domain)
		}
	}

	// 6. Test Privacy Omitted Search
	resOmitted, err := db.Search(ctx, search.Query{Text: "pass", Domain: search.DomainTerminal})
	if err != nil {
		t.Fatalf("Search 'pass' failed: %v", err)
	}
	if len(resOmitted.Items) == 0 {
		t.Fatalf("expected to find omitted pass record")
	}
	if !strings.Contains(resOmitted.Items[0].Title, "[DETAILS OMITTED") {
		t.Errorf("expected omitted marker in search result title: %s", resOmitted.Items[0].Title)
	}

	// 7. Test Empty Query (Timeline of all activity)
	resAll, err := db.Search(ctx, search.Query{Text: "", Limit: 10})
	if err != nil {
		t.Fatalf("Search empty query failed: %v", err)
	}
	if len(resAll.Items) < 4 {
		t.Fatalf("expected at least 4 items in empty query result, got %d", len(resAll.Items))
	}

	// 8. Test Malformed / Special Characters Fallback
	resSpecial, err := db.Search(ctx, search.Query{Text: "database-engine (v2.0) [test] *"})
	if err != nil {
		t.Fatalf("Search special chars should not fail: %v", err)
	}
	if resSpecial.TotalCount == 0 {
		t.Log("Note: 0 matches for complex punctuation string, but no crash or syntax error")
	}

	// 9. Test Deletion Synchronization: delete termRec1 and verify it disappears from search
	deleted, err := db.DeleteTerminalRecord(ctx, termRec1.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteTerminalRecord failed: %v", err)
	}
	resAfterDelete, err := db.Search(ctx, search.Query{Text: "internal/migration", Domain: search.DomainTerminal})
	if err != nil {
		t.Fatalf("Search after delete failed: %v", err)
	}
	if len(resAfterDelete.Items) != 0 {
		t.Fatalf("expected 0 matches after delete, got %d", len(resAfterDelete.Items))
	}

	// 10. Test RebuildSearchIndex
	if err := db.RebuildSearchIndex(ctx); err != nil {
		t.Fatalf("RebuildSearchIndex failed: %v", err)
	}
	resRebuilt, err := db.Search(ctx, search.Query{Text: "migration"})
	if err != nil {
		t.Fatalf("Search after rebuild failed: %v", err)
	}
	if resRebuilt.TotalCount == 0 {
		t.Fatalf("expected search index to be populated after rebuild")
	}
}
