package storage_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/search"
	"github.com/lifelog/lifelog/internal/storage"
)

func BenchmarkStorage_SearchFTS(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "bench_search.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// Seed 200 events
	var batch []events.Event
	for i := 0; i < 200; i++ {
		batch = append(batch, events.Event{
			ID:        fmt.Sprintf("evt_bench_%d", i),
			Type:      events.EventCreated,
			Path:      fmt.Sprintf("/bench/project/file_%d.go", i),
			Size:      int64(i * 10),
			ModTime:   now,
			Source:    "bench",
			CreatedAt: now,
		})
	}
	if err := db.RecordEvents(ctx, batch); err != nil {
		b.Fatal(err)
	}

	query := search.Query{
		Text:  "file_50",
		Limit: 20,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := db.Search(ctx, query)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStorage_DiscoverProjects(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "bench_proj.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	// Register 10 repositories
	for i := 0; i < 10; i++ {
		p := filepath.Join(tempDir, fmt.Sprintf("proj_%d", i))
		_, _ = db.RegisterOrUpdateGitRepo(ctx, p, false)
	}

	opts := storage.ProjectOverviewOptions{
		SortMode: storage.SortRecent,
		Limit:    50,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := db.DiscoverProjects(ctx, opts)
		if err != nil {
			b.Fatal(err)
		}
	}
}
