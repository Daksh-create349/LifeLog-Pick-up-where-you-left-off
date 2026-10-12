package resumption

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/paths"
)

const (
	// ExportSchemaVersion defines the serialized JSON/Markdown export contract.
	ExportSchemaVersion = "1.0.0"

	// ExportGenerator defines the application signature on exports.
	ExportGenerator = "LIFELOG"
)

// ExportEnvelope wraps a ResumptionReport with portable version and generation metadata.
type ExportEnvelope struct {
	SchemaVersion string            `json:"schema_version"`
	ExportedAt    time.Time         `json:"exported_at"`
	Generator     string            `json:"generator"`
	Report        *ResumptionReport `json:"report"`
}

// FormatJSON produces an indented, machine-readable JSON representation of the resumption report.
func FormatJSON(r *ResumptionReport) ([]byte, error) {
	if r == nil {
		return nil, errors.New("cannot format nil report")
	}

	envelope := ExportEnvelope{
		SchemaVersion: ExportSchemaVersion,
		ExportedAt:    time.Now().UTC(),
		Generator:     ExportGenerator,
		Report:        r,
	}

	return json.MarshalIndent(envelope, "", "  ")
}

// FormatMarkdown produces a human-readable Markdown export preserving Observed, Derived, and Unavailable boundaries.
func FormatMarkdown(r *ResumptionReport) ([]byte, error) {
	if r == nil {
		return nil, errors.New("cannot format nil report")
	}

	var buf bytes.Buffer
	now := time.Now().UTC()

	buf.WriteString("# LIFELOG Project Resumption Report\n\n")

	// Overview metadata table
	buf.WriteString("| Field | Value |\n")
	buf.WriteString("| :--- | :--- |\n")
	buf.WriteString(fmt.Sprintf("| **Schema Version** | `%s` |\n", ExportSchemaVersion))
	buf.WriteString(fmt.Sprintf("| **Exported At** | `%s` |\n", now.Format("2006-01-02 15:04:05 UTC")))
	buf.WriteString(fmt.Sprintf("| **Target Path** | `%s` |\n", escapeMarkdown(r.ProjectPath)))
	buf.WriteString(fmt.Sprintf("| **Canonical Path** | `%s` |\n", escapeMarkdown(r.CanonicalPath)))

	if r.LatestActivityTime != nil {
		buf.WriteString(fmt.Sprintf("| **Last Observed Activity** | `%s` |\n", r.LatestActivityTime.UTC().Format("2006-01-02 15:04:05 UTC")))
	} else {
		buf.WriteString("| **Last Observed Activity** | *No recorded activity* |\n")
	}

	if r.Since != nil {
		buf.WriteString(fmt.Sprintf("| **Reporting Window** | Since `%s` (inactivity gap: `%v`) |\n", r.Since.UTC().Format("2006-01-02 15:04:05 UTC"), r.Derived.InactivityGap))
	} else {
		buf.WriteString(fmt.Sprintf("| **Reporting Window** | All recorded history (inactivity gap: `%v`) |\n", r.Derived.InactivityGap))
	}
	buf.WriteString("\n---\n\n")

	// 1. OBSERVED ACTIVITY
	buf.WriteString("## 1. Observed Activity (Persisted Local Facts)\n\n")

	// 1A. Filesystem
	buf.WriteString("### 1.1 Filesystem Memory\n\n")
	fs := r.Observed.Filesystem
	if fs.TotalEvents > 0 {
		buf.WriteString(fmt.Sprintf("- **Total Events:** %d\n", fs.TotalEvents))
		buf.WriteString(fmt.Sprintf("- **Breakdown:** %d created, %d modified, %d deleted\n\n",
			fs.TypeCounts[events.EventCreated],
			fs.TypeCounts[events.EventModified],
			fs.TypeCounts[events.EventDeleted]))

		if len(fs.RecentChanges) > 0 {
			buf.WriteString("| Action | Relative Path | Events | Last Modified (UTC) |\n")
			buf.WriteString("| :--- | :--- | :--- | :--- |\n")
			for _, ch := range fs.RecentChanges {
				buf.WriteString(fmt.Sprintf("| `%s` | `%s` | %d | `%s` |\n",
					ch.LastEventType,
					escapeMarkdown(ch.RelativePath),
					ch.EventCount,
					ch.LastModTime.UTC().Format("2006-01-02 15:04:05")))
			}
			buf.WriteString("\n")
		}
	} else {
		buf.WriteString("*No filesystem events recorded for this path.*\n\n")
	}

	// 1B. Git
	buf.WriteString("### 1.2 Git Repository Memory\n\n")
	g := r.Observed.Git
	if g.IsRegistered {
		branchState := g.CurrentBranch
		if g.IsDetached {
			branchState += " (detached HEAD)"
		}
		buf.WriteString(fmt.Sprintf("- **Repository ID:** `%s`\n", g.RepoID))
		buf.WriteString(fmt.Sprintf("- **Repository Path:** `%s`\n", escapeMarkdown(g.RepoPath)))
		buf.WriteString(fmt.Sprintf("- **Active Branch:** `%s`\n", escapeMarkdown(branchState)))

		if g.HeadCommit != nil {
			buf.WriteString(fmt.Sprintf("- **HEAD Commit:** `%s` by %s (`%s`)\n",
				truncate(g.HeadCommit.CommitHash, 8),
				escapeMarkdown(g.HeadCommit.AuthorName),
				g.HeadCommit.AuthorTime.UTC().Format("2006-01-02 15:04:05")))
			buf.WriteString(fmt.Sprintf("  > %s\n\n", escapeMarkdown(g.HeadCommit.Subject)))
		}

		if len(g.WorkingTree) > 0 {
			buf.WriteString(fmt.Sprintf("#### Working Tree Changes (%d observed)\n\n", len(g.WorkingTree)))
			buf.WriteString("| Status | Path |\n")
			buf.WriteString("| :--- | :--- |\n")
			for _, wt := range g.WorkingTree {
				buf.WriteString(fmt.Sprintf("| `%s` | `%s` |\n", wt.Status, escapeMarkdown(wt.Path)))
			}
			buf.WriteString("\n")
		} else {
			buf.WriteString("- **Working Tree State:** Clean (0 uncommitted changes observed)\n\n")
		}

		if len(g.RecentCommits) > 0 {
			buf.WriteString("#### Recent Commits\n\n")
			buf.WriteString("| Hash | Author | Author Date (UTC) | Subject |\n")
			buf.WriteString("| :--- | :--- | :--- | :--- |\n")
			for _, c := range g.RecentCommits {
				buf.WriteString(fmt.Sprintf("| `%s` | %s | `%s` | %s |\n",
					truncate(c.CommitHash, 8),
					escapeMarkdown(c.AuthorName),
					c.AuthorTime.UTC().Format("2006-01-02 15:04"),
					escapeMarkdown(c.Subject)))
			}
			buf.WriteString("\n")
		}

		if len(g.BranchTransitions) > 0 {
			buf.WriteString("#### Branch Transitions\n\n")
			for _, bt := range g.BranchTransitions {
				oldHash := truncate(bt.OldCommitHash, 8)
				if oldHash == "" {
					oldHash = "<initial>"
				}
				buf.WriteString(fmt.Sprintf("- `%s`: branch `%s` moved `%s` -> `%s`\n",
					bt.ObservedAt.UTC().Format("2006-01-02 15:04:05"),
					escapeMarkdown(bt.BranchName),
					oldHash,
					truncate(bt.NewCommitHash, 8)))
			}
			buf.WriteString("\n")
		}
	} else if g.IsGitOnDisk {
		buf.WriteString("*Git repository detected on disk, but not yet registered/scanned into LIFELOG.*\n\n")
	} else {
		buf.WriteString("*No registered Git repository associated with this path.*\n\n")
	}

	// 1C. Terminal
	buf.WriteString("### 1.3 Terminal Execution Records\n\n")
	term := r.Observed.Terminal
	if term.TotalCommands > 0 {
		buf.WriteString(fmt.Sprintf("- **Explicitly Recorded Commands:** %d\n\n", term.TotalCommands))
		buf.WriteString("| Timestamp (UTC) | Exit Status | Duration | Command |\n")
		buf.WriteString("| :--- | :--- | :--- | :--- |\n")
		for _, rec := range term.RecentCommands {
			cmd := rec.SanitizedCommand
			if rec.IsOmitted {
				cmd = "[DETAILS OMITTED] (" + rec.OmissionReason + ")"
			}
			exitStr := "exit=?"
			if rec.ExitCode != nil {
				exitStr = fmt.Sprintf("exit=%d", *rec.ExitCode)
			}
			buf.WriteString(fmt.Sprintf("| `%s` | `%s` | `%dms` | `%s` |\n",
				rec.StartTime.UTC().Format("2006-01-02 15:04:05"),
				exitStr,
				rec.DurationMs,
				escapeMarkdown(cmd)))
		}
		buf.WriteString("\n")
	} else {
		buf.WriteString("*No terminal commands explicitly recorded for this directory.*\n\n")
	}

	// 2. DERIVED CONTEXT
	buf.WriteString("## 2. Derived Activity Sessions & Synthesis (Computed Context)\n\n")
	buf.WriteString("### 2.1 Context Inference\n\n")
	buf.WriteString(fmt.Sprintf("> %s\n\n", escapeMarkdown(r.Derived.ActivityInference)))

	buf.WriteString("### 2.2 Chronological Activity Sessions\n\n")
	if len(r.Derived.Sessions) > 0 {
		buf.WriteString("| Session ID | Start (UTC) | End (UTC) | Duration | Events | Summary |\n")
		buf.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")
		for _, s := range r.Derived.Sessions {
			buf.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | `%s` | %d | %s |\n",
				s.ID,
				s.StartTime.UTC().Format("2006-01-02 15:04:05"),
				s.EndTime.UTC().Format("15:04:05"),
				formatDuration(s.Duration),
				s.ActivityCount,
				escapeMarkdown(s.Summary)))
		}
		buf.WriteString("\n")
	} else {
		buf.WriteString("*No distinct activity sessions identified.*\n\n")
	}

	// 3. UNAVAILABLE CONTEXT
	buf.WriteString("## 3. Unavailable Context & Privacy Boundaries\n\n")
	for _, notice := range r.Unavailable {
		buf.WriteString(fmt.Sprintf("- %s\n", escapeMarkdown(notice)))
	}
	buf.WriteString("\n---\n\n")
	buf.WriteString("*Note: This export reflects local activity observed by LIFELOG. It does not represent an unobserved or complete record of user activity.*\n")

	return buf.Bytes(), nil
}

// SafeWriteExportFile writes data to destPath atomically, with overwrite protection and
// strict validation against protected LIFELOG directories.
func SafeWriteExportFile(destPath string, data []byte, force bool, dataDir string) error {
	if strings.TrimSpace(destPath) == "" {
		return errors.New("output destination path cannot be empty")
	}

	absDest, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("resolving destination path: %w", err)
	}
	absDest = filepath.Clean(absDest)

	// Protected LIFELOG database / directory check
	if dataDir != "" {
		cleanDataDir := filepath.Clean(dataDir)
		canonicalDataDir, _ := paths.Canonical(cleanDataDir)
		if canonicalDataDir == "" {
			canonicalDataDir = cleanDataDir
		}

		if paths.IsSubpath(cleanDataDir, absDest) || paths.IsSubpath(canonicalDataDir, absDest) {
			return fmt.Errorf("destination %q is within protected LIFELOG data directory %q", destPath, dataDir)
		}

		// Check if destination path evaluates through symlink into protected data directory
		if evalDest, err := filepath.EvalSymlinks(absDest); err == nil {
			evalDest = filepath.Clean(evalDest)
			if paths.IsSubpath(cleanDataDir, evalDest) || paths.IsSubpath(canonicalDataDir, evalDest) {
				return fmt.Errorf("destination %q resolves to protected LIFELOG data directory %q", destPath, dataDir)
			}
		}

		// Check if destination parent directory evaluates through symlink into protected data directory
		parentDir := filepath.Dir(absDest)
		if evalParent, err := filepath.EvalSymlinks(parentDir); err == nil {
			evalParent = filepath.Clean(evalParent)
			if paths.IsSubpath(cleanDataDir, evalParent) || paths.IsSubpath(canonicalDataDir, evalParent) {
				return fmt.Errorf("destination parent directory %q resolves to protected LIFELOG data directory %q", parentDir, dataDir)
			}
		}
	}

	// Verify destination is not an existing directory
	info, err := os.Stat(absDest)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("destination %q is an existing directory", destPath)
		}
		if !force {
			return fmt.Errorf("destination %q already exists; use --force to overwrite", destPath)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking destination path: %w", err)
	}

	parentDir := filepath.Dir(absDest)
	pInfo, err := os.Stat(parentDir)
	if err != nil || !pInfo.IsDir() {
		return fmt.Errorf("destination parent directory %q does not exist", parentDir)
	}

	// Write atomically using temporary file in same parent directory
	tempFile, err := os.CreateTemp(parentDir, fmt.Sprintf(".%s.tmp-*", filepath.Base(absDest)))
	if err != nil {
		return fmt.Errorf("creating temporary file for atomic write: %w", err)
	}
	tempPath := tempFile.Name()

	defer func() {
		if tempPath != "" {
			_ = os.Remove(tempPath)
		}
	}()

	// Ensure restrictive permissions (0600)
	if err := tempFile.Chmod(paths.FilePerm); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("setting file permissions: %w", err)
	}

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("writing export content: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("syncing export file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("closing temporary file: %w", err)
	}

	// Atomic rename to final destination
	if err := os.Rename(tempPath, absDest); err != nil {
		return fmt.Errorf("atomically renaming export to %q: %w", absDest, err)
	}

	// Completed successfully, prevent defer cleanup of tempPath
	tempPath = ""
	return nil
}

func escapeMarkdown(s string) string {
	// Escape pipes and backticks inside table cells or formatted strings
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
