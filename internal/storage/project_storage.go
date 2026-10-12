package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lifelog/lifelog/internal/paths"
)

// ProjectSortMode defines the sorting strategy for the multi-project overview.
type ProjectSortMode string

const (
	SortRecent   ProjectSortMode = "recent"
	SortName     ProjectSortMode = "name"
	SortActivity ProjectSortMode = "activity"
)

// ProjectOverview contains evidence-backed status and metrics for a discovered project.
type ProjectOverview struct {
	CanonicalPath        string         `json:"canonical_path"`
	DisplayName          string         `json:"display_name"`
	ExistsOnDisk         bool           `json:"exists_on_disk"`
	IsGitRegistered      bool           `json:"is_git_registered"`
	GitRepoID            string         `json:"git_repo_id,omitempty"`
	GitBranch            string         `json:"git_branch,omitempty"`
	LastFSActivity       *time.Time     `json:"last_fs_activity,omitempty"`
	LastTerminalActivity *time.Time     `json:"last_terminal_activity,omitempty"`
	LastGitActivity      *time.Time     `json:"last_git_activity,omitempty"`
	LatestActivity       *time.Time     `json:"latest_activity,omitempty"`
	DomainCounts         map[string]int `json:"domain_counts"`
	TotalEvents          int            `json:"total_events"`
	CheckpointCount      int            `json:"checkpoint_count"`
	HasCheckpoint        bool           `json:"has_checkpoint"`
}

// ProjectOverviewOptions configures filtering, sorting, and pagination for project discovery.
type ProjectOverviewOptions struct {
	SortMode ProjectSortMode
	Limit    int
	Window   time.Duration
	Since    *time.Time
}

// DiscoverProjects identifies projects known to LIFELOG across all recorded memory sources
// without recursively scanning the user's home directory or arbitrary filesystem.
func (db *DB) DiscoverProjects(ctx context.Context, opts ProjectOverviewOptions) ([]ProjectOverview, error) {
	// 1. Gather all candidate project roots from recorded SQLite sources
	candidateRoots, gitRepos, err := db.collectCandidateRoots(ctx)
	if err != nil {
		return nil, fmt.Errorf("collecting candidate project roots: %w", err)
	}

	if len(candidateRoots) == 0 {
		return []ProjectOverview{}, nil
	}

	// Sort roots by length descending for unambiguous most-specific attribution
	sort.Slice(candidateRoots, func(i, j int) bool {
		return len(candidateRoots[i]) > len(candidateRoots[j])
	})

	// Initialize project overview map keyed by canonical path
	projects := make(map[string]*ProjectOverview, len(candidateRoots))
	for _, root := range candidateRoots {
		dispName := filepath.Base(root)
		if dispName == "" || dispName == "." || dispName == string(filepath.Separator) {
			dispName = root
		}

		info, statErr := os.Stat(root)
		exists := statErr == nil && info.IsDir()

		proj := &ProjectOverview{
			CanonicalPath: root,
			DisplayName:   dispName,
			ExistsOnDisk:  exists,
			DomainCounts: map[string]int{
				"filesystem": 0,
				"git":        0,
				"terminal":   0,
			},
		}

		if repo, ok := gitRepos[root]; ok {
			proj.IsGitRegistered = true
			proj.GitRepoID = repo.ID
		}

		projects[root] = proj
	}

	// 2. Attribute Git observations
	if err := db.populateGitObservations(ctx, projects, opts.Since); err != nil {
		return nil, fmt.Errorf("populating git observations: %w", err)
	}

	// 3. Attribute Filesystem events
	if err := db.populateFilesystemObservations(ctx, candidateRoots, projects, opts.Since); err != nil {
		return nil, fmt.Errorf("populating filesystem observations: %w", err)
	}

	// 4. Attribute Terminal execution records
	if err := db.populateTerminalObservations(ctx, candidateRoots, projects, opts.Since); err != nil {
		return nil, fmt.Errorf("populating terminal observations: %w", err)
	}

	// 5. Attribute Checkpoint records
	if err := db.populateCheckpointObservations(ctx, candidateRoots, projects); err != nil {
		return nil, fmt.Errorf("populating checkpoint observations: %w", err)
	}

	// 6. Finalize metrics and compute latest activity
	result := make([]ProjectOverview, 0, len(projects))
	for _, p := range projects {
		p.TotalEvents = p.DomainCounts["filesystem"] + p.DomainCounts["git"] + p.DomainCounts["terminal"]
		p.HasCheckpoint = p.CheckpointCount > 0

		var latest *time.Time
		if p.LastFSActivity != nil {
			latest = p.LastFSActivity
		}
		if p.LastTerminalActivity != nil {
			if latest == nil || p.LastTerminalActivity.After(*latest) {
				latest = p.LastTerminalActivity
			}
		}
		if p.LastGitActivity != nil {
			if latest == nil || p.LastGitActivity.After(*latest) {
				latest = p.LastGitActivity
			}
		}
		p.LatestActivity = latest

		result = append(result, *p)
	}

	// 7. Sort projects
	sortProjects(result, opts.SortMode)

	// 8. Apply limit
	if opts.Limit > 0 && opts.Limit < len(result) {
		result = result[:opts.Limit]
	}

	return result, nil
}

func (db *DB) collectCandidateRoots(ctx context.Context) ([]string, map[string]GitRepoRecord, error) {
	gitRepos := make(map[string]GitRepoRecord)
	uniqueRoots := make(map[string]bool)

	// 1. Registered Git Repositories
	repoRows, err := db.sql.QueryContext(ctx, "SELECT id, path, is_bare, created_at, updated_at FROM git_repositories")
	if err != nil {
		return nil, nil, err
	}
	defer repoRows.Close()

	for repoRows.Next() {
		var r GitRepoRecord
		var cStr, uStr string
		if err := repoRows.Scan(&r.ID, &r.Path, &r.IsBare, &cStr, &uStr); err != nil {
			return nil, nil, err
		}
		canon, err := paths.Canonical(r.Path)
		if err == nil && isDefensibleRoot(canon) {
			uniqueRoots[canon] = true
			gitRepos[canon] = r
		}
	}
	if err := repoRows.Err(); err != nil {
		return nil, nil, err
	}

	// 2. Checkpoint project paths
	chkRows, err := db.sql.QueryContext(ctx, "SELECT DISTINCT project_path FROM checkpoints")
	if err != nil {
		return nil, nil, err
	}
	defer chkRows.Close()

	for chkRows.Next() {
		var p string
		if err := chkRows.Scan(&p); err == nil {
			canon, err := paths.Canonical(p)
			if err == nil && isDefensibleRoot(canon) {
				uniqueRoots[canon] = true
			}
		}
	}
	if err := chkRows.Err(); err != nil {
		return nil, nil, err
	}

	// 3. Terminal working directories
	termRows, err := db.sql.QueryContext(ctx, "SELECT DISTINCT working_dir FROM terminal_records")
	if err != nil {
		return nil, nil, err
	}
	defer termRows.Close()

	var nonGitTermRoots []string
	for termRows.Next() {
		var p string
		if err := termRows.Scan(&p); err == nil {
			canon, err := paths.Canonical(p)
			if err == nil && isDefensibleRoot(canon) {
				// If already in an established git repo or checkpoint, don't create separate root
				if !matchesAnyRoot(canon, uniqueRoots) {
					nonGitTermRoots = append(nonGitTermRoots, canon)
				}
			}
		}
	}
	if err := termRows.Err(); err != nil {
		return nil, nil, err
	}

	for _, tr := range nonGitTermRoots {
		if !matchesAnyRoot(tr, uniqueRoots) {
			uniqueRoots[tr] = true
		}
	}

	// 4. Filesystem events paths (directory events or file parent directories not covered by existing roots)
	fsRows, err := db.sql.QueryContext(ctx, "SELECT DISTINCT path, is_dir FROM filesystem_events")
	if err != nil {
		return nil, nil, err
	}
	defer fsRows.Close()

	for fsRows.Next() {
		var p string
		var isDir bool
		if err := fsRows.Scan(&p, &isDir); err == nil {
			dir := p
			if !isDir {
				dir = filepath.Dir(p)
			}
			canon, err := paths.Canonical(dir)
			if err == nil && isDefensibleRoot(canon) {
				if !matchesAnyRoot(canon, uniqueRoots) {
					uniqueRoots[canon] = true
				}
			}
		}
	}
	if err := fsRows.Err(); err != nil {
		return nil, nil, err
	}

	roots := make([]string, 0, len(uniqueRoots))
	for r := range uniqueRoots {
		roots = append(roots, r)
	}

	return roots, gitRepos, nil
}

func isDefensibleRoot(p string) bool {
	if p == "" || p == "/" || p == "." {
		return false
	}
	// Check against home directory
	if home, err := os.UserHomeDir(); err == nil {
		if filepath.Clean(home) == p {
			return false
		}
	}
	// Check against LIFELOG data directory if set
	if dataDir := os.Getenv(paths.EnvDataDir); dataDir != "" {
		cleanData := filepath.Clean(dataDir)
		if p == cleanData || paths.IsSubpath(cleanData, p) {
			return false
		}
	}
	return true
}

func matchesAnyRoot(target string, roots map[string]bool) bool {
	for r := range roots {
		if paths.IsSubpath(r, target) {
			return true
		}
	}
	return false
}

func attributeToMostSpecificRoot(target string, rootsSortedDesc []string) string {
	targetClean := filepath.Clean(target)
	targetEval := targetClean
	if eval, err := filepath.EvalSymlinks(targetClean); err == nil {
		targetEval = filepath.Clean(eval)
	} else if evalDir, err := filepath.EvalSymlinks(filepath.Dir(targetClean)); err == nil {
		targetEval = filepath.Clean(filepath.Join(evalDir, filepath.Base(targetClean)))
	}

	for _, root := range rootsSortedDesc {
		rootClean := filepath.Clean(root)
		rootEval := rootClean
		if eval, err := filepath.EvalSymlinks(rootClean); err == nil {
			rootEval = filepath.Clean(eval)
		}

		if paths.IsSubpath(rootClean, targetClean) ||
			paths.IsSubpath(rootEval, targetClean) ||
			paths.IsSubpath(rootClean, targetEval) ||
			paths.IsSubpath(rootEval, targetEval) {
			return root
		}
	}
	return ""
}

func (db *DB) populateGitObservations(ctx context.Context, projects map[string]*ProjectOverview, since *time.Time) error {
	for _, proj := range projects {
		if !proj.IsGitRegistered || proj.GitRepoID == "" {
			continue
		}
		repoID := proj.GitRepoID

		// Active branch from latest snapshot
		var branch string
		err := db.sql.QueryRowContext(ctx, "SELECT branch FROM git_snapshots WHERE repository_id = ? ORDER BY observed_at DESC LIMIT 1", repoID).Scan(&branch)
		if err == nil {
			proj.GitBranch = branch
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		// Most recent Git activity timestamp
		var maxCommitStr, maxSnapStr, maxLogStr sql.NullString
		_ = db.sql.QueryRowContext(ctx, "SELECT MAX(author_time) FROM git_commits WHERE repository_id = ?", repoID).Scan(&maxCommitStr)
		_ = db.sql.QueryRowContext(ctx, "SELECT MAX(observed_at) FROM git_snapshots WHERE repository_id = ?", repoID).Scan(&maxSnapStr)
		_ = db.sql.QueryRowContext(ctx, "SELECT MAX(observed_at) FROM git_branch_log WHERE repository_id = ?", repoID).Scan(&maxLogStr)

		var latestGit *time.Time
		for _, s := range []sql.NullString{maxCommitStr, maxSnapStr, maxLogStr} {
			if s.Valid && s.String != "" {
				t, err := parseTime(s.String)
				if err == nil {
					if latestGit == nil || t.After(*latestGit) {
						latestGit = &t
					}
				}
			}
		}
		proj.LastGitActivity = latestGit

		// Count Git events in window
		var commitCount, snapCount, logCount int
		if since != nil {
			sinceStr := since.UTC().Format(time.RFC3339Nano)
			_ = db.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM git_commits WHERE repository_id = ? AND author_time >= ?", repoID, sinceStr).Scan(&commitCount)
			_ = db.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM git_snapshots WHERE repository_id = ? AND observed_at >= ?", repoID, sinceStr).Scan(&snapCount)
			_ = db.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM git_branch_log WHERE repository_id = ? AND observed_at >= ?", repoID, sinceStr).Scan(&logCount)
		} else {
			_ = db.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM git_commits WHERE repository_id = ?", repoID).Scan(&commitCount)
			_ = db.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM git_snapshots WHERE repository_id = ?", repoID).Scan(&snapCount)
			_ = db.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM git_branch_log WHERE repository_id = ?", repoID).Scan(&logCount)
		}

		proj.DomainCounts["git"] = commitCount + snapCount + logCount
	}
	return nil
}

func (db *DB) populateFilesystemObservations(ctx context.Context, rootsDesc []string, projects map[string]*ProjectOverview, since *time.Time) error {
	rows, err := db.sql.QueryContext(ctx, "SELECT path, created_at FROM filesystem_events ORDER BY created_at ASC")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var pathStr, createdStr string
		if err := rows.Scan(&pathStr, &createdStr); err != nil {
			return err
		}
		t, err := parseTime(createdStr)
		if err != nil {
			continue
		}

		matchedRoot := attributeToMostSpecificRoot(pathStr, rootsDesc)
		if matchedRoot == "" {
			continue
		}

		proj, ok := projects[matchedRoot]
		if !ok {
			continue
		}

		if proj.LastFSActivity == nil || t.After(*proj.LastFSActivity) {
			tt := t
			proj.LastFSActivity = &tt
		}

		if since == nil || !t.Before(*since) {
			proj.DomainCounts["filesystem"]++
		}
	}
	return rows.Err()
}

func (db *DB) populateTerminalObservations(ctx context.Context, rootsDesc []string, projects map[string]*ProjectOverview, since *time.Time) error {
	rows, err := db.sql.QueryContext(ctx, "SELECT working_dir, start_time FROM terminal_records ORDER BY start_time ASC")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var dirStr, startStr string
		if err := rows.Scan(&dirStr, &startStr); err != nil {
			return err
		}
		t, err := parseTime(startStr)
		if err != nil {
			continue
		}

		matchedRoot := attributeToMostSpecificRoot(dirStr, rootsDesc)
		if matchedRoot == "" {
			continue
		}

		proj, ok := projects[matchedRoot]
		if !ok {
			continue
		}

		if proj.LastTerminalActivity == nil || t.After(*proj.LastTerminalActivity) {
			tt := t
			proj.LastTerminalActivity = &tt
		}

		if since == nil || !t.Before(*since) {
			proj.DomainCounts["terminal"]++
		}
	}
	return rows.Err()
}

func (db *DB) populateCheckpointObservations(ctx context.Context, rootsDesc []string, projects map[string]*ProjectOverview) error {
	rows, err := db.sql.QueryContext(ctx, "SELECT project_path FROM checkpoints")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return err
		}
		matchedRoot := attributeToMostSpecificRoot(p, rootsDesc)
		if matchedRoot != "" {
			if proj, ok := projects[matchedRoot]; ok {
				proj.CheckpointCount++
			}
		}
	}
	return rows.Err()
}

func sortProjects(projects []ProjectOverview, mode ProjectSortMode) {
	switch mode {
	case SortName:
		sort.Slice(projects, func(i, j int) bool {
			nameA := strings.ToLower(projects[i].DisplayName)
			nameB := strings.ToLower(projects[j].DisplayName)
			if nameA != nameB {
				return nameA < nameB
			}
			return projects[i].CanonicalPath < projects[j].CanonicalPath
		})
	case SortActivity:
		sort.Slice(projects, func(i, j int) bool {
			if projects[i].TotalEvents != projects[j].TotalEvents {
				return projects[i].TotalEvents > projects[j].TotalEvents
			}
			// Tie breaker: recent activity
			timeA := projects[i].LatestActivity
			timeB := projects[j].LatestActivity
			if timeA != nil && timeB != nil && !timeA.Equal(*timeB) {
				return timeA.After(*timeB)
			}
			if timeA != nil && timeB == nil {
				return true
			}
			if timeA == nil && timeB != nil {
				return false
			}
			nameA := strings.ToLower(projects[i].DisplayName)
			nameB := strings.ToLower(projects[j].DisplayName)
			if nameA != nameB {
				return nameA < nameB
			}
			return projects[i].CanonicalPath < projects[j].CanonicalPath
		})
	case SortRecent:
		fallthrough
	default:
		sort.Slice(projects, func(i, j int) bool {
			timeA := projects[i].LatestActivity
			timeB := projects[j].LatestActivity
			if timeA != nil && timeB != nil {
				if !timeA.Equal(*timeB) {
					return timeA.After(*timeB)
				}
			} else if timeA != nil && timeB == nil {
				return true
			} else if timeA == nil && timeB != nil {
				return false
			}
			// Both nil or equal timestamp: tie breaker by display name then canonical path
			nameA := strings.ToLower(projects[i].DisplayName)
			nameB := strings.ToLower(projects[j].DisplayName)
			if nameA != nameB {
				return nameA < nameB
			}
			return projects[i].CanonicalPath < projects[j].CanonicalPath
		})
	}
}
