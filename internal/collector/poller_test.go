package collector_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/collector"
	"github.com/lifelog/lifelog/internal/events"
)

func TestNewFSPoller_Validation(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "lifelog_data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		t.Fatal(err)
	}

	// 1. Empty root
	if _, err := collector.NewFSPoller(collector.PollerConfig{Root: ""}); err == nil {
		t.Error("expected error for empty root, got nil")
	}

	// 2. Non-existent root
	if _, err := collector.NewFSPoller(collector.PollerConfig{Root: filepath.Join(tempDir, "does_not_exist")}); err == nil {
		t.Error("expected error for non-existent root, got nil")
	}

	// 3. Regular file as root
	filePath := filepath.Join(tempDir, "regular_file.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := collector.NewFSPoller(collector.PollerConfig{Root: filePath}); err == nil {
		t.Error("expected error when root is a file, got nil")
	}

	// 4. Filesystem root /
	if _, err := collector.NewFSPoller(collector.PollerConfig{Root: "/"}); err == nil {
		t.Error("expected error for filesystem root, got nil")
	}

	// 5. User home directory
	home, err := os.UserHomeDir()
	if err == nil {
		if _, err := collector.NewFSPoller(collector.PollerConfig{Root: home}); err == nil {
			t.Error("expected error when root is home directory, got nil")
		}
	}

	// 6. LIFELOG data directory
	if _, err := collector.NewFSPoller(collector.PollerConfig{Root: dataDir, DataDir: dataDir}); err == nil {
		t.Error("expected error when root is LIFELOG data directory, got nil")
	}

	// 7. Valid project directory
	projectDir := filepath.Join(tempDir, "my_project")
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatal(err)
	}
	poller, err := collector.NewFSPoller(collector.PollerConfig{Root: projectDir, DataDir: dataDir})
	if err != nil {
		t.Fatalf("expected valid poller, got error: %v", err)
	}
	if poller.MonitoredRoot() != projectDir {
		t.Errorf("got %q, want %q", poller.MonitoredRoot(), projectDir)
	}
}

func TestFSPoller_DuplicateWatcherRejection(t *testing.T) {
	tempDir := t.TempDir()
	poller1, err := collector.NewFSPoller(collector.PollerConfig{
		Root:     tempDir,
		Interval: 20 * time.Millisecond,
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
		_ = poller1.Start(ctx)
	}()

	// Wait briefly for poller1 to register
	time.Sleep(10 * time.Millisecond)

	// Attempt second poller on same root
	poller2, err := collector.NewFSPoller(collector.PollerConfig{
		Root:     tempDir,
		Interval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = poller2.Start(ctx)
	if err == nil {
		t.Fatal("expected error on duplicate watcher, got nil")
	}
	if !errors.Is(err, collector.ErrAlreadyWatching) {
		t.Errorf("expected ErrAlreadyWatching, got: %v", err)
	}

	cancel()
	wg.Wait()
}

func TestFSPoller_FileLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	watchDir := filepath.Join(tempDir, "watched_project")
	if err := os.MkdirAll(watchDir, 0700); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var collected []events.Event

	poller, err := collector.NewFSPoller(collector.PollerConfig{
		Root:     watchDir,
		Interval: 25 * time.Millisecond,
		OnEvents: func(batch []events.Event) error {
			mu.Lock()
			collected = append(collected, batch...)
			mu.Unlock()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("failed creating poller: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = poller.Start(ctx)
	}()

	// Allow initial scan to complete
	time.Sleep(40 * time.Millisecond)

	// 1. Create file
	targetFile := filepath.Join(watchDir, "test.txt")
	if err := os.WriteFile(targetFile, []byte("initial"), 0600); err != nil {
		t.Fatal(err)
	}

	// Wait for creation event
	waitForEvents(t, &mu, &collected, 1, 2*time.Second)

	mu.Lock()
	if len(collected) < 1 {
		t.Fatalf("expected at least 1 event for creation, got %d", len(collected))
	}
	if collected[0].Type != events.EventCreated || collected[0].Path != targetFile {
		t.Errorf("unexpected creation event: %+v", collected[0])
	}
	mu.Unlock()

	// 2. Modify file
	time.Sleep(50 * time.Millisecond) // Ensure distinct modTime
	if err := os.WriteFile(targetFile, []byte("updated content here"), 0600); err != nil {
		t.Fatal(err)
	}

	// Wait for modification event
	waitForEvents(t, &mu, &collected, 2, 2*time.Second)

	mu.Lock()
	if len(collected) < 2 {
		t.Fatalf("expected at least 2 events for modification, got %d", len(collected))
	}
	if collected[1].Type != events.EventModified || collected[1].Path != targetFile {
		t.Errorf("unexpected modification event: %+v", collected[1])
	}
	mu.Unlock()

	// 3. Delete file
	if err := os.Remove(targetFile); err != nil {
		t.Fatal(err)
	}

	// Wait for deletion event
	waitForEvents(t, &mu, &collected, 3, 2*time.Second)

	mu.Lock()
	if len(collected) < 3 {
		t.Fatalf("expected at least 3 events for deletion, got %d", len(collected))
	}
	if collected[2].Type != events.EventDeleted || collected[2].Path != targetFile {
		t.Errorf("unexpected deletion event: %+v", collected[2])
	}
	mu.Unlock()

	cancel()
	wg.Wait()
}

func TestFSPoller_SymlinksAndDataDirExclusion(t *testing.T) {
	tempDir := t.TempDir()
	watchDir := filepath.Join(tempDir, "watched_project")
	if err := os.MkdirAll(watchDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Create nested LIFELOG data directory inside watched root
	nestedDataDir := filepath.Join(watchDir, ".lifelog_data")
	if err := os.MkdirAll(nestedDataDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Create outside target directory and symlink to it
	outsideDir := filepath.Join(tempDir, "outside_folder")
	if err := os.MkdirAll(outsideDir, 0700); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("outside content"), 0600); err != nil {
		t.Fatal(err)
	}

	// Create symlink inside watchDir pointing to outsideDir
	symlinkPath := filepath.Join(watchDir, "symlink_dir")
	if err := os.Symlink(outsideDir, symlinkPath); err != nil {
		t.Skip("symlinks not supported on this environment")
	}

	var mu sync.Mutex
	var collected []events.Event

	poller, err := collector.NewFSPoller(collector.PollerConfig{
		Root:     watchDir,
		DataDir:  nestedDataDir,
		Interval: 25 * time.Millisecond,
		OnEvents: func(batch []events.Event) error {
			mu.Lock()
			collected = append(collected, batch...)
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

	// Create file inside nestedDataDir: MUST NOT be collected!
	dataFile := filepath.Join(nestedDataDir, "config.json")
	if err := os.WriteFile(dataFile, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}

	// Create normal project file: MUST be collected!
	normalFile := filepath.Join(watchDir, "normal.go")
	if err := os.WriteFile(normalFile, []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}

	waitForEvents(t, &mu, &collected, 1, 2*time.Second)

	mu.Lock()
	for _, e := range collected {
		if e.Path == dataFile || filepath.Dir(e.Path) == nestedDataDir {
			t.Errorf("data directory file %s was unexpectedly collected", e.Path)
		}
		if e.Path == outsideFile {
			t.Errorf("symlink target file %s was unexpectedly collected", e.Path)
		}
	}
	mu.Unlock()

	cancel()
	wg.Wait()
}

func waitForEvents(t *testing.T, mu *sync.Mutex, list *[]events.Event, count int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(*list)
		mu.Unlock()
		if n >= count {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("timed out waiting for %d events, current count: %d", count, len(*list))
}
