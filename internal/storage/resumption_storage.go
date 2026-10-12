package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/paths"
	"github.com/lifelog/lifelog/internal/terminal"
)

// ProjectFilesystemSummary holds aggregated filesystem activity for a project path.
type ProjectFilesystemSummary struct {
	TotalCount   int
	TypeCounts   map[events.EventType]int
	RecentEvents []events.Event
}

// pathVariants returns canonical and symlink-aliased forms of a path on macOS (e.g. /var vs /private/var).
func pathVariants(p string) []string {
	clean := filepath.Clean(p)
	if !filepath.IsAbs(clean) {
		if abs, err := filepath.Abs(clean); err == nil {
			clean = abs
		}
	}
	variants := []string{clean}
	if eval, err := filepath.EvalSymlinks(clean); err == nil && eval != "" && eval != clean {
		variants = append(variants, eval)
	}
	for _, v := range variants {
		if strings.HasPrefix(v, "/private/var/") {
			alt := "/var/" + strings.TrimPrefix(v, "/private/var/")
			variants = append(variants, alt)
		} else if strings.HasPrefix(v, "/var/") {
			alt := "/private/var/" + strings.TrimPrefix(v, "/var/")
			variants = append(variants, alt)
		} else if strings.HasPrefix(v, "/private/tmp/") {
			alt := "/tmp/" + strings.TrimPrefix(v, "/private/tmp/")
			variants = append(variants, alt)
		} else if strings.HasPrefix(v, "/tmp/") {
			alt := "/private/tmp/" + strings.TrimPrefix(v, "/tmp/")
			variants = append(variants, alt)
		}
	}
	seen := make(map[string]bool)
	var deduped []string
	for _, v := range variants {
		if !seen[v] {
			seen[v] = true
			deduped = append(deduped, v)
		}
	}
	return deduped
}

// FindGitRepoForPath identifies the registered Git repository that covers canonicalPath.
// Handles macOS /var -> /private/var symlink equivalences and nested repositories.
func (db *DB) FindGitRepoForPath(ctx context.Context, targetPath string) (*GitRepoRecord, error) {
	rows, err := db.sql.QueryContext(ctx, "SELECT id, path, is_bare, created_at, updated_at FROM git_repositories ORDER BY length(path) DESC")
	if err != nil {
		return nil, fmt.Errorf("querying repositories: %w", err)
	}
	defer rows.Close()

	targets := pathVariants(targetPath)

	var bestRepo *GitRepoRecord
	for rows.Next() {
		var r GitRepoRecord
		var createdStr, updatedStr string
		if err := rows.Scan(&r.ID, &r.Path, &r.IsBare, &createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("scanning repository row: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		if r.CreatedAt.IsZero() {
			r.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		}
		r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
		if r.UpdatedAt.IsZero() {
			r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		}

		repoVariants := pathVariants(r.Path)

		matched := false
		for _, repoV := range repoVariants {
			for _, targetV := range targets {
				if paths.IsSubpath(repoV, targetV) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}

		if matched {
			bestRepo = &r
			break // Since sorted by length(path) DESC, first match is most specific
		}
	}

	return bestRepo, rows.Err()
}

// GetProjectFilesystemSummary aggregates filesystem events for a specific project path.
func (db *DB) GetProjectFilesystemSummary(ctx context.Context, projectPath string, since *time.Time, limit int) (*ProjectFilesystemSummary, error) {
	variants := pathVariants(projectPath)

	if limit <= 0 {
		limit = 20
	} else if limit > 500 {
		limit = 500
	}

	summary := &ProjectFilesystemSummary{
		TypeCounts: make(map[events.EventType]int),
	}

	var clauses []string
	var pathArgs []interface{}
	for _, v := range variants {
		clauses = append(clauses, "(path = ? OR path LIKE ? ESCAPE '\\')")
		pathArgs = append(pathArgs, v, escapeLike(v)+"/%")
	}
	pathClause := strings.Join(clauses, " OR ")
	if len(clauses) > 1 {
		pathClause = "(" + pathClause + ")"
	}

	// 1. Grouped counts by event_type
	countQuery := fmt.Sprintf("SELECT event_type, COUNT(*) FROM filesystem_events WHERE %s", pathClause)
	var countArgs []interface{}
	countArgs = append(countArgs, pathArgs...)
	if since != nil {
		countQuery += " AND created_at >= ?"
		countArgs = append(countArgs, since.UTC().Format(time.RFC3339Nano))
	}
	countQuery += " GROUP BY event_type"

	rows, err := db.sql.QueryContext(ctx, countQuery, countArgs...)
	if err != nil {
		return nil, fmt.Errorf("querying event counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var typeStr string
		var count int
		if err := rows.Scan(&typeStr, &count); err != nil {
			return nil, fmt.Errorf("scanning count row: %w", err)
		}
		summary.TypeCounts[events.EventType(typeStr)] = count
		summary.TotalCount += count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 2. Recent bounded events
	recentQuery := fmt.Sprintf("SELECT id, event_type, path, size, mod_time, is_dir, source, created_at FROM filesystem_events WHERE %s", pathClause)
	var recentArgs []interface{}
	recentArgs = append(recentArgs, pathArgs...)
	if since != nil {
		recentQuery += " AND created_at >= ?"
		recentArgs = append(recentArgs, since.UTC().Format(time.RFC3339Nano))
	}
	recentQuery += " ORDER BY created_at DESC, id DESC LIMIT ?"
	recentArgs = append(recentArgs, limit)

	eventRows, err := db.sql.QueryContext(ctx, recentQuery, recentArgs...)
	if err != nil {
		return nil, fmt.Errorf("querying recent events: %w", err)
	}
	defer eventRows.Close()

	for eventRows.Next() {
		var e events.Event
		var typeStr string
		var modTimeStr sql.NullString
		var sizeVal sql.NullInt64
		var createdAtStr string
		if err := eventRows.Scan(&e.ID, &typeStr, &e.Path, &sizeVal, &modTimeStr, &e.IsDir, &e.Source, &createdAtStr); err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}
		e.Type = events.EventType(typeStr)
		if sizeVal.Valid {
			e.Size = sizeVal.Int64
		}
		if modTimeStr.Valid {
			e.ModTime, _ = time.Parse(time.RFC3339Nano, modTimeStr.String)
			if e.ModTime.IsZero() {
				e.ModTime, _ = time.Parse(time.RFC3339, modTimeStr.String)
			}
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAtStr)
		if e.CreatedAt.IsZero() {
			e.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		}
		summary.RecentEvents = append(summary.RecentEvents, e)
	}

	return summary, eventRows.Err()
}

// ListTerminalRecordsForProject retrieves terminal execution records for a project directory.
func (db *DB) ListTerminalRecordsForProject(ctx context.Context, projectPath string, since *time.Time, limit int) ([]terminal.Record, error) {
	variants := pathVariants(projectPath)

	if limit <= 0 {
		limit = 20
	} else if limit > 500 {
		limit = 500
	}

	var clauses []string
	var dirArgs []interface{}
	for _, v := range variants {
		clauses = append(clauses, "(working_dir = ? OR working_dir LIKE ? ESCAPE '\\')")
		dirArgs = append(dirArgs, v, escapeLike(v)+"/%")
	}
	dirClause := strings.Join(clauses, " OR ")
	if len(clauses) > 1 {
		dirClause = "(" + dirClause + ")"
	}

	query := fmt.Sprintf(`
		SELECT id, working_dir, executable, sanitized_args, sanitized_command,
		       start_time, end_time, duration_ms, exit_code, collection_mode,
		       is_redacted, is_omitted, omission_reason, created_at
		FROM terminal_records
		WHERE %s
	`, dirClause)
	var args []interface{}
	args = append(args, dirArgs...)
	if since != nil {
		query += " AND start_time >= ?"
		args = append(args, since.UTC().Format(time.RFC3339Nano))
	}
	query += " ORDER BY start_time DESC, id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying terminal records for project: %w", err)
	}
	defer rows.Close()

	var records []terminal.Record
	for rows.Next() {
		var r terminal.Record
		var argsJSON string
		var exitCodeNull sql.NullInt64
		var startStr, endStr, createdStr string

		err := rows.Scan(
			&r.ID,
			&r.WorkingDir,
			&r.Executable,
			&argsJSON,
			&r.SanitizedCommand,
			&startStr,
			&endStr,
			&r.DurationMs,
			&exitCodeNull,
			&r.CollectionMode,
			&r.IsRedacted,
			&r.IsOmitted,
			&r.OmissionReason,
			&createdStr,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning terminal record: %w", err)
		}

		if exitCodeNull.Valid {
			code := int(exitCodeNull.Int64)
			r.ExitCode = &code
		}

		_ = json.Unmarshal([]byte(argsJSON), &r.SanitizedArgs)
		r.StartTime, _ = parseTime(startStr)
		r.EndTime, _ = parseTime(endStr)
		r.CreatedAt, _ = parseTime(createdStr)

		records = append(records, r)
	}

	return records, rows.Err()
}

// QueryGitCommitsSince returns commits for a repository with optional time filtering.
func (db *DB) QueryGitCommitsSince(ctx context.Context, repoID string, since *time.Time, limit int) ([]GitCommitRecord, error) {
	if limit <= 0 {
		limit = 20
	} else if limit > 500 {
		limit = 500
	}

	query := `
		SELECT repository_id, commit_hash, author_name, author_time, subject, discovered_at
		FROM git_commits
		WHERE repository_id = ?
	`
	var args []interface{}
	args = append(args, repoID)
	if since != nil {
		query += " AND author_time >= ?"
		args = append(args, since.UTC().Format(time.RFC3339Nano))
	}
	query += " ORDER BY author_time DESC, commit_hash DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying git commits: %w", err)
	}
	defer rows.Close()

	var commits []GitCommitRecord
	for rows.Next() {
		var c GitCommitRecord
		var authorStr, discoveredStr string
		if err := rows.Scan(&c.RepositoryID, &c.CommitHash, &c.AuthorName, &authorStr, &c.Subject, &discoveredStr); err != nil {
			return nil, fmt.Errorf("scanning commit row: %w", err)
		}
		c.AuthorTime, _ = time.Parse(time.RFC3339Nano, authorStr)
		if c.AuthorTime.IsZero() {
			c.AuthorTime, _ = time.Parse(time.RFC3339, authorStr)
		}
		c.DiscoveredAt, _ = time.Parse(time.RFC3339Nano, discoveredStr)
		if c.DiscoveredAt.IsZero() {
			c.DiscoveredAt, _ = time.Parse(time.RFC3339, discoveredStr)
		}
		commits = append(commits, c)
	}
	return commits, rows.Err()
}

// QueryGitBranchLogSince returns branch transitions for a repository with optional time filtering.
func (db *DB) QueryGitBranchLogSince(ctx context.Context, repoID string, since *time.Time, limit int) ([]GitBranchLogRecord, error) {
	if limit <= 0 {
		limit = 20
	} else if limit > 500 {
		limit = 500
	}

	query := `
		SELECT id, repository_id, branch_name, old_commit_hash, new_commit_hash, observed_at
		FROM git_branch_log
		WHERE repository_id = ?
	`
	var args []interface{}
	args = append(args, repoID)
	if since != nil {
		query += " AND observed_at >= ?"
		args = append(args, since.UTC().Format(time.RFC3339Nano))
	}
	query += " ORDER BY observed_at DESC, id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying git branch log: %w", err)
	}
	defer rows.Close()

	var logs []GitBranchLogRecord
	for rows.Next() {
		var l GitBranchLogRecord
		var observedStr string
		if err := rows.Scan(&l.ID, &l.RepositoryID, &l.BranchName, &l.OldCommitHash, &l.NewCommitHash, &observedStr); err != nil {
			return nil, err
		}
		l.ObservedAt, _ = time.Parse(time.RFC3339Nano, observedStr)
		if l.ObservedAt.IsZero() {
			l.ObservedAt, _ = time.Parse(time.RFC3339, observedStr)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}
