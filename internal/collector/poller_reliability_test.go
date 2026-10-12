package collector_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/collector"
	"github.com/lifelog/lifelog/internal/events"
)

func TestFSPoller_TransientInaccessibleDirectory_NoMassDeletion(t *testing.T) {
	tempDir := t.TempDir()
	watchDir := filepath.Join(tempDir, "watched")
	subDir := filepath.Join(watchDir, "locked_dir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create 5 files in subDir
	for i := 0; i < 5; i++ {
		filePath := filepath.Join(subDir, "file.txt")
		if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	var collected []events.Event

	poller, err := collector.NewFSPoller(collector.PollerConfig{
		Root:           watchDir,
		Interval:       50 * time.Millisecond,
		RecordBaseline: true, // Baseline captures the files
		OnEvents: func(batch []events.Event) error {
			mu.Lock()
			collected = append(collected, batch...)
			mu.Unlock()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewFSPoller error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- poller.Start(ctx)
	}()

	// Wait for baseline to be recorded
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	initialCount := len(collected)
	mu.Unlock()
	if initialCount == 0 {
		t.Fatalf("expected initial baseline events")
	}

	// Make subDir inaccessible (chmod 0000)
	if err := os.Chmod(subDir, 0000); err != nil {
		t.Skipf("skipping permission test: chmod not supported on this filesystem: %v", err)
	}

	// Reset collected events to check what diff emits during locked state
	mu.Lock()
	collected = nil
	mu.Unlock()

	// Wait for a few poll cycles while inaccessible
	time.Sleep(200 * time.Millisecond)

	// Restore permissions before asserting
	_ = os.Chmod(subDir, 0755)

	mu.Lock()
	eventsWhileInaccessible := append([]events.Event(nil), collected...)
	mu.Unlock()

	// Check that NO mass deletion events were emitted for the files inside locked_dir
	for _, e := range eventsWhileInaccessible {
		if e.Type == events.EventDeleted && filepath.Dir(e.Path) == subDir {
			t.Errorf("spurious EventDeleted emitted for inaccessible directory file: %s", e.Path)
		}
	}

	cancel()
	<-errCh
}

func TestFSPoller_BaselineEmission_ConfigRespect(t *testing.T) {
	tempDir := t.TempDir()
	watchDir := filepath.Join(tempDir, "watched")
	if err := os.MkdirAll(watchDir, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(watchDir, "existing.txt"), []byte("data"), 0644)

	var mu sync.Mutex
	var collected []events.Event

	// 1. RecordBaseline: false -> should NOT emit existing.txt
	poller, err := collector.NewFSPoller(collector.PollerConfig{
		Root:           watchDir,
		Interval:       50 * time.Millisecond,
		RecordBaseline: false,
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
	go func() { _ = poller.Start(ctx) }()

	time.Sleep(120 * time.Millisecond)
	cancel()

	mu.Lock()
	if len(collected) != 0 {
		t.Errorf("expected 0 events when RecordBaseline is false, got %d", len(collected))
	}
	mu.Unlock()
}

func TestFSPoller_DataDirSymlink_NotMonitored(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "lifelog-data")
	_ = os.MkdirAll(dataDir, 0700)
	_ = os.WriteFile(filepath.Join(dataDir, "lifelog.db"), []byte("secret"), 0600)

	watchDir := filepath.Join(tempDir, "workspace")
	_ = os.MkdirAll(watchDir, 0755)

	// Create symlink inside watchDir pointing to dataDir
	symlinkPath := filepath.Join(watchDir, "data_link")
	if err := os.Symlink(dataDir, symlinkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	var mu sync.Mutex
	var collected []events.Event

	poller, err := collector.NewFSPoller(collector.PollerConfig{
		Root:           watchDir,
		DataDir:        dataDir,
		Interval:       50 * time.Millisecond,
		RecordBaseline: true,
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

	go func() { _ = poller.Start(ctx) }()
	time.Sleep(120 * time.Millisecond)

	mu.Lock()
	for _, e := range collected {
		if e.Path == symlinkPath || filepath.Clean(e.Path) == filepath.Join(symlinkPath, "lifelog.db") {
			t.Errorf("symlink to dataDir was improperly observed: %s", e.Path)
		}
	}
	mu.Unlock()
}

func TestFSPoller_DeepUnicodePaths(t *testing.T) {
	tempDir := t.TempDir()
	watchDir := filepath.Join(tempDir, "unicode_watch")
	nested := filepath.Join(watchDir, "📁 folder with spaces", "日本語", "sub")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}

	fileA := filepath.Join(nested, "テスト file 1.txt")
	_ = os.WriteFile(fileA, []byte("hello"), 0644)

	var mu sync.Mutex
	var collected []events.Event

	poller, err := collector.NewFSPoller(collector.PollerConfig{
		Root:           watchDir,
		Interval:       50 * time.Millisecond,
		RecordBaseline: true,
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

	go func() { _ = poller.Start(ctx) }()
	time.Sleep(120 * time.Millisecond)

	mu.Lock()
	found := false
	for _, e := range collected {
		if e.Path == fileA {
			found = true
			break
		}
	}
	mu.Unlock()

	if !found {
		t.Errorf("file with unicode and spaces was not captured: %s", fileA)
	}
}

func TestFSPoller_SymlinkLoop_NoHang(t *testing.T) {
	tempDir := t.TempDir()
	watchDir := filepath.Join(tempDir, "loop_watch")
	if err := os.MkdirAll(watchDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create self-referential symlink loop
	loopLink := filepath.Join(watchDir, "self_loop")
	if err := os.Symlink(watchDir, loopLink); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	var mu sync.Mutex
	var collected []events.Event

	poller, err := collector.NewFSPoller(collector.PollerConfig{
		Root:           watchDir,
		Interval:       50 * time.Millisecond,
		RecordBaseline: true,
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

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// Must complete gracefully without stack overflow or infinite loop
	if err := poller.Start(ctx); err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFSPoller_FileRename(t *testing.T) {
	tempDir := t.TempDir()
	watchDir := filepath.Join(tempDir, "rename_watch")
	if err := os.MkdirAll(watchDir, 0755); err != nil {
		t.Fatal(err)
	}

	origFile := filepath.Join(watchDir, "orig.txt")
	renamedFile := filepath.Join(watchDir, "renamed.txt")
	_ = os.WriteFile(origFile, []byte("data"), 0644)

	var mu sync.Mutex
	var collected []events.Event

	poller, err := collector.NewFSPoller(collector.PollerConfig{
		Root:           watchDir,
		Interval:       50 * time.Millisecond,
		RecordBaseline: false,
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

	go func() { _ = poller.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Rename file
	if err := os.Rename(origFile, renamedFile); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	var hasDeletedOrig, hasCreatedRenamed bool
	for _, e := range collected {
		if e.Path == origFile && e.Type == events.EventDeleted {
			hasDeletedOrig = true
		}
		if e.Path == renamedFile && e.Type == events.EventCreated {
			hasCreatedRenamed = true
		}
	}
	mu.Unlock()

	if !hasDeletedOrig || !hasCreatedRenamed {
		t.Errorf("rename detection failed: deletedOrig=%v, createdRenamed=%v", hasDeletedOrig, hasCreatedRenamed)
	}
}
