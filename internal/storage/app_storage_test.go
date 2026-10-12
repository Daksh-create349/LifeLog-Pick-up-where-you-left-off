package storage_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/search"
	"github.com/lifelog/lifelog/internal/storage"
)

func TestStorage_AppSessions_CRUDAndAggregation(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_app_storage.db")
	ctx := context.Background()

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Verify schema reached version 8
	ver, err := db.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion failed: %v", err)
	}
	if ver != 9 {
		t.Fatalf("expected schema version 9, got %d", ver)
	}

	baseTime := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

	// 1. Insert session 1: VS Code (active)
	sess1 := &storage.AppSession{
		ID:          "app_sess_01",
		AppBundleID: "com.microsoft.VSCode",
		AppName:     "Code",
		StartTime:   baseTime,
		EndTime:     baseTime,
		DurationMs:  0,
		IsActive:    true,
		CreatedAt:   baseTime,
	}
	if err := db.RecordAppSession(ctx, sess1); err != nil {
		t.Fatalf("RecordAppSession failed: %v", err)
	}

	// Active session check
	active, err := db.GetActiveAppSession(ctx)
	if err != nil {
		t.Fatalf("GetActiveAppSession failed: %v", err)
	}
	if active == nil || active.ID != "app_sess_01" || !active.IsActive {
		t.Fatalf("expected active session app_sess_01, got %+v", active)
	}

	// 2. Update session 1 when switching: 15 minutes of VS Code
	sess1End := baseTime.Add(15 * time.Minute)
	sess1Dur := int64(15 * 60 * 1000)
	if err := db.UpdateAppSession(ctx, "app_sess_01", sess1End, sess1Dur, false); err != nil {
		t.Fatalf("UpdateAppSession failed: %v", err)
	}

	// 3. Insert session 2: Terminal (5 minutes)
	sess2 := &storage.AppSession{
		ID:          "app_sess_02",
		AppBundleID: "com.apple.Terminal",
		AppName:     "Terminal",
		StartTime:   sess1End,
		EndTime:     sess1End.Add(5 * time.Minute),
		DurationMs:  5 * 60 * 1000,
		IsActive:    false,
		CreatedAt:   sess1End,
	}
	if err := db.RecordAppSession(ctx, sess2); err != nil {
		t.Fatalf("RecordAppSession sess2 failed: %v", err)
	}

	// 4. Insert session 3: VS Code again (10 minutes)
	sess3Start := sess1End.Add(5 * time.Minute)
	sess3 := &storage.AppSession{
		ID:          "app_sess_03",
		AppBundleID: "com.microsoft.VSCode",
		AppName:     "Code",
		StartTime:   sess3Start,
		EndTime:     sess3Start.Add(10 * time.Minute),
		DurationMs:  10 * 60 * 1000,
		IsActive:    false,
		CreatedAt:   sess3Start,
	}
	if err := db.RecordAppSession(ctx, sess3); err != nil {
		t.Fatalf("RecordAppSession sess3 failed: %v", err)
	}

	// 5. Test GetAppUsage aggregation
	since := baseTime.Add(-1 * time.Hour)
	until := baseTime.Add(2 * time.Hour)
	usage, err := db.GetAppUsage(ctx, storage.AppUsageFilter{
		Since: &since,
		Until: &until,
	})
	if err != nil {
		t.Fatalf("GetAppUsage failed: %v", err)
	}

	expectedTotal := int64((15 + 5 + 10) * 60 * 1000) // 30 minutes in ms = 1,800,000
	if usage.TotalDurationMs != expectedTotal {
		t.Errorf("expected total duration %d ms, got %d", expectedTotal, usage.TotalDurationMs)
	}
	if usage.TotalSessions != 3 {
		t.Errorf("expected 3 sessions, got %d", usage.TotalSessions)
	}
	if len(usage.Apps) != 2 {
		t.Fatalf("expected 2 distinct apps, got %d", len(usage.Apps))
	}

	// VS Code should be #1 with 25 minutes (1,500,000 ms) and ~83.33%
	codeApp := usage.Apps[0]
	if codeApp.AppName != "Code" || codeApp.SessionCount != 2 {
		t.Errorf("expected Code app at index 0 with 2 sessions, got %+v", codeApp)
	}
	expectedCodeDur := int64(25 * 60 * 1000)
	if codeApp.TotalDurationMs != expectedCodeDur {
		t.Errorf("expected Code duration %d, got %d", expectedCodeDur, codeApp.TotalDurationMs)
	}
	if codeApp.Percentage < 83.0 || codeApp.Percentage > 84.0 {
		t.Errorf("expected ~83.33%%, got %f", codeApp.Percentage)
	}

	// 6. Test GetAppSessions with pagination and filter
	sessions, count, err := db.GetAppSessions(ctx, storage.AppSessionFilter{
		BundleID: "com.microsoft.VSCode",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("GetAppSessions failed: %v", err)
	}
	if count != 2 || len(sessions) != 2 {
		t.Errorf("expected 2 VSCode sessions, got count=%d len=%d", count, len(sessions))
	}

	// 7. Test FTS search on app domain
	searchResults, err := db.Search(ctx, search.Query{
		Text:   "Code",
		Domain: "app",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("Search on app domain failed: %v", err)
	}
	if len(searchResults.Items) != 2 {
		t.Errorf("expected 2 search results in domain 'app' for 'Code', got %d", len(searchResults.Items))
	}
}

func TestStorage_AppSessions_CrashRecovery(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_app_recovery.db")
	ctx := context.Background()

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	start := time.Now().UTC().Add(-10 * time.Minute)
	// Create an orphan session where daemon crashed while is_active was 1
	orphan := &storage.AppSession{
		ID:          "app_sess_orphan",
		AppBundleID: "com.apple.Safari",
		AppName:     "Safari",
		StartTime:   start,
		EndTime:     start, // zero elapsed
		DurationMs:  0,
		IsActive:    true,
		CreatedAt:   start,
	}
	if err := db.RecordAppSession(ctx, orphan); err != nil {
		t.Fatalf("RecordAppSession failed: %v", err)
	}

	// Reconcile open sessions on startup
	closed, err := db.CloseOpenAppSessions(ctx, 30*time.Second)
	if err != nil {
		t.Fatalf("CloseOpenAppSessions failed: %v", err)
	}
	if closed != 1 {
		t.Errorf("expected 1 session closed, got %d", closed)
	}

	// Active session should now be nil
	active, err := db.GetActiveAppSession(ctx)
	if err != nil {
		t.Fatalf("GetActiveAppSession failed: %v", err)
	}
	if active != nil {
		t.Errorf("expected no active session, got %+v", active)
	}

	// Query closed orphan
	sessions, _, err := db.GetAppSessions(ctx, storage.AppSessionFilter{BundleID: "com.apple.Safari"})
	if err != nil {
		t.Fatalf("GetAppSessions failed: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].IsActive {
		t.Errorf("expected is_active = false")
	}
	if sessions[0].DurationMs != 30000 {
		t.Errorf("expected capped fallback duration 30000 ms, got %d", sessions[0].DurationMs)
	}
}

func TestStorage_MigrationFromSchemaV7ToV8(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "migrate_v7_v8.db")
	ctx := context.Background()

	// 1. Setup a database with schema version 7
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}

	// Create tables up to v7
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
			size INTEGER NOT NULL DEFAULT 0,
			mod_time TIMESTAMP NOT NULL,
			is_dir BOOLEAN NOT NULL DEFAULT 0,
			source TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL
		);
		CREATE TABLE checkpoints (
			id TEXT PRIMARY KEY,
			project_path TEXT NOT NULL,
			label TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			format_version TEXT NOT NULL DEFAULT '1.0.0',
			payload TEXT NOT NULL
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
			(1, 'initial_metadata_table', '2026-10-12T00:00:00Z'),
			(2, 'filesystem_events_table', '2026-10-12T00:00:00Z'),
			(3, 'git_memory_tables', '2026-10-12T00:00:00Z'),
			(4, 'terminal_records_table', '2026-10-12T00:00:00Z'),
			(5, 'search_index_fts5', '2026-10-12T00:00:00Z'),
			(6, 'checkpoints_table', '2026-10-12T00:00:00Z'),
			(7, 'fix_search_delete_triggers', '2026-10-12T00:00:00Z');

		INSERT INTO filesystem_events (id, event_type, path, size, mod_time, is_dir, source, created_at)
		VALUES ('evt_v7_01', 'created', '/path/test.txt', 12, '2026-10-12T00:00:00Z', 0, 'fs_poll', '2026-10-12T00:00:00Z');

		INSERT INTO checkpoints (id, project_path, label, created_at, payload)
		VALUES ('chk_v7_01', '/path', 'V7 Checkpoint', '2026-10-12T00:00:00Z', '{}');
	`
	if _, err := rawDB.ExecContext(ctx, initSQL); err != nil {
		rawDB.Close()
		t.Fatalf("executing v7 schema failed: %v", err)
	}
	rawDB.Close()

	// 2. Open via storage.Open to trigger automatic forward migration to v8
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open on v7 database failed: %v", err)
	}
	defer db.Close()

	v, err := db.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != 9 {
		t.Fatalf("expected schema version 9 after migration, got %d", v)
	}

	// 3. Verify existing v7 data is 100% intact
	evts, err := db.QueryEvents(ctx, storage.EventFilter{Limit: 10})
	if err != nil || len(evts) != 1 || evts[0].ID != "evt_v7_01" {
		t.Fatalf("expected preserved v7 event, got %+v (err: %v)", evts, err)
	}

	chk, err := db.GetCheckpoint(ctx, "chk_v7_01")
	if err != nil || chk.Label != "V7 Checkpoint" {
		t.Fatalf("expected preserved v7 checkpoint, got %+v (err: %v)", chk, err)
	}

	// 4. Verify new v8 app_sessions table is fully operational
	newSess := &storage.AppSession{
		ID:          "app_migrated_01",
		AppBundleID: "com.apple.dt.Xcode",
		AppName:     "Xcode",
		StartTime:   time.Now().UTC(),
		EndTime:     time.Now().UTC().Add(5 * time.Minute),
		DurationMs:  300000,
		IsActive:    false,
		CreatedAt:   time.Now().UTC(),
	}
	if err := db.RecordAppSession(ctx, newSess); err != nil {
		t.Fatalf("RecordAppSession on migrated db failed: %v", err)
	}
}

func BenchmarkStorage_AppSessionTransition(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "bench_app.db")
	ctx := context.Background()

	db, err := storage.Open(dbPath)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sess := &storage.AppSession{
			ID:          "app_sess_bench",
			AppBundleID: "com.apple.Terminal",
			AppName:     "Terminal",
			StartTime:   now,
			EndTime:     now.Add(time.Second),
			DurationMs:  1000,
			IsActive:    false,
			CreatedAt:   now,
		}
		sess.ID = "app_sess_bench_" + strconv.Itoa(i)
		if err := db.RecordAppSession(ctx, sess); err != nil {
			b.Fatal(err)
		}
	}
}
