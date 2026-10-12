package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AppSession represents a discrete foreground application session.
type AppSession struct {
	ID          string    `json:"id"`
	AppBundleID string    `json:"app_bundle_id"`
	AppName     string    `json:"app_name"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	DurationMs  int64     `json:"duration_ms"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
}

// Validate checks that mandatory fields in AppSession are populated and valid.
func (s *AppSession) Validate() error {
	if s.ID == "" {
		return errors.New("app session ID cannot be empty")
	}
	if s.AppBundleID == "" && s.AppName == "" {
		return errors.New("app session must have bundle identifier or name")
	}
	if s.StartTime.IsZero() {
		return errors.New("app session start time cannot be zero")
	}
	if s.DurationMs < 0 {
		return errors.New("app session duration cannot be negative")
	}
	return nil
}

// AppUsageFilter defines time window and limits for aggregated app usage.
type AppUsageFilter struct {
	Since *time.Time
	Until *time.Time
	Limit int
}

// AppUsageStat represents aggregated usage statistics for a single application.
type AppUsageStat struct {
	AppBundleID     string  `json:"app_bundle_id"`
	AppName         string  `json:"app_name"`
	TotalDurationMs int64   `json:"total_duration_ms"`
	SessionCount    int     `json:"session_count"`
	Percentage      float64 `json:"percentage"`
}

// AppUsageSummary wraps aggregated application usage across a reporting period.
type AppUsageSummary struct {
	TotalDurationMs int64          `json:"total_duration_ms"`
	TotalSessions   int            `json:"total_sessions"`
	Since           time.Time      `json:"since"`
	Until           time.Time      `json:"until"`
	Apps            []AppUsageStat `json:"apps"`
}

// AppSessionFilter defines filtering and pagination criteria for querying sessions.
type AppSessionFilter struct {
	Since    *time.Time
	Until    *time.Time
	BundleID string
	Limit    int
	Offset   int
}

// RecordAppSession inserts a new app session into storage.
func (db *DB) RecordAppSession(ctx context.Context, s *AppSession) error {
	if s == nil {
		return errors.New("cannot record nil app session")
	}
	if err := s.Validate(); err != nil {
		return err
	}

	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}

	startStr := s.StartTime.UTC().Format(time.RFC3339Nano)
	endStr := s.EndTime.UTC().Format(time.RFC3339Nano)
	createdStr := s.CreatedAt.UTC().Format(time.RFC3339Nano)

	query := `
		INSERT INTO app_sessions (
			id, app_bundle_id, app_name, start_time, end_time, duration_ms, is_active, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.sql.ExecContext(ctx, query,
		s.ID,
		s.AppBundleID,
		s.AppName,
		startStr,
		endStr,
		s.DurationMs,
		s.IsActive,
		createdStr,
	)
	if err != nil {
		return fmt.Errorf("inserting app session %s: %w", s.ID, err)
	}
	return nil
}

// UpdateAppSession updates the end time, duration, and active state of an existing session.
func (db *DB) UpdateAppSession(ctx context.Context, id string, endTime time.Time, durationMs int64, isActive bool) error {
	if id == "" {
		return errors.New("app session ID cannot be empty")
	}
	if durationMs < 0 {
		durationMs = 0
	}

	endStr := endTime.UTC().Format(time.RFC3339Nano)
	query := `
		UPDATE app_sessions
		SET end_time = ?, duration_ms = ?, is_active = ?
		WHERE id = ?
	`
	res, err := db.sql.ExecContext(ctx, query, endStr, durationMs, isActive, id)
	if err != nil {
		return fmt.Errorf("updating app session %s: %w", id, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// GetActiveAppSession returns the most recent active session, if one exists.
func (db *DB) GetActiveAppSession(ctx context.Context) (*AppSession, error) {
	query := `
		SELECT id, app_bundle_id, app_name, start_time, end_time, duration_ms, is_active, created_at
		FROM app_sessions
		WHERE is_active = 1
		ORDER BY start_time DESC
		LIMIT 1
	`
	var s AppSession
	var startStr, endStr, createdStr string

	err := db.sql.QueryRowContext(ctx, query).Scan(
		&s.ID,
		&s.AppBundleID,
		&s.AppName,
		&startStr,
		&endStr,
		&s.DurationMs,
		&s.IsActive,
		&createdStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("querying active app session: %w", err)
	}

	s.StartTime, _ = time.Parse(time.RFC3339Nano, startStr)
	s.EndTime, _ = time.Parse(time.RFC3339Nano, endStr)
	s.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)

	return &s, nil
}

// CloseOpenAppSessions reconciles and closes open sessions after crashes or restarts.
// If an open session has end_time <= start_time, it caps duration to fallbackDuration.
func (db *DB) CloseOpenAppSessions(ctx context.Context, fallbackDuration time.Duration) (int64, error) {
	query := `
		SELECT id, start_time, end_time, duration_ms
		FROM app_sessions
		WHERE is_active = 1
	`
	rows, err := db.sql.QueryContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("querying open app sessions: %w", err)
	}
	defer rows.Close()

	type openItem struct {
		id         string
		startTime  time.Time
		endTime    time.Time
		durationMs int64
	}
	var items []openItem

	for rows.Next() {
		var item openItem
		var startStr, endStr string
		if err := rows.Scan(&item.id, &startStr, &endStr, &item.durationMs); err != nil {
			return 0, fmt.Errorf("scanning open app session: %w", err)
		}
		item.startTime, _ = time.Parse(time.RFC3339Nano, startStr)
		if item.startTime.IsZero() {
			item.startTime, _ = time.Parse(time.RFC3339, startStr)
		}
		item.endTime, _ = time.Parse(time.RFC3339Nano, endStr)
		if item.endTime.IsZero() {
			item.endTime, _ = time.Parse(time.RFC3339, endStr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if len(items) == 0 {
		return 0, nil
	}

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning tx to close open sessions: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmtUpdate, err := tx.PrepareContext(ctx, `
		UPDATE app_sessions
		SET end_time = ?, duration_ms = ?, is_active = 0
		WHERE id = ?
	`)
	if err != nil {
		return 0, fmt.Errorf("preparing session close statement: %w", err)
	}
	defer stmtUpdate.Close()

	var closedCount int64
	for _, it := range items {
		resolvedEnd := it.endTime
		if resolvedEnd.Before(it.startTime) || resolvedEnd.Equal(it.startTime) {
			if fallbackDuration > 0 {
				resolvedEnd = it.startTime.Add(fallbackDuration)
			} else {
				resolvedEnd = it.startTime
			}
		}

		dur := resolvedEnd.Sub(it.startTime).Milliseconds()
		if dur < 0 {
			dur = 0
		}

		endStr := resolvedEnd.UTC().Format(time.RFC3339Nano)
		if _, err := stmtUpdate.ExecContext(ctx, endStr, dur, it.id); err != nil {
			return 0, fmt.Errorf("closing session %s: %w", it.id, err)
		}
		closedCount++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing closed sessions tx: %w", err)
	}
	return closedCount, nil
}

// GetAppUsage returns usage grouped by application for the requested time window.
func (db *DB) GetAppUsage(ctx context.Context, filter AppUsageFilter) (*AppUsageSummary, error) {
	now := time.Now().UTC()
	since := now.Add(-24 * time.Hour)
	if filter.Since != nil && !filter.Since.IsZero() {
		since = filter.Since.UTC()
	}
	until := now
	if filter.Until != nil && !filter.Until.IsZero() {
		until = filter.Until.UTC()
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	sinceStr := since.Format(time.RFC3339Nano)
	untilStr := until.Format(time.RFC3339Nano)

	query := `
		SELECT
			app_bundle_id,
			app_name,
			SUM(duration_ms) as total_dur,
			COUNT(*) as sess_count
		FROM app_sessions
		WHERE start_time >= ? AND start_time <= ?
		GROUP BY app_bundle_id, app_name
		ORDER BY total_dur DESC
		LIMIT ?
	`

	rows, err := db.sql.QueryContext(ctx, query, sinceStr, untilStr, limit)
	if err != nil {
		return nil, fmt.Errorf("querying app usage: %w", err)
	}
	defer rows.Close()

	var stats []AppUsageStat
	var overallDuration int64
	var overallSessions int

	for rows.Next() {
		var stat AppUsageStat
		if err := rows.Scan(&stat.AppBundleID, &stat.AppName, &stat.TotalDurationMs, &stat.SessionCount); err != nil {
			return nil, fmt.Errorf("scanning app usage row: %w", err)
		}
		if stat.AppName == "" {
			stat.AppName = stat.AppBundleID
		}
		stats = append(stats, stat)
		overallDuration += stat.TotalDurationMs
		overallSessions += stat.SessionCount
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Compute percentage share
	if overallDuration > 0 {
		for i := range stats {
			stats[i].Percentage = (float64(stats[i].TotalDurationMs) / float64(overallDuration)) * 100.0
		}
	}

	return &AppUsageSummary{
		TotalDurationMs: overallDuration,
		TotalSessions:   overallSessions,
		Since:           since,
		Until:           until,
		Apps:            stats,
	}, nil
}

// GetAppSessions returns paginated individual application sessions.
func (db *DB) GetAppSessions(ctx context.Context, filter AppSessionFilter) ([]AppSession, int, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var whereClauses []string
	var args []any

	if filter.Since != nil && !filter.Since.IsZero() {
		whereClauses = append(whereClauses, "start_time >= ?")
		args = append(args, filter.Since.UTC().Format(time.RFC3339Nano))
	}
	if filter.Until != nil && !filter.Until.IsZero() {
		whereClauses = append(whereClauses, "start_time <= ?")
		args = append(args, filter.Until.UTC().Format(time.RFC3339Nano))
	}
	if filter.BundleID != "" {
		whereClauses = append(whereClauses, "app_bundle_id = ?")
		args = append(args, filter.BundleID)
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + joinSQL(whereClauses, " AND ")
	}

	// Count total matching
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM app_sessions %s", whereSQL)
	var totalCount int
	if err := db.sql.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("counting app sessions: %w", err)
	}

	// Query paginated
	dataQuery := fmt.Sprintf(`
		SELECT id, app_bundle_id, app_name, start_time, end_time, duration_ms, is_active, created_at
		FROM app_sessions
		%s
		ORDER BY start_time DESC
		LIMIT ? OFFSET ?
	`, whereSQL)

	queryArgs := append(args, limit, offset)
	rows, err := db.sql.QueryContext(ctx, dataQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying app sessions: %w", err)
	}
	defer rows.Close()

	var sessions []AppSession
	for rows.Next() {
		var s AppSession
		var startStr, endStr, createdStr string
		err := rows.Scan(
			&s.ID,
			&s.AppBundleID,
			&s.AppName,
			&startStr,
			&endStr,
			&s.DurationMs,
			&s.IsActive,
			&createdStr,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning app session: %w", err)
		}
		s.StartTime, _ = time.Parse(time.RFC3339Nano, startStr)
		if s.StartTime.IsZero() {
			s.StartTime, _ = time.Parse(time.RFC3339, startStr)
		}
		s.EndTime, _ = time.Parse(time.RFC3339Nano, endStr)
		if s.EndTime.IsZero() {
			s.EndTime, _ = time.Parse(time.RFC3339, endStr)
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		if s.CreatedAt.IsZero() {
			s.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return sessions, totalCount, nil
}

func joinSQL(items []string, sep string) string {
	res := ""
	for i, item := range items {
		if i > 0 {
			res += sep
		}
		res += item
	}
	return res
}
