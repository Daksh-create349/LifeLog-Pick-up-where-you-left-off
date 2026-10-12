package storage_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/search"
	"github.com/lifelog/lifelog/internal/storage"
)

func TestStorage_SystemEvents_CRUDAndSummary(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_system.db")
	ctx := context.Background()

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Verify schema reached version 9
	ver, err := db.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion failed: %v", err)
	}
	if ver != 9 {
		t.Fatalf("expected schema version 9, got %d", ver)
	}

	base := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)

	// 1. Record system events: sleep, wake, lock, unlock
	evts := []*storage.SystemEvent{
		{ID: "sys_1", EventType: storage.SystemEventSleep, Timestamp: base},
		{ID: "sys_2", EventType: storage.SystemEventWake, Timestamp: base.Add(5 * time.Minute)},
		{ID: "sys_3", EventType: storage.SystemEventLock, Timestamp: base.Add(15 * time.Minute)},
		{ID: "sys_4", EventType: storage.SystemEventUnlock, Timestamp: base.Add(20 * time.Minute)},
	}

	for _, e := range evts {
		if err := db.RecordSystemEvent(ctx, e); err != nil {
			t.Fatalf("RecordSystemEvent failed for %s: %v", e.ID, err)
		}
	}

	// 2. Query system events
	res, err := db.GetSystemEvents(ctx, storage.SystemEventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("GetSystemEvents failed: %v", err)
	}
	if len(res) != 4 {
		t.Fatalf("expected 4 events, got %d", len(res))
	}
	// Verify reverse-chronological order (sys_4 newest)
	if res[0].ID != "sys_4" || res[0].EventType != storage.SystemEventUnlock {
		t.Errorf("expected newest event sys_4 unlock, got %+v", res[0])
	}

	// 3. System activity summary
	summary, err := db.GetSystemActivitySummary(ctx, base.Add(-time.Hour), base.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetSystemActivitySummary failed: %v", err)
	}
	if summary.TotalEvents != 4 {
		t.Errorf("expected 4 total events, got %d", summary.TotalEvents)
	}
	if summary.SleepCount != 1 || summary.WakeCount != 1 || summary.LockCount != 1 || summary.UnlockCount != 1 {
		t.Errorf("unexpected counts in summary: %+v", summary)
	}

	// 4. Search integration for system domain
	searchRes, err := db.Search(ctx, search.Query{Text: "sleep", Domain: search.DomainSystem})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if searchRes.TotalCount < 1 {
		t.Errorf("expected search match for 'sleep' in system domain, got %d", searchRes.TotalCount)
	}
}

func TestStorage_BrowserDomains(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_browser.db")
	ctx := context.Background()

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()

	// Record visits to domain (with sanitized input like scheme/path)
	if err := db.RecordBrowserDomain(ctx, "https://github.com/features/actions", now); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordBrowserDomain(ctx, "github.com", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordBrowserDomain(ctx, "news.ycombinator.com:443", now); err != nil {
		t.Fatal(err)
	}

	domains, err := db.GetBrowserDomains(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 2 {
		t.Fatalf("expected 2 unique domains, got %d", len(domains))
	}
	if domains[0].Domain != "github.com" || domains[0].VisitCount != 2 {
		t.Errorf("expected github.com with 2 visits, got %+v", domains[0])
	}
}

func TestStorage_RetentionAndDataManagement(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_mgmt.db")
	ctx := context.Background()

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	oldTime := time.Now().UTC().AddDate(0, 0, -45)   // 45 days ago
	recentTime := time.Now().UTC().AddDate(0, 0, -5) // 5 days ago

	// Add old and recent app sessions
	_ = db.RecordAppSession(ctx, &storage.AppSession{
		ID:          "old_app",
		AppBundleID: "com.apple.Safari",
		AppName:     "Safari",
		StartTime:   oldTime,
		EndTime:     oldTime.Add(10 * time.Minute),
		DurationMs:  600000,
		IsActive:    false,
		CreatedAt:   oldTime,
	})
	_ = db.RecordAppSession(ctx, &storage.AppSession{
		ID:          "recent_app",
		AppBundleID: "com.apple.Safari",
		AppName:     "Safari",
		StartTime:   recentTime,
		EndTime:     recentTime.Add(10 * time.Minute),
		DurationMs:  600000,
		IsActive:    false,
		CreatedAt:   recentTime,
	})

	// Add old and recent system events
	_ = db.RecordSystemEvent(ctx, &storage.SystemEvent{
		ID:        "old_sys",
		EventType: storage.SystemEventSleep,
		Timestamp: oldTime,
	})
	_ = db.RecordSystemEvent(ctx, &storage.SystemEvent{
		ID:        "recent_sys",
		EventType: storage.SystemEventWake,
		Timestamp: recentTime,
	})

	// Verify DataStats
	stats, err := db.GetDataStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.AppSessionsCount != 2 || stats.SystemEventsCount != 2 {
		t.Errorf("expected 2 app sessions and 2 sys events, got %+v", stats)
	}
	if stats.DBSizeBytes <= 0 {
		t.Errorf("expected positive db size, got %d", stats.DBSizeBytes)
	}

	// Run retention purge with 30-day retention
	purgeStats, err := db.PurgeRetention(ctx, 30)
	if err != nil {
		t.Fatalf("PurgeRetention failed: %v", err)
	}
	if purgeStats.AppSessionsPurged != 1 || purgeStats.SystemEventsPurged != 1 {
		t.Errorf("expected 1 app session and 1 sys event purged, got %+v", purgeStats)
	}

	// Verify old records are gone and recent records remain
	statsAfter, err := db.GetDataStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if statsAfter.AppSessionsCount != 1 || statsAfter.SystemEventsCount != 1 {
		t.Errorf("expected 1 app session and 1 sys event remaining, got %+v", statsAfter)
	}
	if statsAfter.LastRetentionCleanup == nil {
		t.Error("expected LastRetentionCleanup to be recorded")
	}

	// Verify ExportUserData
	export, err := db.ExportUserData(ctx)
	if err != nil {
		t.Fatalf("ExportUserData failed: %v", err)
	}
	if export.ExportVersion != "1.0" {
		t.Errorf("expected export version 1.0, got %s", export.ExportVersion)
	}
	if len(export.AppSessions) != 1 || export.AppSessions[0].ID != "recent_app" {
		t.Errorf("unexpected export app sessions: %+v", export.AppSessions)
	}

	// Verify Selective DeleteActivity
	delCount, err := db.DeleteActivity(ctx, "apps", nil, nil)
	if err != nil {
		t.Fatalf("DeleteActivity failed: %v", err)
	}
	if delCount != 1 {
		t.Errorf("expected 1 deleted app session, got %d", delCount)
	}

	statsFinal, err := db.GetDataStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if statsFinal.AppSessionsCount != 0 {
		t.Errorf("expected 0 app sessions after delete, got %d", statsFinal.AppSessionsCount)
	}
}
