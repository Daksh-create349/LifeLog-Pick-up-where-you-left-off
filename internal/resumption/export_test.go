package resumption_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/paths"
	"github.com/lifelog/lifelog/internal/resumption"
)

func sampleReport() *resumption.ResumptionReport {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	return &resumption.ResumptionReport{
		ProjectPath:   "/Users/alice/projects/demo",
		CanonicalPath: "/Users/alice/projects/demo",
		PathExists:    true,
		QueryTime:     now,
		TimeWindow:    7 * 24 * time.Hour,
		Observed: resumption.ObservedContext{
			Filesystem: resumption.ObservedFilesystem{
				TotalEvents: 3,
				TypeCounts: map[events.EventType]int{
					events.EventCreated:  1,
					events.EventModified: 2,
				},
				RecentChanges: []resumption.PathChange{
					{
						Path:          "/Users/alice/projects/demo/main.go",
						RelativePath:  "main.go",
						LastEventType: events.EventModified,
						EventCount:    2,
						LastModTime:   now.Add(-10 * time.Minute),
					},
				},
			},
			Git: resumption.ObservedGit{
				IsRegistered:  true,
				RepoID:        "repo_demo123",
				RepoPath:      "/Users/alice/projects/demo",
				CurrentBranch: "main",
			},
		},
		Derived: resumption.DerivedContext{
			InactivityGap:     30 * time.Minute,
			ActivityInference: "Recent recorded activity suggests work on main.go.",
			Sessions: []resumption.Session{
				{
					ID:            "session-001",
					StartTime:     now.Add(-20 * time.Minute),
					EndTime:       now.Add(-10 * time.Minute),
					Duration:      10 * time.Minute,
					ActivityCount: 2,
					Summary:       "2 file event(s)",
				},
			},
		},
		Unavailable: []string{
			"File contents and source code diffs are not indexed (privacy boundary).",
		},
	}
}

func TestExport_JSONRoundTrip(t *testing.T) {
	orig := sampleReport()
	data, err := resumption.FormatJSON(orig)
	if err != nil {
		t.Fatalf("FormatJSON failed: %v", err)
	}

	var env resumption.ExportEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshaling JSON export failed: %v", err)
	}

	if env.SchemaVersion != resumption.ExportSchemaVersion {
		t.Errorf("got schema version %q, want %q", env.SchemaVersion, resumption.ExportSchemaVersion)
	}
	if env.Generator != resumption.ExportGenerator {
		t.Errorf("got generator %q, want %q", env.Generator, resumption.ExportGenerator)
	}
	if env.Report == nil {
		t.Fatal("expected report to be non-nil in envelope")
	}
	if env.Report.CanonicalPath != orig.CanonicalPath {
		t.Errorf("got canonical path %q, want %q", env.Report.CanonicalPath, orig.CanonicalPath)
	}
	if env.Report.Observed.Filesystem.TotalEvents != orig.Observed.Filesystem.TotalEvents {
		t.Errorf("got total events %d, want %d", env.Report.Observed.Filesystem.TotalEvents, orig.Observed.Filesystem.TotalEvents)
	}
}

func TestExport_MarkdownFormatting(t *testing.T) {
	report := sampleReport()
	data, err := resumption.FormatMarkdown(report)
	if err != nil {
		t.Fatalf("FormatMarkdown failed: %v", err)
	}
	md := string(data)

	// Verify required sections and structure
	if !strings.Contains(md, "# LIFELOG Project Resumption Report") {
		t.Errorf("missing title in markdown export: %s", md)
	}
	if !strings.Contains(md, "## 1. Observed Activity (Persisted Local Facts)") {
		t.Errorf("missing Observed section in markdown export: %s", md)
	}
	if !strings.Contains(md, "## 2. Derived Activity Sessions & Synthesis (Computed Context)") {
		t.Errorf("missing Derived section in markdown export: %s", md)
	}
	if !strings.Contains(md, "## 3. Unavailable Context & Privacy Boundaries") {
		t.Errorf("missing Unavailable section in markdown export: %s", md)
	}
	if !strings.Contains(md, "main.go") {
		t.Errorf("missing main.go in markdown export: %s", md)
	}
	if !strings.Contains(md, "session-001") {
		t.Errorf("missing session-001 in markdown export: %s", md)
	}
}

func TestSafeWriteExportFile_AtomicAndPermissions(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "lifelog_data")
	destFile := filepath.Join(tempDir, "export.md")

	content := []byte("# Export Content Test\n")
	err := resumption.SafeWriteExportFile(destFile, content, false, dataDir)
	if err != nil {
		t.Fatalf("SafeWriteExportFile failed: %v", err)
	}

	readBack, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("failed reading back export file: %v", err)
	}
	if string(readBack) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(readBack), string(content))
	}

	info, err := os.Stat(destFile)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != paths.FilePerm {
		t.Errorf("got permissions %04o, want %04o", perm, paths.FilePerm)
	}
}

func TestSafeWriteExportFile_ProtectedPathRejection(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "lifelog_data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Attempting to write into dataDir must fail
	protectedDest := filepath.Join(dataDir, "malicious_overwrite.db")
	err := resumption.SafeWriteExportFile(protectedDest, []byte("data"), true, dataDir)
	if err == nil {
		t.Errorf("expected error writing into protected dataDir, got nil")
	}
	if !strings.Contains(err.Error(), "protected") {
		t.Errorf("expected protected message, got: %v", err)
	}
}

func TestSafeWriteExportFile_OverwriteProtection(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "lifelog_data")
	destFile := filepath.Join(tempDir, "existing.md")

	if err := os.WriteFile(destFile, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}

	// 1. Without force -> must return error
	err := resumption.SafeWriteExportFile(destFile, []byte("new"), false, dataDir)
	if err == nil {
		t.Errorf("expected error overwriting existing file without force, got nil")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("expected --force error message, got: %v", err)
	}

	// 2. With force -> must succeed
	err = resumption.SafeWriteExportFile(destFile, []byte("new"), true, dataDir)
	if err != nil {
		t.Fatalf("expected success with force, got: %v", err)
	}
	readBack, _ := os.ReadFile(destFile)
	if string(readBack) != "new" {
		t.Errorf("expected new content after force overwrite, got %q", string(readBack))
	}
}
