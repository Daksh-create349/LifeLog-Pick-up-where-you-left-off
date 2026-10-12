package resumption

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/git"
	"github.com/lifelog/lifelog/internal/storage"
	"github.com/lifelog/lifelog/internal/terminal"
)

func TestBuildPackage_Basic(t *testing.T) {
	now := time.Now().UTC()
	rep := &ResumptionReport{
		ProjectPath:   "/test/my-project",
		CanonicalPath: "/test/my-project",
		PathExists:    true,
		QueryTime:     now,
		Observed: ObservedContext{
			Filesystem: ObservedFilesystem{
				TotalEvents: 5,
				TypeCounts:  map[events.EventType]int{events.EventCreated: 5},
			},
			Git: ObservedGit{
				IsRegistered:  true,
				RepoID:        "repo_123",
				CurrentBranch: "main",
			},
			Terminal: ObservedTerminal{
				TotalCommands: 0,
			},
		},
		Derived: DerivedContext{
			ActivityInference: "Active development observed on branch main.",
			InactivityGap:     30 * time.Minute,
		},
		Unavailable: []string{"No terminal commands recorded."},
	}

	pkg, err := BuildPackage(rep, "0.1.0")
	if err != nil {
		t.Fatalf("BuildPackage failed: %v", err)
	}

	if pkg.PackageVersion != PackageFormatVersion {
		t.Errorf("expected package version %s, got %s", PackageFormatVersion, pkg.PackageVersion)
	}
	if pkg.Generator != PackageGenerator {
		t.Errorf("expected generator %s, got %s", PackageGenerator, pkg.Generator)
	}
	if pkg.AppVersion != "0.1.0" {
		t.Errorf("expected app version 0.1.0, got %s", pkg.AppVersion)
	}
	if pkg.DisplayName != "my-project" {
		t.Errorf("expected display name 'my-project', got %q", pkg.DisplayName)
	}
	if len(pkg.IncludedDomains) != 2 || pkg.IncludedDomains[0] != "filesystem" || pkg.IncludedDomains[1] != "git" {
		t.Errorf("unexpected included domains: %v", pkg.IncludedDomains)
	}
	if len(pkg.EmptyDomains) != 1 || pkg.EmptyDomains[0] != "terminal" {
		t.Errorf("unexpected empty domains: %v", pkg.EmptyDomains)
	}
}

func TestPackageJSON_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	exitCode := 0
	rep := &ResumptionReport{
		ProjectPath:        "/repos/sample",
		CanonicalPath:      "/repos/sample",
		PathExists:         true,
		QueryTime:          now,
		LatestActivityTime: &now,
		Observed: ObservedContext{
			Filesystem: ObservedFilesystem{
				TotalEvents: 1,
				TypeCounts:  map[events.EventType]int{events.EventModified: 1},
				RecentChanges: []PathChange{
					{
						Path:          "/repos/sample/main.go",
						RelativePath:  "main.go",
						LastEventType: events.EventModified,
						EventCount:    1,
						LastModTime:   now,
					},
				},
			},
			Git: ObservedGit{
				IsRegistered:  true,
				RepoID:        "repo_xyz",
				RepoPath:      "/repos/sample",
				CurrentBranch: "feature/auth",
				HeadCommit: &storage.GitCommitRecord{
					CommitHash: "abcdef1234567890",
					AuthorName: "Developer",
					AuthorTime: now,
					Subject:    "Initial commit",
				},
				WorkingTree: []git.FileChange{
					{Path: "main.go", Status: "M"},
				},
			},
			Terminal: ObservedTerminal{
				TotalCommands: 1,
				RecentCommands: []terminal.Record{
					{
						ID:               "term_1",
						WorkingDir:       "/repos/sample",
						Executable:       "go",
						SanitizedCommand: "go test ./...",
						StartTime:        now,
						DurationMs:       120,
						ExitCode:         &exitCode,
					},
				},
			},
		},
		Derived: DerivedContext{
			ActivityInference: "Full-stack activity observed.",
			InactivityGap:     15 * time.Minute,
		},
		Unavailable: []string{"Git diffs are not collected."},
	}

	pkg, err := BuildPackage(rep, "0.1.0")
	if err != nil {
		t.Fatalf("BuildPackage error: %v", err)
	}

	jsonBytes, err := FormatPackageJSON(pkg)
	if err != nil {
		t.Fatalf("FormatPackageJSON error: %v", err)
	}

	var decoded ContextPackage
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.PackageVersion != pkg.PackageVersion {
		t.Errorf("decoded version mismatch: %s vs %s", decoded.PackageVersion, pkg.PackageVersion)
	}
	if decoded.DisplayName != pkg.DisplayName {
		t.Errorf("decoded display name mismatch: %s vs %s", decoded.DisplayName, pkg.DisplayName)
	}
	if len(decoded.IncludedDomains) != 3 {
		t.Errorf("expected 3 included domains, got %d", len(decoded.IncludedDomains))
	}
	if len(decoded.EmptyDomains) != 0 {
		t.Errorf("expected 0 empty domains, got %d", len(decoded.EmptyDomains))
	}
}

func TestPackageMarkdown_Formatting(t *testing.T) {
	now := time.Now().UTC()
	rep := &ResumptionReport{
		ProjectPath:   "/my | unusual `path`",
		CanonicalPath: "/my | unusual `path`",
		PathExists:    true,
		QueryTime:     now,
		Observed:      ObservedContext{},
		Derived: DerivedContext{
			ActivityInference: "No activity found.",
		},
		Unavailable: []string{"All domains empty."},
	}

	pkg, err := BuildPackage(rep, "0.1.0")
	if err != nil {
		t.Fatalf("BuildPackage error: %v", err)
	}

	mdBytes, err := FormatPackageMarkdown(pkg)
	if err != nil {
		t.Fatalf("FormatPackageMarkdown error: %v", err)
	}

	mdStr := string(mdBytes)
	if !strings.Contains(mdStr, "# LIFELOG Project Context Package") {
		t.Errorf("missing header in markdown: %s", mdStr)
	}
	if !strings.Contains(mdStr, "Data Domain Coverage") {
		t.Errorf("missing domain coverage section: %s", mdStr)
	}
	if !strings.Contains(mdStr, "Domains with No Available Observations") {
		t.Errorf("missing empty domains notice: %s", mdStr)
	}
}

func TestSafeWriteExportFile_SymlinkEscapeRejection(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	dbFile := filepath.Join(dataDir, "lifelog.db")
	if err := os.WriteFile(dbFile, []byte("sqlite content"), 0600); err != nil {
		t.Fatal(err)
	}

	// Create symlink outside dataDir pointing to dbFile
	outDir := filepath.Join(tempDir, "outside")
	if err := os.MkdirAll(outDir, 0700); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(outDir, "fake_export.md")
	if err := os.Symlink(dbFile, symlinkPath); err != nil {
		t.Fatal(err)
	}

	// Attempting to write to symlinkPath should be REJECTED because target resolves to dataDir
	err := SafeWriteExportFile(symlinkPath, []byte("overwrite payload"), true, dataDir)
	if err == nil {
		t.Fatal("expected error writing through symlink pointing into dataDir, got nil")
	}
	if !strings.Contains(err.Error(), "resolves to protected LIFELOG data directory") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Verify original file was not modified
	content, err := os.ReadFile(dbFile)
	if err != nil || string(content) != "sqlite content" {
		t.Fatalf("dbFile was modified or corrupted: %v (content=%s)", err, string(content))
	}
}
