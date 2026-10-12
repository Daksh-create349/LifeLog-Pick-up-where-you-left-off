package resumption

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/paths"
	"github.com/lifelog/lifelog/internal/storage"
)

// BuildResumptionReport queries all local memory domains, associates records with the
// canonical project path, synthesizes chronological activity sessions, and returns a unified report.
func BuildResumptionReport(ctx context.Context, db *storage.DB, projectPath string, opts Options) (*ResumptionReport, error) {
	if projectPath == "" {
		projectPath = "."
	}

	canonical, err := paths.Canonical(projectPath)
	if err != nil {
		return nil, fmt.Errorf("canonicalizing project path %q: %w", projectPath, err)
	}

	lookupPath := projectPath
	if !filepath.IsAbs(lookupPath) {
		if abs, err := filepath.Abs(lookupPath); err == nil {
			lookupPath = abs
		} else {
			lookupPath = canonical
		}
	}

	// Validate path existence on disk
	pathExists := false
	if info, err := os.Stat(canonical); err == nil && info.IsDir() {
		pathExists = true
	}

	if opts.Limit <= 0 {
		opts.Limit = 20
	} else if opts.Limit > 500 {
		opts.Limit = 500
	}

	if opts.InactivityGap <= 0 {
		opts.InactivityGap = DefaultInactivityGap
	}

	now := time.Now().UTC()
	var since *time.Time
	if opts.Since != nil {
		since = opts.Since
	} else if opts.Window > 0 {
		t := now.Add(-opts.Window)
		since = &t
	}

	report := &ResumptionReport{
		ProjectPath:   projectPath,
		CanonicalPath: canonical,
		PathExists:    pathExists,
		QueryTime:     now,
		TimeWindow:    opts.Window,
		Since:         since,
		Derived: DerivedContext{
			InactivityGap: opts.InactivityGap,
		},
		Observed: ObservedContext{
			Filesystem: ObservedFilesystem{
				TypeCounts: make(map[events.EventType]int),
			},
		},
	}

	var allActivities []ActivityItem
	var latestTime *time.Time

	updateLatest := func(t time.Time) {
		if latestTime == nil || t.After(*latestTime) {
			tCopy := t
			latestTime = &tCopy
		}
	}

	// 1. FILESYSTEM ACTIVITY
	fsSummary, err := db.GetProjectFilesystemSummary(ctx, lookupPath, since, opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("retrieving filesystem activity: %w", err)
	}
	report.Observed.Filesystem.TotalEvents = fsSummary.TotalCount
	report.Observed.Filesystem.TypeCounts = fsSummary.TypeCounts

	// Aggregate path changes
	changeMap := make(map[string]*PathChange)
	for _, ev := range fsSummary.RecentEvents {
		updateLatest(ev.CreatedAt)

		rel, err := filepath.Rel(canonical, ev.Path)
		if err != nil || strings.HasPrefix(rel, "..") {
			rel, err = filepath.Rel(filepath.Clean(projectPath), ev.Path)
			if err != nil || strings.HasPrefix(rel, "..") {
				rel = ev.Path
			}
		}

		c, exists := changeMap[ev.Path]
		if !exists {
			c = &PathChange{
				Path:          ev.Path,
				RelativePath:  rel,
				LastEventType: ev.Type,
				LastModTime:   ev.ModTime,
			}
			changeMap[ev.Path] = c
		}
		c.EventCount++
		if ev.ModTime.After(c.LastModTime) {
			c.LastModTime = ev.ModTime
			c.LastEventType = ev.Type
		}

		allActivities = append(allActivities, ActivityItem{
			ID:        ev.ID,
			Domain:    "filesystem",
			Type:      "fs_" + string(ev.Type),
			Timestamp: ev.CreatedAt,
			Summary:   rel,
			Details:   fmt.Sprintf("%s (%d bytes)", ev.Type, ev.Size),
			Path:      ev.Path,
		})
	}

	for _, c := range changeMap {
		report.Observed.Filesystem.RecentChanges = append(report.Observed.Filesystem.RecentChanges, *c)
	}
	sort.Slice(report.Observed.Filesystem.RecentChanges, func(i, j int) bool {
		return report.Observed.Filesystem.RecentChanges[i].LastModTime.After(report.Observed.Filesystem.RecentChanges[j].LastModTime)
	})
	if len(report.Observed.Filesystem.RecentChanges) > opts.Limit {
		report.Observed.Filesystem.RecentChanges = report.Observed.Filesystem.RecentChanges[:opts.Limit]
	}

	// 2. GIT ACTIVITY
	gitRepo, err := db.FindGitRepoForPath(ctx, lookupPath)
	if err != nil {
		return nil, fmt.Errorf("locating git repository: %w", err)
	}

	if gitRepo != nil {
		report.Observed.Git.IsRegistered = true
		report.Observed.Git.RepoID = gitRepo.ID
		report.Observed.Git.RepoPath = gitRepo.Path

		// Latest snapshot
		snap, wtChanges, err := db.GetLatestGitSnapshot(ctx, gitRepo.ID)
		if err == nil && snap != nil {
			report.Observed.Git.CurrentBranch = snap.Branch
			report.Observed.Git.IsDetached = snap.IsDetached
			updateLatest(snap.ObservedAt)

			// If project is a subdirectory of the repo, filter working tree changes to this subpath
			for _, wt := range wtChanges {
				fullChangePath := filepath.Join(gitRepo.Path, wt.Path)
				if paths.IsSubpath(canonical, fullChangePath) || paths.IsSubpath(filepath.Clean(projectPath), fullChangePath) {
					report.Observed.Git.WorkingTree = append(report.Observed.Git.WorkingTree, wt)
				}
			}
		}

		// Commits
		commits, err := db.QueryGitCommitsSince(ctx, gitRepo.ID, since, opts.Limit)
		if err == nil {
			report.Observed.Git.RecentCommits = commits
			for _, c := range commits {
				updateLatest(c.AuthorTime)
				if snap != nil && snap.HeadCommitHash != "" && c.CommitHash == snap.HeadCommitHash && report.Observed.Git.HeadCommit == nil {
					cCopy := c
					report.Observed.Git.HeadCommit = &cCopy
				}

				allActivities = append(allActivities, ActivityItem{
					ID:        c.CommitHash,
					Domain:    "git",
					Type:      "git_commit",
					Timestamp: c.AuthorTime,
					Summary:   c.Subject,
					Details:   fmt.Sprintf("author: %s", c.AuthorName),
					Path:      gitRepo.Path,
				})
			}
		}

		// Branch transitions
		branchLogs, err := db.QueryGitBranchLogSince(ctx, gitRepo.ID, since, opts.Limit)
		if err == nil {
			report.Observed.Git.BranchTransitions = branchLogs
			for _, b := range branchLogs {
				updateLatest(b.ObservedAt)
				allActivities = append(allActivities, ActivityItem{
					ID:        b.ID,
					Domain:    "git",
					Type:      "git_branch",
					Timestamp: b.ObservedAt,
					Summary:   fmt.Sprintf("branch %s tip moved to %s", b.BranchName, truncate(b.NewCommitHash, 8)),
					Details:   b.BranchName,
					Path:      gitRepo.Path,
				})
			}
		}
	} else {
		// Check if a .git directory exists on disk to provide helpful diagnostic notice
		gitDir := filepath.Join(canonical, ".git")
		if info, err := os.Stat(gitDir); err == nil && (info.IsDir() || !info.IsDir()) {
			report.Observed.Git.IsGitOnDisk = true
		}
	}

	// 3. TERMINAL ACTIVITY
	termRecords, err := db.ListTerminalRecordsForProject(ctx, lookupPath, since, opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("retrieving terminal records: %w", err)
	}
	report.Observed.Terminal.TotalCommands = len(termRecords)
	report.Observed.Terminal.RecentCommands = termRecords

	for _, rec := range termRecords {
		updateLatest(rec.StartTime)
		summary := rec.SanitizedCommand
		if rec.IsOmitted {
			summary = "[DETAILS OMITTED]"
		}

		allActivities = append(allActivities, ActivityItem{
			ID:        rec.ID,
			Domain:    "terminal",
			Type:      "terminal_exec",
			Timestamp: rec.StartTime,
			Summary:   summary,
			Details:   fmt.Sprintf("exit=%v duration=%dms", rec.ExitCode, rec.DurationMs),
			Path:      rec.WorkingDir,
		})
	}

	report.LatestActivityTime = latestTime

	// 4. DERIVED ACTIVITY SESSIONS
	report.Derived.Sessions = SynthesizeSessions(allActivities, opts.InactivityGap, opts.Limit)

	// 5. ACTIVITY INFERENCE
	report.Derived.ActivityInference = generateActivityInference(report)

	// 6. UNAVAILABLE CONTEXT & NOTICES
	report.Unavailable = buildUnavailableNotices(report)

	return report, nil
}

func generateActivityInference(r *ResumptionReport) string {
	if r.LatestActivityTime == nil || len(r.Derived.Sessions) == 0 {
		return "No recorded activity found for this project in the selected time window."
	}

	var parts []string

	// Dominant paths or directories
	dirSet := make(map[string]int)
	for _, ch := range r.Observed.Filesystem.RecentChanges {
		dir := filepath.Dir(ch.RelativePath)
		if dir == "." {
			dir = "root files"
		}
		dirSet[dir] += ch.EventCount
	}
	type dirCount struct {
		name  string
		count int
	}
	var dirs []dirCount
	for d, cnt := range dirSet {
		dirs = append(dirs, dirCount{name: d, count: cnt})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].count > dirs[j].count
	})

	if len(dirs) > 0 {
		var topDirs []string
		for i := 0; i < len(dirs) && i < 2; i++ {
			topDirs = append(topDirs, fmt.Sprintf("%s (%d events)", dirs[i].name, dirs[i].count))
		}
		parts = append(parts, fmt.Sprintf("filesystem modifications in %s", strings.Join(topDirs, ", ")))
	}

	// Git details
	if r.Observed.Git.IsRegistered {
		gitDesc := fmt.Sprintf("branch %q", r.Observed.Git.CurrentBranch)
		if len(r.Observed.Git.RecentCommits) > 0 {
			gitDesc += fmt.Sprintf(" with commit %q", truncate(r.Observed.Git.RecentCommits[0].Subject, 40))
		}
		parts = append(parts, gitDesc)
	}

	// Terminal execution highlights
	if len(r.Observed.Terminal.RecentCommands) > 0 {
		topCmd := r.Observed.Terminal.RecentCommands[0]
		cmdDesc := topCmd.SanitizedCommand
		if topCmd.IsOmitted {
			cmdDesc = "[sensitive command execution]"
		}
		parts = append(parts, fmt.Sprintf("terminal execution (e.g., %q)", truncate(cmdDesc, 35)))
	}

	if len(parts) == 0 {
		return "Recent recorded activity suggests passive background observation with minimal changes."
	}

	return fmt.Sprintf("Recent recorded activity suggests work involving %s across %d derived activity session(s).",
		strings.Join(parts, ", "), len(r.Derived.Sessions))
}

func buildUnavailableNotices(r *ResumptionReport) []string {
	var notices []string

	// Project-specific gaps
	if !r.Observed.Git.IsRegistered {
		if r.Observed.Git.IsGitOnDisk {
			notices = append(notices, fmt.Sprintf("Git repository detected on disk at %s, but not yet scanned into LIFELOG (run 'lifelog git scan %s').", r.CanonicalPath, r.CanonicalPath))
		} else {
			notices = append(notices, "No Git repository has been registered or associated with this path.")
		}
	} else if len(r.Observed.Git.WorkingTree) == 0 && r.Observed.Git.HeadCommit == nil {
		notices = append(notices, "Working-tree snapshot not yet recorded for this Git repository.")
	}

	if r.Observed.Filesystem.TotalEvents == 0 {
		notices = append(notices, "No filesystem events have been observed under this directory (run 'lifelog watch' to monitor).")
	}

	if r.Observed.Terminal.TotalCommands == 0 {
		notices = append(notices, "No terminal command executions have been recorded in this working directory (use 'lifelog terminal record -- <cmd>').")
	}

	// Architectural privacy guarantees
	notices = append(notices, "File contents and source code diffs are not indexed or inspected (privacy boundary).")
	notices = append(notices, "Terminal standard output, standard error, and shell history are never captured (privacy boundary).")
	notices = append(notices, "Git remote URLs, credentials, and network repositories are not collected (privacy boundary).")

	return notices
}
