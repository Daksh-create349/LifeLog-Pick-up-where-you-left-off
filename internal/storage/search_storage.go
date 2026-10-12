package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lifelog/lifelog/internal/search"
)

// Search executes a unified cross-domain search against the SQLite search index.
// It leverages FTS5 full-text indexing, supports domain and date filters,
// and falls back gracefully to parameterized substring search if FTS syntax errors occur.
func (db *DB) Search(ctx context.Context, q search.Query) (*search.SearchResult, error) {
	startTimer := time.Now()

	limit := q.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 500 {
		limit = 500
	}

	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	domainFilter := q.Domain
	if domainFilter == "" {
		domainFilter = search.DomainAll
	}

	rawText := strings.TrimSpace(q.Text)

	var items []search.ResultItem
	var err error

	if rawText == "" {
		// Empty query: chronological / filtered retrieval without text matching
		items, err = db.searchEmptyQuery(ctx, q, domainFilter)
	} else {
		// FTS5 query with fallback to parameterized LIKE search
		ftsQuery, tokens := search.BuildFTSQuery(rawText)
		items, err = db.searchFTS(ctx, q, rawText, ftsQuery, tokens, domainFilter)
		if err != nil {
			// Fallback to parameterized LIKE search
			items, err = db.searchLikeFallback(ctx, q, rawText, tokens, domainFilter)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	totalCount := len(items)

	// Sort items
	if q.Chronological {
		sort.Slice(items, func(i, j int) bool {
			if items[i].Timestamp.Equal(items[j].Timestamp) {
				return items[i].RecordID < items[j].RecordID
			}
			return items[i].Timestamp.Before(items[j].Timestamp)
		})
	} else if rawText != "" {
		sort.Slice(items, func(i, j int) bool {
			if items[i].Score == items[j].Score {
				return items[i].Timestamp.After(items[j].Timestamp)
			}
			return items[i].Score > items[j].Score
		})
	} else {
		// Non-chronological empty query defaults to newest first
		sort.Slice(items, func(i, j int) bool {
			if items[i].Timestamp.Equal(items[j].Timestamp) {
				return items[i].RecordID < items[j].RecordID
			}
			return items[i].Timestamp.After(items[j].Timestamp)
		})
	}

	// Apply offset and limit
	if offset > len(items) {
		items = []search.ResultItem{}
	} else {
		items = items[offset:]
		if len(items) > limit {
			items = items[:limit]
		}
	}

	durationMs := time.Since(startTimer).Milliseconds()

	return &search.SearchResult{
		Query:      q,
		Items:      items,
		TotalCount: totalCount,
		DurationMs: durationMs,
	}, nil
}

func (db *DB) searchFTS(
	ctx context.Context,
	q search.Query,
	rawText, ftsQuery string,
	tokens []string,
	domainFilter search.Domain,
) ([]search.ResultItem, error) {
	if ftsQuery == "" {
		return nil, fmt.Errorf("empty FTS query")
	}

	baseQuery := `
		SELECT domain, record_id, title, content, working_path, timestamp, rank
		FROM search_index
		WHERE search_index MATCH ?
	`
	var args []interface{}
	args = append(args, ftsQuery)

	if domainFilter != search.DomainAll {
		baseQuery += " AND domain = ?"
		args = append(args, string(domainFilter))
	}

	if q.StartTime != nil {
		baseQuery += " AND timestamp >= ?"
		args = append(args, q.StartTime.UTC().Format(time.RFC3339Nano))
	}

	if q.EndTime != nil {
		baseQuery += " AND timestamp <= ?"
		args = append(args, q.EndTime.UTC().Format(time.RFC3339Nano))
	}

	rows, err := db.sql.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSearchRows(rows, rawText, tokens)
}

func (db *DB) searchLikeFallback(
	ctx context.Context,
	q search.Query,
	rawText string,
	tokens []string,
	domainFilter search.Domain,
) ([]search.ResultItem, error) {
	baseQuery := `
		SELECT domain, record_id, title, content, working_path, timestamp, 0.0 AS rank
		FROM search_index
		WHERE (title LIKE ? ESCAPE '\' OR content LIKE ? ESCAPE '\' OR working_path LIKE ? ESCAPE '\')
	`
	pattern := "%" + escapeLike(rawText) + "%"
	var args []interface{}
	args = append(args, pattern, pattern, pattern)

	if domainFilter != search.DomainAll {
		baseQuery += " AND domain = ?"
		args = append(args, string(domainFilter))
	}

	if q.StartTime != nil {
		baseQuery += " AND timestamp >= ?"
		args = append(args, q.StartTime.UTC().Format(time.RFC3339Nano))
	}

	if q.EndTime != nil {
		baseQuery += " AND timestamp <= ?"
		args = append(args, q.EndTime.UTC().Format(time.RFC3339Nano))
	}

	rows, err := db.sql.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSearchRows(rows, rawText, tokens)
}

func (db *DB) searchEmptyQuery(
	ctx context.Context,
	q search.Query,
	domainFilter search.Domain,
) ([]search.ResultItem, error) {
	baseQuery := `
		SELECT domain, record_id, title, content, working_path, timestamp, 0.0 AS rank
		FROM search_index
		WHERE 1=1
	`
	var args []interface{}

	if domainFilter != search.DomainAll {
		baseQuery += " AND domain = ?"
		args = append(args, string(domainFilter))
	}

	if q.StartTime != nil {
		baseQuery += " AND timestamp >= ?"
		args = append(args, q.StartTime.UTC().Format(time.RFC3339Nano))
	}

	if q.EndTime != nil {
		baseQuery += " AND timestamp <= ?"
		args = append(args, q.EndTime.UTC().Format(time.RFC3339Nano))
	}

	rows, err := db.sql.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSearchRows(rows, "", nil)
}

func scanSearchRows(rows *sql.Rows, rawText string, tokens []string) ([]search.ResultItem, error) {
	var items []search.ResultItem
	seen := make(map[string]bool)

	for rows.Next() {
		var domainStr, recordID, title, content, workingPath, timeStr string
		var rank float64

		if err := rows.Scan(&domainStr, &recordID, &title, &content, &workingPath, &timeStr, &rank); err != nil {
			return nil, err
		}

		key := domainStr + ":" + recordID
		if seen[key] {
			continue
		}
		seen[key] = true

		ts, _ := parseTime(timeStr)
		score := search.CalculateScore(title, content, workingPath, rawText, tokens, rank)

		desc := content
		if len(desc) > 120 {
			desc = desc[:117] + "..."
		}

		item := search.ResultItem{
			Domain:      search.Domain(domainStr),
			RecordID:    recordID,
			Title:       title,
			Description: desc,
			WorkingPath: workingPath,
			Timestamp:   ts,
			Score:       score,
			Metadata: map[string]string{
				"working_path": workingPath,
			},
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

// RebuildSearchIndex clears and completely repopulates the search_index from all base tables.
func (db *DB) RebuildSearchIndex(ctx context.Context) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction to rebuild search index: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rebuildSQL := `
		DELETE FROM search_index;

		-- 1. Filesystem events
		INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
		SELECT 'filesystem', id, path, event_type || ' ' || source || ' ' || path, path, created_at
		FROM filesystem_events;

		-- 2. Git commits
		INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
		SELECT 'git', commit_hash, subject, author_name || ' ' || commit_hash || ' ' || subject || ' ' || r.path, r.path, c.author_time
		FROM git_commits c
		JOIN git_repositories r ON c.repository_id = r.id;

		-- 3. Git branches
		INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
		SELECT 'git', b.repository_id || ':' || b.name, 'branch: ' || b.name, b.name || ' ' || b.commit_hash || ' ' || r.path, r.path, b.observed_at
		FROM git_branches b
		JOIN git_repositories r ON b.repository_id = r.id;

		-- 4. Git branch log
		INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
		SELECT 'git', l.id, 'branch transition: ' || l.branch_name, l.branch_name || ' ' || coalesce(l.old_commit_hash, '') || ' ' || l.new_commit_hash || ' ' || r.path, r.path, l.observed_at
		FROM git_branch_log l
		JOIN git_repositories r ON l.repository_id = r.id;

		-- 5. Git repositories
		INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
		SELECT 'git', id, 'repository: ' || path, path, path, created_at
		FROM git_repositories;

		-- 6. Terminal records
		INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
		SELECT 'terminal', id, sanitized_command, executable || ' ' || sanitized_command || ' ' || working_dir, working_dir, start_time
		FROM terminal_records;
	`

	if _, err := tx.ExecContext(ctx, rebuildSQL); err != nil {
		return fmt.Errorf("rebuilding search index: %w", err)
	}

	return tx.Commit()
}
