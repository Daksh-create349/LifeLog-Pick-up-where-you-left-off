package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"
)

// GenerateID produces a unique prefixed identifier.
func GenerateID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%d_%s", prefix, time.Now().UnixNano(), hex.EncodeToString(b))
}

// SystemEventType represents the kind of macOS system activity observed.
type SystemEventType string

const (
	SystemEventSleep  SystemEventType = "sleep"
	SystemEventWake   SystemEventType = "wake"
	SystemEventLock   SystemEventType = "lock"
	SystemEventUnlock SystemEventType = "unlock"
)

// SystemEvent represents an individual system activity transition.
type SystemEvent struct {
	ID        string          `json:"id"`
	EventType SystemEventType `json:"event_type"`
	Timestamp time.Time       `json:"timestamp"`
	CreatedAt time.Time       `json:"created_at"`
}

// SystemEventFilter defines query parameters for fetching system events.
type SystemEventFilter struct {
	EventType SystemEventType
	Since     *time.Time
	Until     *time.Time
	Limit     int
	Offset    int
}

// SystemActivitySummary provides high-level metrics for system activity over a time window.
type SystemActivitySummary struct {
	TotalEvents int64        `json:"total_events"`
	SleepCount  int64        `json:"sleep_count"`
	WakeCount   int64        `json:"wake_count"`
	LockCount   int64        `json:"lock_count"`
	UnlockCount int64        `json:"unlock_count"`
	LastEvent   *SystemEvent `json:"last_event,omitempty"`
	Since       time.Time    `json:"since"`
	Until       time.Time    `json:"until"`
}

// BrowserDomain stores domain-only visits (strictly domain names e.g. "example.com", never full URLs).
type BrowserDomain struct {
	ID          string    `json:"id"`
	Domain      string    `json:"domain"`
	VisitCount  int64     `json:"visit_count"`
	LastVisited time.Time `json:"last_visited"`
	CreatedAt   time.Time `json:"created_at"`
}

// DataStats captures database file metrics and record counts for data management.
type DataStats struct {
	DBSizeBytes           int64      `json:"db_size_bytes"`
	AppSessionsCount      int64      `json:"app_sessions_count"`
	SystemEventsCount     int64      `json:"system_events_count"`
	FilesystemEventsCount int64      `json:"filesystem_events_count"`
	GitCommitsCount       int64      `json:"git_commits_count"`
	TerminalSessionsCount int64      `json:"terminal_sessions_count"`
	CheckpointsCount      int64      `json:"checkpoints_count"`
	BrowserDomainsCount   int64      `json:"browser_domains_count"`
	RetentionDays         int        `json:"retention_days"`
	LastRetentionCleanup  *time.Time `json:"last_retention_cleanup,omitempty"`
}

// PurgeStats records the number of deleted records across tables.
type PurgeStats struct {
	AppSessionsPurged      int64 `json:"app_sessions_purged"`
	SystemEventsPurged     int64 `json:"system_events_purged"`
	FilesystemEventsPurged int64 `json:"filesystem_events_purged"`
	TerminalSessionsPurged int64 `json:"terminal_sessions_purged"`
	BrowserDomainsPurged   int64 `json:"browser_domains_purged"`
	TotalPurged            int64 `json:"total_purged"`
}

// UserDataExport defines a complete, sanitized export of local LIFELOG data.
type UserDataExport struct {
	ExportVersion    string            `json:"export_version"`
	ExportedAt       time.Time         `json:"exported_at"`
	AppSessions      []AppSession      `json:"app_sessions"`
	SystemEvents     []*SystemEvent    `json:"system_events"`
	BrowserDomains   []*BrowserDomain  `json:"browser_domains"`
	FilesystemEvents []ExportFSEvent   `json:"filesystem_events"`
	TerminalSessions []ExportTermEvent `json:"terminal_sessions"`
}

// ExportFSEvent is an export-safe representation of a filesystem event.
type ExportFSEvent struct {
	ID        string    `json:"id"`
	EventType string    `json:"event_type"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"mod_time"`
	CreatedAt time.Time `json:"created_at"`
}

// ExportTermEvent is an export-safe representation of a terminal command.
type ExportTermEvent struct {
	ID               string    `json:"id"`
	SanitizedCommand string    `json:"sanitized_command"`
	ExitCode         int       `json:"exit_code"`
	DurationMs       int64     `json:"duration_ms"`
	StartedAt        time.Time `json:"started_at"`
}

// RecordSystemEvent inserts a system event into the database.
func (db *DB) RecordSystemEvent(ctx context.Context, evt *SystemEvent) error {
	if evt == nil {
		return fmt.Errorf("system event cannot be nil")
	}
	if evt.ID == "" {
		evt.ID = GenerateID("sys_evt")
	}
	if evt.CreatedAt.IsZero() {
		evt.CreatedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO system_events (id, event_type, timestamp, created_at)
		VALUES (?, ?, ?, ?)
	`
	_, err := db.sql.ExecContext(ctx, query,
		evt.ID,
		string(evt.EventType),
		evt.Timestamp.UTC().Format(time.RFC3339Nano),
		evt.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("recording system event: %w", err)
	}
	return nil
}

// GetSystemEvents retrieves system events matching the filter.
func (db *DB) GetSystemEvents(ctx context.Context, filter SystemEventFilter) ([]*SystemEvent, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	var conditions []string
	var args []any

	if filter.EventType != "" {
		conditions = append(conditions, "event_type = ?")
		args = append(args, string(filter.EventType))
	}
	if filter.Since != nil {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, filter.Since.UTC().Format(time.RFC3339Nano))
	}
	if filter.Until != nil {
		conditions = append(conditions, "timestamp <= ?")
		args = append(args, filter.Until.UTC().Format(time.RFC3339Nano))
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT id, event_type, timestamp, created_at
		FROM system_events
		%s
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	args = append(args, limit, filter.Offset)

	rows, err := db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying system events: %w", err)
	}
	defer rows.Close()

	var events []*SystemEvent
	for rows.Next() {
		var evt SystemEvent
		var typeStr, tsStr, createdStr string

		if err := rows.Scan(&evt.ID, &typeStr, &tsStr, &createdStr); err != nil {
			return nil, fmt.Errorf("scanning system event: %w", err)
		}
		evt.EventType = SystemEventType(typeStr)
		if parsed, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
			evt.Timestamp = parsed
		} else if parsed, err := time.Parse(time.RFC3339, tsStr); err == nil {
			evt.Timestamp = parsed
		}
		if parsed, err := time.Parse(time.RFC3339Nano, createdStr); err == nil {
			evt.CreatedAt = parsed
		} else if parsed, err := time.Parse(time.RFC3339, createdStr); err == nil {
			evt.CreatedAt = parsed
		}
		events = append(events, &evt)
	}

	return events, nil
}

// GetSystemActivitySummary computes summary counts for system activity over a time window.
func (db *DB) GetSystemActivitySummary(ctx context.Context, since, until time.Time) (*SystemActivitySummary, error) {
	if until.IsZero() {
		until = time.Now().UTC()
	}
	if since.IsZero() {
		since = until.Add(-24 * time.Hour)
	}

	summary := &SystemActivitySummary{
		Since: since,
		Until: until,
	}

	query := `
		SELECT event_type, COUNT(*)
		FROM system_events
		WHERE timestamp >= ? AND timestamp <= ?
		GROUP BY event_type
	`
	rows, err := db.sql.QueryContext(ctx, query,
		since.UTC().Format(time.RFC3339Nano),
		until.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, fmt.Errorf("summarizing system events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var et string
		var count int64
		if err := rows.Scan(&et, &count); err != nil {
			return nil, err
		}
		summary.TotalEvents += count
		switch SystemEventType(et) {
		case SystemEventSleep:
			summary.SleepCount = count
		case SystemEventWake:
			summary.WakeCount = count
		case SystemEventLock:
			summary.LockCount = count
		case SystemEventUnlock:
			summary.UnlockCount = count
		}
	}

	// Fetch last event
	lastEvts, err := db.GetSystemEvents(ctx, SystemEventFilter{Limit: 1})
	if err == nil && len(lastEvts) > 0 {
		summary.LastEvent = lastEvts[0]
	}

	return summary, nil
}

// RecordBrowserDomain records or increments a visited domain (domain only, no URLs or paths).
func (db *DB) RecordBrowserDomain(ctx context.Context, domain string, visitedAt time.Time) error {
	cleanDomain := strings.ToLower(strings.TrimSpace(domain))
	if cleanDomain == "" {
		return nil
	}
	// Strip any path or scheme if inadvertently passed
	if idx := strings.Index(cleanDomain, "://"); idx != -1 {
		cleanDomain = cleanDomain[idx+3:]
	}
	if idx := strings.Index(cleanDomain, "/"); idx != -1 {
		cleanDomain = cleanDomain[:idx]
	}
	if idx := strings.Index(cleanDomain, ":"); idx != -1 {
		cleanDomain = cleanDomain[:idx]
	}

	if visitedAt.IsZero() {
		visitedAt = time.Now().UTC()
	}

	// Check if unique index exists or manual upsert
	var existingID string
	err := db.sql.QueryRowContext(ctx, `SELECT id FROM browser_domains WHERE domain = ?`, cleanDomain).Scan(&existingID)
	if err == sql.ErrNoRows {
		id := GenerateID("bdom")
		_, err = db.sql.ExecContext(ctx, `
			INSERT INTO browser_domains (id, domain, visit_count, last_visited, created_at)
			VALUES (?, ?, 1, ?, ?)
		`, id, cleanDomain, visitedAt.UTC().Format(time.RFC3339Nano), visitedAt.UTC().Format(time.RFC3339Nano))
		return err
	} else if err != nil {
		return err
	}

	_, err = db.sql.ExecContext(ctx, `
		UPDATE browser_domains
		SET visit_count = visit_count + 1, last_visited = ?
		WHERE id = ?
	`, visitedAt.UTC().Format(time.RFC3339Nano), existingID)
	return err
}

// GetBrowserDomains retrieves recorded domains sorted by visit count or recency.
func (db *DB) GetBrowserDomains(ctx context.Context, limit int) ([]*BrowserDomain, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, domain, visit_count, last_visited, created_at
		FROM browser_domains
		ORDER BY visit_count DESC, last_visited DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying browser domains: %w", err)
	}
	defer rows.Close()

	var domains []*BrowserDomain
	for rows.Next() {
		var bd BrowserDomain
		var lastStr, createdStr string
		if err := rows.Scan(&bd.ID, &bd.Domain, &bd.VisitCount, &lastStr, &createdStr); err != nil {
			return nil, err
		}
		if p, err := time.Parse(time.RFC3339Nano, lastStr); err == nil {
			bd.LastVisited = p
		}
		if p, err := time.Parse(time.RFC3339Nano, createdStr); err == nil {
			bd.CreatedAt = p
		}
		domains = append(domains, &bd)
	}
	return domains, nil
}

// Preference methods
func (db *DB) SetPreference(ctx context.Context, key, val string) error {
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	query := `
		INSERT INTO system_preferences (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`
	_, err := db.sql.ExecContext(ctx, query, key, val, nowStr)
	return err
}

func (db *DB) GetPreference(ctx context.Context, key string, defaultVal string) (string, error) {
	var val string
	err := db.sql.QueryRowContext(ctx, `SELECT value FROM system_preferences WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return defaultVal, nil
	}
	if err != nil {
		return defaultVal, err
	}
	return val, nil
}

// GetDataStats computes record counts and file size for data management.
func (db *DB) GetDataStats(ctx context.Context) (*DataStats, error) {
	stats := &DataStats{
		RetentionDays: 30, // default
	}

	if db.path != "" {
		if fi, err := os.Stat(db.path); err == nil {
			stats.DBSizeBytes = fi.Size()
		}
	}

	// Record counts
	_ = db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_sessions`).Scan(&stats.AppSessionsCount)
	_ = db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM system_events`).Scan(&stats.SystemEventsCount)
	_ = db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM filesystem_events`).Scan(&stats.FilesystemEventsCount)
	_ = db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM git_commits`).Scan(&stats.GitCommitsCount)
	_ = db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM terminal_sessions`).Scan(&stats.TerminalSessionsCount)
	_ = db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM checkpoints`).Scan(&stats.CheckpointsCount)
	_ = db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM browser_domains`).Scan(&stats.BrowserDomainsCount)

	// Preferences
	var retStr string
	if err := db.sql.QueryRowContext(ctx, `SELECT value FROM system_preferences WHERE key = 'retention_days'`).Scan(&retStr); err == nil {
		var rd int
		if _, err := fmt.Sscanf(retStr, "%d", &rd); err == nil {
			stats.RetentionDays = rd
		}
	}

	var lastCleanStr string
	if err := db.sql.QueryRowContext(ctx, `SELECT value FROM system_preferences WHERE key = 'last_retention_cleanup'`).Scan(&lastCleanStr); err == nil {
		if t, err := time.Parse(time.RFC3339Nano, lastCleanStr); err == nil {
			stats.LastRetentionCleanup = &t
		}
	}

	return stats, nil
}

// PurgeRetention purges records older than retentionDays (if retentionDays > 0).
func (db *DB) PurgeRetention(ctx context.Context, retentionDays int) (*PurgeStats, error) {
	if retentionDays <= 0 {
		return &PurgeStats{}, nil // 0 means keep forever
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	cutoffStr := cutoff.Format(time.RFC3339Nano)

	stats := &PurgeStats{}

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting purge transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. App sessions older than cutoff and not active
	res, err := tx.ExecContext(ctx, `DELETE FROM app_sessions WHERE end_time < ? AND is_active = 0`, cutoffStr)
	if err == nil {
		stats.AppSessionsPurged, _ = res.RowsAffected()
	}

	// 2. System events
	res, err = tx.ExecContext(ctx, `DELETE FROM system_events WHERE timestamp < ?`, cutoffStr)
	if err == nil {
		stats.SystemEventsPurged, _ = res.RowsAffected()
	}

	// 3. Filesystem events
	res, err = tx.ExecContext(ctx, `DELETE FROM filesystem_events WHERE created_at < ?`, cutoffStr)
	if err == nil {
		stats.FilesystemEventsPurged, _ = res.RowsAffected()
	}

	// 4. Terminal sessions
	res, err = tx.ExecContext(ctx, `DELETE FROM terminal_sessions WHERE started_at < ?`, cutoffStr)
	if err == nil {
		stats.TerminalSessionsPurged, _ = res.RowsAffected()
	}

	stats.TotalPurged = stats.AppSessionsPurged + stats.SystemEventsPurged + stats.FilesystemEventsPurged + stats.TerminalSessionsPurged

	// Record last cleanup timestamp
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	_, _ = tx.ExecContext(ctx, `
		INSERT INTO system_preferences (key, value, updated_at)
		VALUES ('last_retention_cleanup', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, nowStr, nowStr)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing purge transaction: %w", err)
	}

	return stats, nil
}

// DeleteActivity selectively clears recorded activity for a given domain/source and optional date range.
func (db *DB) DeleteActivity(ctx context.Context, source string, since, until *time.Time) (int64, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("starting delete transaction: %w", err)
	}
	defer tx.Rollback()

	var totalDeleted int64

	buildTimeWhere := func(col string) (string, []any) {
		var conds []string
		var args []any
		if since != nil {
			conds = append(conds, col+" >= ?")
			args = append(args, since.UTC().Format(time.RFC3339Nano))
		}
		if until != nil {
			conds = append(conds, col+" <= ?")
			args = append(args, until.UTC().Format(time.RFC3339Nano))
		}
		if len(conds) == 0 {
			return "", nil
		}
		return " WHERE " + strings.Join(conds, " AND "), args
	}

	deleteFromTable := func(table, timeCol string) error {
		where, args := buildTimeWhere(timeCol)
		q := fmt.Sprintf("DELETE FROM %s%s", table, where)
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		totalDeleted += n
		return nil
	}

	switch strings.ToLower(source) {
	case "all":
		_ = deleteFromTable("app_sessions", "start_time")
		_ = deleteFromTable("system_events", "timestamp")
		_ = deleteFromTable("browser_domains", "last_visited")
		_ = deleteFromTable("filesystem_events", "created_at")
		_ = deleteFromTable("terminal_sessions", "started_at")
		_ = deleteFromTable("git_commits", "committed_at")
		_ = deleteFromTable("checkpoints", "created_at")
	case "apps":
		if err := deleteFromTable("app_sessions", "start_time"); err != nil {
			return 0, err
		}
	case "system":
		if err := deleteFromTable("system_events", "timestamp"); err != nil {
			return 0, err
		}
	case "browser":
		if err := deleteFromTable("browser_domains", "last_visited"); err != nil {
			return 0, err
		}
	case "filesystem":
		if err := deleteFromTable("filesystem_events", "created_at"); err != nil {
			return 0, err
		}
	case "terminal":
		if err := deleteFromTable("terminal_sessions", "started_at"); err != nil {
			return 0, err
		}
	case "git":
		if err := deleteFromTable("git_commits", "committed_at"); err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("unknown deletion source: %q", source)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing deletion transaction: %w", err)
	}

	return totalDeleted, nil
}

// ExportUserData extracts all user activity data for local export (strictly sanitized).
func (db *DB) ExportUserData(ctx context.Context) (*UserDataExport, error) {
	export := &UserDataExport{
		ExportVersion: "1.0",
		ExportedAt:    time.Now().UTC(),
	}

	// App sessions
	sessions, _, err := db.GetAppSessions(ctx, AppSessionFilter{Limit: 10000})
	if err == nil {
		export.AppSessions = sessions
	}

	// System events
	sysEvents, err := db.GetSystemEvents(ctx, SystemEventFilter{Limit: 10000})
	if err == nil {
		export.SystemEvents = sysEvents
	}

	// Browser domains
	bDomains, err := db.GetBrowserDomains(ctx, 1000)
	if err == nil {
		export.BrowserDomains = bDomains
	}

	// Filesystem events (up to 10000)
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, event_type, path, size, mod_time, created_at
		FROM filesystem_events
		ORDER BY created_at DESC
		LIMIT 10000
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var e ExportFSEvent
			var modStr, crStr sql.NullString
			var sizeVal sql.NullInt64
			if err := rows.Scan(&e.ID, &e.EventType, &e.Path, &sizeVal, &modStr, &crStr); err == nil {
				e.Size = sizeVal.Int64
				if modStr.Valid {
					e.ModTime, _ = time.Parse(time.RFC3339Nano, modStr.String)
				}
				if crStr.Valid {
					e.CreatedAt, _ = time.Parse(time.RFC3339Nano, crStr.String)
				}
				export.FilesystemEvents = append(export.FilesystemEvents, e)
			}
		}
	}

	// Terminal sessions
	termRows, err := db.sql.QueryContext(ctx, `
		SELECT id, sanitized_command, exit_code, duration_ms, started_at
		FROM terminal_sessions
		ORDER BY started_at DESC
		LIMIT 10000
	`)
	if err == nil {
		defer termRows.Close()
		for termRows.Next() {
			var t ExportTermEvent
			var stStr string
			if err := termRows.Scan(&t.ID, &t.SanitizedCommand, &t.ExitCode, &t.DurationMs, &stStr); err == nil {
				t.StartedAt, _ = time.Parse(time.RFC3339Nano, stStr)
				export.TerminalSessions = append(export.TerminalSessions, t)
			}
		}
	}

	return export, nil
}
