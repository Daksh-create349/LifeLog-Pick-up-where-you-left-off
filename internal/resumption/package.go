package resumption

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lifelog/lifelog/internal/events"
)

const (
	// PackageFormatVersion defines the version specification of the Context Package.
	PackageFormatVersion = "1.0.0"

	// PackageGenerator defines the generator name for Context Packages.
	PackageGenerator = "LIFELOG"
)

// ContextPackage is a structured, reusable, portable package containing project identity,
// recorded activity across domains, session synthesis, and explicit limitations.
type ContextPackage struct {
	PackageVersion  string            `json:"package_version"`
	Generator       string            `json:"generator"`
	AppVersion      string            `json:"app_version"`
	ExportedAt      time.Time         `json:"exported_at"`
	DisplayName     string            `json:"display_name"`
	ProjectPath     string            `json:"project_path"`
	CanonicalPath   string            `json:"canonical_path"`
	PathExists      bool              `json:"path_exists"`
	IncludedDomains []string          `json:"included_domains"`
	EmptyDomains    []string          `json:"empty_domains"`
	Report          *ResumptionReport `json:"report"`
}

// BuildPackage constructs a ContextPackage from a ResumptionReport and application version.
func BuildPackage(report *ResumptionReport, appVersion string) (*ContextPackage, error) {
	if report == nil {
		return nil, errors.New("cannot build package from nil resumption report")
	}

	dispName := filepath.Base(report.CanonicalPath)
	if dispName == "" || dispName == "." || dispName == string(filepath.Separator) {
		if report.CanonicalPath != "" {
			dispName = report.CanonicalPath
		} else {
			dispName = "unnamed-project"
		}
	}

	var included []string
	var empty []string

	// Check filesystem domain
	if report.Observed.Filesystem.TotalEvents > 0 {
		included = append(included, "filesystem")
	} else {
		empty = append(empty, "filesystem")
	}

	// Check git domain
	if report.Observed.Git.IsRegistered ||
		report.Observed.Git.HeadCommit != nil ||
		len(report.Observed.Git.WorkingTree) > 0 ||
		len(report.Observed.Git.RecentCommits) > 0 ||
		len(report.Observed.Git.BranchTransitions) > 0 {
		included = append(included, "git")
	} else {
		empty = append(empty, "git")
	}

	// Check terminal domain
	if report.Observed.Terminal.TotalCommands > 0 {
		included = append(included, "terminal")
	} else {
		empty = append(empty, "terminal")
	}

	sort.Strings(included)
	sort.Strings(empty)

	if included == nil {
		included = []string{}
	}
	if empty == nil {
		empty = []string{}
	}

	if appVersion == "" {
		appVersion = "unknown"
	}

	return &ContextPackage{
		PackageVersion:  PackageFormatVersion,
		Generator:       PackageGenerator,
		AppVersion:      appVersion,
		ExportedAt:      time.Now().UTC(),
		DisplayName:     dispName,
		ProjectPath:     report.ProjectPath,
		CanonicalPath:   report.CanonicalPath,
		PathExists:      report.PathExists,
		IncludedDomains: included,
		EmptyDomains:    empty,
		Report:          report,
	}, nil
}

// FormatPackageJSON serializes a ContextPackage to deterministic indented JSON.
func FormatPackageJSON(pkg *ContextPackage) ([]byte, error) {
	if pkg == nil {
		return nil, errors.New("cannot format nil package")
	}
	return json.MarshalIndent(pkg, "", "  ")
}

// FormatPackageMarkdown generates a human-readable Markdown representation of the package.
func FormatPackageMarkdown(pkg *ContextPackage) ([]byte, error) {
	if pkg == nil {
		return nil, errors.New("cannot format nil package")
	}
	r := pkg.Report
	if r == nil {
		return nil, errors.New("package contains nil report")
	}

	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("# LIFELOG Project Context Package: %s\n\n", escapeMarkdown(pkg.DisplayName)))

	// Package overview table
	buf.WriteString("| Field | Value |\n")
	buf.WriteString("| :--- | :--- |\n")
	buf.WriteString(fmt.Sprintf("| **Package Version** | `%s` |\n", pkg.PackageVersion))
	buf.WriteString(fmt.Sprintf("| **Generator** | `%s (v%s)` |\n", pkg.Generator, pkg.AppVersion))
	buf.WriteString(fmt.Sprintf("| **Exported At** | `%s` |\n", pkg.ExportedAt.Format("2006-01-02 15:04:05 UTC")))
	buf.WriteString(fmt.Sprintf("| **Display Name** | `%s` |\n", escapeMarkdown(pkg.DisplayName)))
	buf.WriteString(fmt.Sprintf("| **Project Path** | `%s` |\n", escapeMarkdown(pkg.ProjectPath)))
	buf.WriteString(fmt.Sprintf("| **Canonical Path** | `%s` |\n", escapeMarkdown(pkg.CanonicalPath)))

	if pkg.PathExists {
		buf.WriteString("| **Directory Exists On Disk** | Yes |\n")
	} else {
		buf.WriteString("| **Directory Exists On Disk** | *No (directory missing or inaccessible)* |\n")
	}

	if r.LatestActivityTime != nil {
		buf.WriteString(fmt.Sprintf("| **Latest Activity** | `%s` |\n", r.LatestActivityTime.UTC().Format("2006-01-02 15:04:05 UTC")))
	} else {
		buf.WriteString("| **Latest Activity** | *No recorded activity* |\n")
	}

	if r.Since != nil {
		buf.WriteString(fmt.Sprintf("| **Reporting Window** | Since `%s` (inactivity gap: `%v`) |\n", r.Since.UTC().Format("2006-01-02 15:04:05 UTC"), r.Derived.InactivityGap))
	} else {
		buf.WriteString(fmt.Sprintf("| **Reporting Window** | All recorded history (inactivity gap: `%v`) |\n", r.Derived.InactivityGap))
	}
	buf.WriteString("\n---\n\n")

	// Domain Coverage Section
	buf.WriteString("## 1. Data Domain Coverage\n\n")
	buf.WriteString(fmt.Sprintf("- **Domains with Recorded Observations (%d):** %s\n",
		len(pkg.IncludedDomains), formatDomainList(pkg.IncludedDomains)))
	buf.WriteString(fmt.Sprintf("- **Domains with No Available Observations (%d):** %s\n\n",
		len(pkg.EmptyDomains), formatDomainList(pkg.EmptyDomains)))

	// 2. OBSERVED ACTIVITY
	buf.WriteString("## 2. Observed Activity (Persisted Local Facts)\n\n")

	// 2A. Filesystem
	buf.WriteString("### 2.1 Filesystem Memory\n\n")
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

	// 2B. Git
	buf.WriteString("### 2.2 Git Repository Memory\n\n")
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

	// 2C. Terminal
	buf.WriteString("### 2.3 Terminal Execution Records\n\n")
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

	// 3. DERIVED CONTEXT
	buf.WriteString("## 3. Derived Activity Sessions & Synthesis (Computed Context)\n\n")
	buf.WriteString("### 3.1 Context Inference\n\n")
	buf.WriteString(fmt.Sprintf("> %s\n\n", escapeMarkdown(r.Derived.ActivityInference)))

	buf.WriteString("### 3.2 Chronological Activity Sessions\n\n")
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

	// 4. UNAVAILABLE CONTEXT
	buf.WriteString("## 4. Unavailable Context & Privacy Boundaries\n\n")
	for _, notice := range r.Unavailable {
		buf.WriteString(fmt.Sprintf("- %s\n", escapeMarkdown(notice)))
	}
	buf.WriteString("\n---\n\n")
	buf.WriteString("*Note: This context package is locally generated by LIFELOG. It contains only observed, sanitized metadata and does not contain file contents, Git diffs, shell streams, or remote credentials.*\n")

	return buf.Bytes(), nil
}

func formatDomainList(domains []string) string {
	if len(domains) == 0 {
		return "*None*"
	}
	var formatted []string
	for _, d := range domains {
		formatted = append(formatted, fmt.Sprintf("`%s`", d))
	}
	return strings.Join(formatted, ", ")
}
