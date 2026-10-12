package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/events"
)

func BenchmarkFSPoller_Scan(b *testing.B) {
	tempDir := b.TempDir()
	benchDir := filepath.Join(tempDir, "bench_files")
	if err := os.MkdirAll(benchDir, 0755); err != nil {
		b.Fatal(err)
	}

	// Create 500 files across 10 subdirectories
	for i := 0; i < 10; i++ {
		sub := filepath.Join(benchDir, fmt.Sprintf("dir_%d", i))
		if err := os.MkdirAll(sub, 0755); err != nil {
			b.Fatal(err)
		}
		for j := 0; j < 50; j++ {
			f := filepath.Join(sub, fmt.Sprintf("file_%d.txt", j))
			if err := os.WriteFile(f, []byte("benchmark payload data"), 0644); err != nil {
				b.Fatal(err)
			}
		}
	}

	poller, err := NewFSPoller(PollerConfig{
		Root:           benchDir,
		Interval:       1 * time.Second,
		RecordBaseline: false,
		OnEvents:       func(evts []events.Event) error { return nil },
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	// Benchmark the real scan routine by invoking scan() directly
	for i := 0; i < b.N; i++ {
		snap, err := poller.scan()
		if err != nil {
			b.Fatalf("scan error: %v", err)
		}
		if len(snap) != 510 { // 500 files + 10 directories
			b.Fatalf("unexpected snapshot size: %d", len(snap))
		}
	}
}
