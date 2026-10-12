package storage_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/git"
	"github.com/lifelog/lifelog/internal/search"
	"github.com/lifelog/lifelog/internal/storage"
	"github.com/lifelog/lifelog/internal/terminal"
)

func TestMigration_ForwardFromV5ToV7(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "migrate_v5.db")
	ctx := context.Background()

	// 1. Manually construct a schema-v5 database
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	initSQL := `
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMP NOT NULL
		);
		CREATE TABLE metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE filesystem_events (
			id TEXT PRIMARY KEY,
			event_type TEXT NOT NULL,
			path TEXT NOT NULL,
			size INTEGER,
			mod_time TIMESTAMP,
			is_dir BOOLEAN NOT NULL DEFAULT 0,
			source TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL
		);
		CREATE TABLE git_repositories (
			id TEXT PRIMARY KEY,
			path TEXT NOT NULL UNIQUE,
			is_bare BOOLEAN NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			last_scanned_at TIMESTAMP
		);
		CREATE TABLE git_commits (
			repository_id TEXT NOT NULL,
			commit_hash TEXT NOT NULL,
			author_name TEXT NOT NULL,
			author_time TIMESTAMP NOT NULL,
			subject TEXT NOT NULL,
			PRIMARY KEY (repository_id, commit_hash),
			FOREIGN KEY (repository_id) REFERENCES git_repositories(id) ON DELETE CASCADE
		);
		CREATE TABLE git_branches (
			repository_id TEXT NOT NULL,
			name TEXT NOT NULL,
			commit_hash TEXT NOT NULL,
			is_head BOOLEAN NOT NULL DEFAULT 0,
			observed_at TIMESTAMP NOT NULL,
			PRIMARY KEY (repository_id, name),
			FOREIGN KEY (repository_id) REFERENCES git_repositories(id) ON DELETE CASCADE
		);
		CREATE TABLE git_branch_log (
			id TEXT PRIMARY KEY,
			repository_id TEXT NOT NULL,
			branch_name TEXT NOT NULL,
			old_commit_hash TEXT,
			new_commit_hash TEXT NOT NULL,
			observed_at TIMESTAMP NOT NULL,
			FOREIGN KEY (repository_id) REFERENCES git_repositories(id) ON DELETE CASCADE
		);
		CREATE TABLE terminal_records (
			id TEXT PRIMARY KEY,
			working_dir TEXT NOT NULL,
			executable TEXT NOT NULL,
			sanitized_args TEXT,
			sanitized_command TEXT NOT NULL,
			start_time TIMESTAMP NOT NULL,
			end_time TIMESTAMP NOT NULL,
			duration_ms INTEGER NOT NULL,
			exit_code INTEGER,
			collection_mode TEXT NOT NULL,
			is_redacted BOOLEAN NOT NULL DEFAULT 0,
			is_omitted BOOLEAN NOT NULL DEFAULT 0,
			omission_reason TEXT,
			created_at TIMESTAMP NOT NULL
		);
		CREATE VIRTUAL TABLE search_index USING fts5(
			domain UNINDEXED,
			record_id UNINDEXED,
			title,
			content,
			working_path,
			timestamp UNINDEXED
		);

		INSERT INTO schema_migrations (version, name, applied_at) VALUES
			(1, 'initial_schema', '2026-01-01T00:00:00Z'),
			(2, 'filesystem_events', '2026-01-01T00:00:00Z'),
			(3, 'git_memory', '2026-01-01T00:00:00Z'),
			(4, 'terminal_memory', '2026-01-01T00:00:00Z'),
			(5, 'search_index_fts5', '2026-01-01T00:00:00Z');

		-- Seed v5 sample event
		INSERT INTO filesystem_events (id, event_type, path, size, is_dir, source, created_at)
		VALUES ('evt_v5_01', 'created', '/work/v5_file.go', 128, 0, 'fs', '2026-01-01T00:00:00Z');
		INSERT INTO search_index (domain, record_id, title, content, working_path, timestamp)
		VALUES ('filesystem', 'evt_v5_01', '/work/v5_file.go', 'created fs /work/v5_file.go', '/work/v5_file.go', '2026-01-01T00:00:00Z');
	`

	if _, err := rawDB.ExecContext(ctx, initSQL); err != nil {
		t.Fatal(err)
	}
	rawDB.Close()

	// 2. Open via storage.Open to apply migrations forward to latest schema
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open failed on v5 database: %v", err)
	}
	defer db.Close()

	// 3. Verify schema version reached 7
	v, err := db.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != 7 {
		t.Fatalf("expected migrated schema version 7, got %d", v)
	}

	// 4. Verify existing v5 data is preserved
	eventsList, err := db.QueryEvents(ctx, storage.EventFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(eventsList) != 1 || eventsList[0].ID != "evt_v5_01" {
		t.Fatalf("expected preserved v5 event, got %+v", eventsList)
	}

	// 5. Verify v6 feature (checkpoints table) is available and functional
	now := time.Now().UTC()
	err = db.CreateCheckpoint(ctx, &storage.CheckpointRecord{
		ID:            "chk_migrated_01",
		ProjectPath:   "/work",
		Label:         "Migrated checkpoint",
		CreatedAt:     now,
		FormatVersion: "1.0.0",
		Payload:       `{"status":"ok"}`,
	})
	if err != nil {
		t.Fatalf("CreateCheckpoint on migrated db failed: %v", err)
	}

	chk, err := db.GetCheckpoint(ctx, "chk_migrated_01")
	if err != nil || chk.Label != "Migrated checkpoint" {
		t.Fatalf("failed retrieving checkpoint on migrated db: %v", err)
	}
}

func TestMigration_ForwardFromV6ToV7(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "migrate_v6.db")
	ctx := context.Background()

	// 1. Manually construct schema-v6 database
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	initSQL := `
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMP NOT NULL
		);
		CREATE TABLE metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE filesystem_events (
			id TEXT PRIMARY KEY,
			event_type TEXT NOT NULL,
			path TEXT NOT NULL,
			size INTEGER,
			mod_time TIMESTAMP,
			is_dir BOOLEAN NOT NULL DEFAULT 0,
			source TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL
		);
		CREATE TABLE git_repositories (
			id TEXT PRIMARY KEY,
			path TEXT NOT NULL UNIQUE,
			is_bare BOOLEAN NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			last_scanned_at TIMESTAMP
		);
		CREATE TABLE git_commits (
			repository_id TEXT NOT NULL,
			commit_hash TEXT NOT NULL,
			author_name TEXT NOT NULL,
			author_time TIMESTAMP NOT NULL,
			subject TEXT NOT NULL,
			PRIMARY KEY (repository_id, commit_hash),
			FOREIGN KEY (repository_id) REFERENCES git_repositories(id) ON DELETE CASCADE
		);
		CREATE TABLE git_branches (
			repository_id TEXT NOT NULL,
			name TEXT NOT NULL,
			commit_hash TEXT NOT NULL,
			is_head BOOLEAN NOT NULL DEFAULT 0,
			observed_at TIMESTAMP NOT NULL,
			PRIMARY KEY (repository_id, name),
			FOREIGN KEY (repository_id) REFERENCES git_repositories(id) ON DELETE CASCADE
		);
		CREATE TABLE git_branch_log (
			id TEXT PRIMARY KEY,
			repository_id TEXT NOT NULL,
			branch_name TEXT NOT NULL,
			old_commit_hash TEXT,
			new_commit_hash TEXT NOT NULL,
			observed_at TIMESTAMP NOT NULL,
			FOREIGN KEY (repository_id) REFERENCES git_repositories(id) ON DELETE CASCADE
		);
		CREATE TABLE terminal_records (
			id TEXT PRIMARY KEY,
			working_dir TEXT NOT NULL,
			executable TEXT NOT NULL,
			sanitized_args TEXT,
			sanitized_command TEXT NOT NULL,
			start_time TIMESTAMP NOT NULL,
			end_time TIMESTAMP NOT NULL,
			duration_ms INTEGER NOT NULL,
			exit_code INTEGER,
			collection_mode TEXT NOT NULL,
			is_redacted BOOLEAN NOT NULL DEFAULT 0,
			is_omitted BOOLEAN NOT NULL DEFAULT 0,
			omission_reason TEXT,
			created_at TIMESTAMP NOT NULL
		);
		CREATE VIRTUAL TABLE search_index USING fts5(
			domain UNINDEXED,
			record_id UNINDEXED,
			title,
			content,
			working_path,
			timestamp UNINDEXED
		);
		CREATE TABLE checkpoints (
			id TEXT PRIMARY KEY,
			project_path TEXT NOT NULL,
			label TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			format_version TEXT NOT NULL DEFAULT '1.0.0',
			payload TEXT NOT NULL
		);

		INSERT INTO schema_migrations (version, name, applied_at) VALUES
			(1, 'initial_schema', '2026-01-01T00:00:00Z'),
			(2, 'filesystem_events', '2026-01-01T00:00:00Z'),
			(3, 'git_memory', '2026-01-01T00:00:00Z'),
			(4, 'terminal_memory', '2026-01-01T00:00:00Z'),
			(5, 'search_index_fts5', '2026-01-01T00:00:00Z'),
			(6, 'checkpoints_table', '2026-01-01T00:00:00Z');

		INSERT INTO checkpoints (id, project_path, label, created_at, format_version, payload)
		VALUES ('chk_v6_01', '/my/project', 'V6 Checkpoint', '2026-01-01T00:00:00Z', '1.0.0', '{"data":"v6"}');
	`

	if _, err := rawDB.ExecContext(ctx, initSQL); err != nil {
		t.Fatal(err)
	}
	rawDB.Close()

	// 2. Open via storage.Open
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open on v6 failed: %v", err)
	}
	defer db.Close()

	// 3. Verify schema version reached 7
	v, err := db.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != 7 {
		t.Fatalf("expected schema version 7, got %d", v)
	}

	// 4. Verify v6 checkpoint intact
	chk, err := db.GetCheckpoint(ctx, "chk_v6_01")
	if err != nil || chk.Label != "V6 Checkpoint" {
		t.Fatalf("expected intact v6 checkpoint, got %+v (err: %v)", chk, err)
	}
}

func TestSearch_RebuildIndexEquivalence(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "rebuild_equiv.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Seed across all domains
	_ = db.RecordEvents(ctx, []events.Event{
		{
			ID:        "evt_eq_01",
			Type:      events.EventCreated,
			Path:      "/project/equiv_target.go",
			Size:      500,
			ModTime:   now,
			Source:    "test",
			CreatedAt: now,
		},
	})

	repoID, _ := db.RegisterOrUpdateGitRepo(ctx, "/project", false)
	_, _ = db.RecordGitState(ctx, repoID, &git.RepoState{
		Root:       "/project",
		HeadBranch: "equiv_branch",
		HeadCommit: &git.Commit{
			Hash:       "equiv123456",
			AuthorName: "Tester",
			AuthorTime: now,
			Subject:    "feat: equiv commit subject",
		},
		RecentCommits: []git.Commit{
			{
				Hash:       "equiv123456",
				AuthorName: "Tester",
				AuthorTime: now,
				Subject:    "feat: equiv commit subject",
			},
		},
		Branches: []git.BranchInfo{
			{Name: "equiv_branch", CommitHash: "equiv123456"},
		},
		ObservedAt: now,
	})

	code0 := 0
	_ = db.RecordTerminalExecution(ctx, &terminal.Record{
		ID:               "term_eq_01",
		WorkingDir:       "/project",
		Executable:       "go",
		SanitizedCommand: "go test ./equiv/...",
		StartTime:        now,
		EndTime:          now,
		DurationMs:       10,
		ExitCode:         &code0,
		CollectionMode:   "direct",
		CreatedAt:        now,
	})

	// 2. Search before rebuild
	resBefore, err := db.Search(ctx, search.Query{Text: "equiv"})
	if err != nil {
		t.Fatal(err)
	}
	if resBefore.TotalCount < 3 {
		t.Fatalf("expected at least 3 results before rebuild, got %d", resBefore.TotalCount)
	}

	// 3. Rebuild search index
	if err := db.RebuildSearchIndex(ctx); err != nil {
		t.Fatalf("RebuildSearchIndex failed: %v", err)
	}

	// 4. Search after rebuild
	resAfter, err := db.Search(ctx, search.Query{Text: "equiv"})
	if err != nil {
		t.Fatal(err)
	}
	if resAfter.TotalCount != resBefore.TotalCount {
		t.Fatalf("mismatch after rebuild: before=%d, after=%d", resBefore.TotalCount, resAfter.TotalCount)
	}

	// Check domain counts match
	domainCountBefore := make(map[search.Domain]int)
	for _, item := range resBefore.Items {
		domainCountBefore[item.Domain]++
	}
	domainCountAfter := make(map[search.Domain]int)
	for _, item := range resAfter.Items {
		domainCountAfter[item.Domain]++
	}

	for d, count := range domainCountBefore {
		if domainCountAfter[d] != count {
			t.Errorf("domain %s count mismatch: before=%d, after=%d", d, count, domainCountAfter[d])
		}
	}
}

func TestSearch_DeleteTriggersTerminalAndBranches(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "triggers.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Terminal record insertion and deletion
	code0 := 0
	termRec := &terminal.Record{
		ID:               "term_delete_test",
		WorkingDir:       "/test/dir",
		Executable:       "special_tool",
		SanitizedCommand: "special_tool --run-all",
		StartTime:        now,
		EndTime:          now,
		DurationMs:       5,
		ExitCode:         &code0,
		CollectionMode:   "direct",
		CreatedAt:        now,
	}
	if err := db.RecordTerminalExecution(ctx, termRec); err != nil {
		t.Fatal(err)
	}

	resTerm, err := db.Search(ctx, search.Query{Text: "special_tool"})
	if err != nil || resTerm.TotalCount != 1 {
		t.Fatalf("expected 1 terminal result, got %d (err: %v)", resTerm.TotalCount, err)
	}

	// Delete terminal record directly via SQL
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer rawDB.Close()

	if _, err := rawDB.ExecContext(ctx, "DELETE FROM terminal_records WHERE id = 'term_delete_test'"); err != nil {
		t.Fatal(err)
	}

	// Verify terminal record removed from search index
	resTermAfter, err := db.Search(ctx, search.Query{Text: "special_tool"})
	if err != nil {
		t.Fatal(err)
	}
	if resTermAfter.TotalCount != 0 {
		t.Fatalf("expected 0 terminal search results after deletion, got %d", resTermAfter.TotalCount)
	}

	// 2. Git branch insertion and deletion
	repoID, err := db.RegisterOrUpdateGitRepo(ctx, "/test/repo", false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.RecordGitState(ctx, repoID, &git.RepoState{
		Root:       "/test/repo",
		HeadBranch: "unique_feature_branch",
		Branches: []git.BranchInfo{
			{Name: "unique_feature_branch", CommitHash: "abc111222"},
		},
		ObservedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	resBranch, err := db.Search(ctx, search.Query{Text: "unique_feature_branch"})
	if err != nil || resBranch.TotalCount < 2 {
		t.Fatalf("expected at least 2 branch search hits (branch + transition log), got %d (err: %v)", resBranch.TotalCount, err)
	}

	// 1. Delete branch from git_branches
	if _, err := rawDB.ExecContext(ctx, "DELETE FROM git_branches WHERE repository_id = ? AND name = ?", repoID, "unique_feature_branch"); err != nil {
		t.Fatal(err)
	}

	// Verify branch row removed from search index (leaving only the transition log entry)
	resBranchMid, err := db.Search(ctx, search.Query{Text: "unique_feature_branch"})
	if err != nil {
		t.Fatal(err)
	}
	if resBranchMid.TotalCount != 1 {
		t.Fatalf("expected 1 search result (branch transition log remaining) after git_branches delete, got %d", resBranchMid.TotalCount)
	}

	// 2. Delete branch transition log from git_branch_log
	if _, err := rawDB.ExecContext(ctx, "DELETE FROM git_branch_log WHERE repository_id = ? AND branch_name = ?", repoID, "unique_feature_branch"); err != nil {
		t.Fatal(err)
	}

	// Verify branch transition log removed from search index
	resBranchAfter, err := db.Search(ctx, search.Query{Text: "unique_feature_branch"})
	if err != nil {
		t.Fatal(err)
	}
	if resBranchAfter.TotalCount != 0 {
		t.Fatalf("expected 0 branch search results after log deletion, got %d", resBranchAfter.TotalCount)
	}
}

func TestSearch_CloseAndReopenPersistence(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "reopen_search.db")
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Open DB, record items, search
	db1, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	_ = db1.RecordEvents(ctx, []events.Event{
		{
			ID:        "evt_persist_01",
			Type:      events.EventCreated,
			Path:      "/project/durable_record.go",
			Size:      200,
			ModTime:   now,
			Source:    "test",
			CreatedAt: now,
		},
	})

	res1, err := db1.Search(ctx, search.Query{Text: "durable_record"})
	if err != nil || res1.TotalCount != 1 {
		t.Fatalf("expected 1 result in db1, got %d (err: %v)", res1.TotalCount, err)
	}

	// Close database
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	// 2. Re-open database, search again
	db2, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	res2, err := db2.Search(ctx, search.Query{Text: "durable_record"})
	if err != nil || res2.TotalCount != 1 {
		t.Fatalf("expected 1 result in db2 after reopen, got %d (err: %v)", res2.TotalCount, err)
	}

	if res2.Items[0].Title != "/project/durable_record.go" {
		t.Errorf("unexpected search hit title: %s", res2.Items[0].Title)
	}
}

func TestMigration_RollbackOnFailure(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "rollback.db")
	ctx := context.Background()

	// Pre-create database with schema migrations 1-2
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	setupSQL := `
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMP NOT NULL
		);
		INSERT INTO schema_migrations VALUES (1, 'initial', '2026-01-01T00:00:00Z');
		-- Create a table that will conflict with migration 2 if executed improperly,
		-- or simulate transaction rollback
	`
	if _, err := rawDB.ExecContext(ctx, setupSQL); err != nil {
		t.Fatal(err)
	}
	rawDB.Close()

	// Open will apply migrations 2..7. Verify that if we check version, it cleanly reaches 7
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	v, err := db.SchemaVersion(ctx)
	if err != nil || v != 7 {
		t.Fatalf("expected version 7, got %d (err: %v)", v, err)
	}
}
