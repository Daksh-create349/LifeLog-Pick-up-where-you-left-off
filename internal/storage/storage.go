package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/paths"
	_ "modernc.org/sqlite"
)

var (
	// ErrNotFound is returned when a requested record does not exist.
	ErrNotFound = errors.New("record not found")

	// ErrCorruptDatabase is returned when database integrity check fails.
	ErrCorruptDatabase = errors.New("database integrity check failed")
)

// DB encapsulates the local SQLite database connection and operations.
type DB struct {
	path string
	sql  *sql.DB
}

// Open opens or creates the SQLite database at dbPath, applies safety PRAGMAs,
// and runs pending schema migrations.
func Open(dbPath string) (*DB, error) {
	if dbPath == "" {
		return nil, errors.New("database path cannot be empty")
	}

	dir := filepath.Dir(dbPath)
	if err := paths.EnsureDir(dir); err != nil {
		return nil, fmt.Errorf("ensuring database directory for %s: %w", dbPath, err)
	}

	// SQLite connection string with busy timeout, WAL pragmas, and immediate txlock
	dsn := fmt.Sprintf("%s?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on&_synchronous=NORMAL&_txlock=immediate", dbPath)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database at %s: %w", dbPath, err)
	}

	// Configure pool for local desktop application
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("pinging sqlite database at %s: %w", dbPath, err)
	}

	db := &DB{
		path: dbPath,
		sql:  sqlDB,
	}

	if err := db.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("running migrations for %s: %w", dbPath, err)
	}

	// Ensure database file has restrictive 0600 permissions
	if info, err := os.Stat(dbPath); err == nil && !info.IsDir() {
		_ = os.Chmod(dbPath, paths.FilePerm)
	}

	return db, nil
}

// Close closes the underlying database handle.
func (db *DB) Close() error {
	if db.sql == nil {
		return nil
	}
	return db.sql.Close()
}

// Path returns the filesystem path of the database.
func (db *DB) Path() string {
	return db.path
}

// HealthCheck executes a ping, schema verification, and integrity check.
func (db *DB) HealthCheck(ctx context.Context) error {
	if err := db.sql.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Verify metadata table accessibility
	var count int
	if err := db.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM metadata").Scan(&count); err != nil {
		return fmt.Errorf("querying metadata table: %w", err)
	}

	// Run quick SQLite integrity check
	var result string
	if err := db.sql.QueryRowContext(ctx, "PRAGMA integrity_check(1)").Scan(&result); err != nil {
		return fmt.Errorf("running integrity check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("%w: %s", ErrCorruptDatabase, result)
	}

	return nil
}

// GetMetadata retrieves a key from the metadata table.
func (db *DB) GetMetadata(ctx context.Context, key string) (string, error) {
	var val string
	err := db.sql.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key = ?", key).Scan(&val)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("querying metadata key %q: %w", key, err)
	}
	return val, nil
}

// SetMetadata writes or updates a key in the metadata table.
func (db *DB) SetMetadata(ctx context.Context, key, value string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	query := `
		INSERT INTO metadata (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`
	_, err := db.sql.ExecContext(ctx, query, key, value, now)
	if err != nil {
		return fmt.Errorf("setting metadata key %q: %w", key, err)
	}
	return nil
}

// RecordEvents persists a batch of events within a transaction.
// Deduplication: if an event for the same path matches the most recent record's
// type, size, and mod_time, it is skipped to avoid duplicate notification noise.
func (db *DB) RecordEvents(ctx context.Context, evts []events.Event) error {
	if len(evts) == 0 {
		return nil
	}

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction for event batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmtCheck, err := tx.PrepareContext(ctx, `
		SELECT event_type, size, mod_time
		FROM filesystem_events
		WHERE path = ?
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`)
	if err != nil {
		return fmt.Errorf("preparing deduplication check: %w", err)
	}
	defer stmtCheck.Close()

	stmtInsert, err := tx.PrepareContext(ctx, `
		INSERT INTO filesystem_events (id, event_type, path, size, mod_time, is_dir, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing event insert: %w", err)
	}
	defer stmtInsert.Close()

	for _, e := range evts {
		if err := e.Validate(); err != nil {
			return fmt.Errorf("invalid event %s: %w", e.ID, err)
		}

		var lastType string
		var lastSize int64
		var lastModTimeStr string
		err := stmtCheck.QueryRowContext(ctx, e.Path).Scan(&lastType, &lastSize, &lastModTimeStr)
		if err == nil {
			lastModTime, _ := time.Parse(time.RFC3339Nano, lastModTimeStr)
			if lastModTime.IsZero() {
				lastModTime, _ = time.Parse(time.RFC3339, lastModTimeStr)
			}
			if lastType == string(e.Type) && lastSize == e.Size && lastModTime.Equal(e.ModTime) {
				continue
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("checking previous event for %s: %w", e.Path, err)
		}

		modTimeStr := e.ModTime.UTC().Format(time.RFC3339Nano)
		createdAtStr := e.CreatedAt.UTC().Format(time.RFC3339Nano)

		_, err = stmtInsert.ExecContext(ctx, e.ID, string(e.Type), e.Path, e.Size, modTimeStr, e.IsDir, e.Source, createdAtStr)
		if err != nil {
			return fmt.Errorf("inserting event %s: %w", e.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing event batch: %w", err)
	}
	return nil
}

// EventFilter defines search criteria for querying filesystem events.
type EventFilter struct {
	PathPrefix string
	Limit      int
}

// escapeLike escapes special SQLite LIKE wildcard characters ('%', '_', and '\').
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// QueryEvents queries stored events in deterministic reverse-chronological order.
func (db *DB) QueryEvents(ctx context.Context, filter EventFilter) ([]events.Event, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 1000 {
		limit = 1000
	}

	var query string
	var args []any

	if filter.PathPrefix != "" {
		cleanPrefix := filepath.Clean(filter.PathPrefix)
		query = `
			SELECT id, event_type, path, size, mod_time, is_dir, source, created_at
			FROM filesystem_events
			WHERE path = ? OR path LIKE ? ESCAPE '\'
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		`
		args = []any{cleanPrefix, escapeLike(cleanPrefix) + "/%", limit}
	} else {
		query = `
			SELECT id, event_type, path, size, mod_time, is_dir, source, created_at
			FROM filesystem_events
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		`
		args = []any{limit}
	}

	rows, err := db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying filesystem events: %w", err)
	}
	defer rows.Close()

	var result []events.Event
	for rows.Next() {
		var e events.Event
		var typeStr string
		var modTimeStr sql.NullString
		var sizeVal sql.NullInt64
		var createdAtStr string

		if err := rows.Scan(&e.ID, &typeStr, &e.Path, &sizeVal, &modTimeStr, &e.IsDir, &e.Source, &createdAtStr); err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}

		e.Type = events.EventType(typeStr)
		if sizeVal.Valid {
			e.Size = sizeVal.Int64
		}
		if modTimeStr.Valid {
			if t, err := time.Parse(time.RFC3339Nano, modTimeStr.String); err == nil {
				e.ModTime = t
			} else if t, err := time.Parse(time.RFC3339, modTimeStr.String); err == nil {
				e.ModTime = t
			}
		}
		if t, err := time.Parse(time.RFC3339Nano, createdAtStr); err == nil {
			e.CreatedAt = t
		} else if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
			e.CreatedAt = t
		}

		result = append(result, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating event rows: %w", err)
	}

	return result, nil
}

// SchemaVersion returns the latest applied migration version.
func (db *DB) SchemaVersion(ctx context.Context) (int, error) {
	var version int
	err := db.sql.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("querying schema version: %w", err)
	}
	return version, nil
}

type migrationDef struct {
	version int
	name    string
	apply   func(ctx context.Context, tx *sql.Tx) error
}

func (db *DB) migrate(ctx context.Context) error {
	initQuery := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMP NOT NULL
		);
	`
	if _, err := db.sql.ExecContext(ctx, initQuery); err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}

	currentVersion, err := db.SchemaVersion(ctx)
	if err != nil {
		return err
	}

	migrations := []migrationDef{
		{
			version: 1,
			name:    "initial_metadata_table",
			apply: func(ctx context.Context, tx *sql.Tx) error {
				schema := `
					CREATE TABLE IF NOT EXISTS metadata (
						key TEXT PRIMARY KEY,
						value TEXT NOT NULL,
						updated_at TIMESTAMP NOT NULL
					);
				`
				if _, err := tx.ExecContext(ctx, schema); err != nil {
					return fmt.Errorf("creating metadata table: %w", err)
				}

				now := time.Now().UTC().Format(time.RFC3339)
				seedQuery := `
					INSERT OR IGNORE INTO metadata (key, value, updated_at)
					VALUES ('initialized_at', ?, ?);
				`
				if _, err := tx.ExecContext(ctx, seedQuery, now, now); err != nil {
					return fmt.Errorf("seeding metadata: %w", err)
				}
				return nil
			},
		},
		{
			version: 2,
			name:    "filesystem_events_table",
			apply: func(ctx context.Context, tx *sql.Tx) error {
				schema := `
					CREATE TABLE IF NOT EXISTS filesystem_events (
						id TEXT PRIMARY KEY,
						event_type TEXT NOT NULL,
						path TEXT NOT NULL,
						size INTEGER NOT NULL DEFAULT 0,
						mod_time TIMESTAMP NOT NULL,
						is_dir BOOLEAN NOT NULL DEFAULT 0,
						source TEXT NOT NULL,
						created_at TIMESTAMP NOT NULL
					);
					CREATE INDEX IF NOT EXISTS idx_fs_events_path ON filesystem_events(path);
					CREATE INDEX IF NOT EXISTS idx_fs_events_created_at ON filesystem_events(created_at);
				`
				if _, err := tx.ExecContext(ctx, schema); err != nil {
					return fmt.Errorf("creating filesystem_events table: %w", err)
				}
				return nil
			},
		},
		{
			version: 3,
			name:    "git_memory_tables",
			apply: func(ctx context.Context, tx *sql.Tx) error {
				schema := `
					CREATE TABLE IF NOT EXISTS git_repositories (
						id TEXT PRIMARY KEY,
						path TEXT NOT NULL UNIQUE,
						is_bare BOOLEAN NOT NULL DEFAULT 0,
						created_at TIMESTAMP NOT NULL,
						updated_at TIMESTAMP NOT NULL
					);
					CREATE INDEX IF NOT EXISTS idx_git_repos_path ON git_repositories(path);

					CREATE TABLE IF NOT EXISTS git_commits (
						repository_id TEXT NOT NULL,
						commit_hash TEXT NOT NULL,
						author_name TEXT NOT NULL,
						author_time TIMESTAMP NOT NULL,
						subject TEXT NOT NULL,
						discovered_at TIMESTAMP NOT NULL,
						PRIMARY KEY (repository_id, commit_hash),
						FOREIGN KEY (repository_id) REFERENCES git_repositories(id) ON DELETE CASCADE
					);
					CREATE INDEX IF NOT EXISTS idx_git_commits_repo_time ON git_commits(repository_id, author_time);

					CREATE TABLE IF NOT EXISTS git_snapshots (
						id TEXT PRIMARY KEY,
						repository_id TEXT NOT NULL,
						branch TEXT NOT NULL,
						is_detached BOOLEAN NOT NULL DEFAULT 0,
						head_commit_hash TEXT,
						observed_at TIMESTAMP NOT NULL,
						FOREIGN KEY (repository_id) REFERENCES git_repositories(id) ON DELETE CASCADE
					);
					CREATE INDEX IF NOT EXISTS idx_git_snapshots_repo_time ON git_snapshots(repository_id, observed_at);

					CREATE TABLE IF NOT EXISTS git_working_tree_changes (
						snapshot_id TEXT NOT NULL,
						path TEXT NOT NULL,
						status TEXT NOT NULL,
						PRIMARY KEY (snapshot_id, path, status),
						FOREIGN KEY (snapshot_id) REFERENCES git_snapshots(id) ON DELETE CASCADE
					);

					CREATE TABLE IF NOT EXISTS git_branches (
						repository_id TEXT NOT NULL,
						name TEXT NOT NULL,
						commit_hash TEXT NOT NULL,
						observed_at TIMESTAMP NOT NULL,
						PRIMARY KEY (repository_id, name),
						FOREIGN KEY (repository_id) REFERENCES git_repositories(id) ON DELETE CASCADE
					);

					CREATE TABLE IF NOT EXISTS git_branch_log (
						id TEXT PRIMARY KEY,
						repository_id TEXT NOT NULL,
						branch_name TEXT NOT NULL,
						old_commit_hash TEXT,
						new_commit_hash TEXT NOT NULL,
						observed_at TIMESTAMP NOT NULL,
						FOREIGN KEY (repository_id) REFERENCES git_repositories(id) ON DELETE CASCADE
					);
					CREATE INDEX IF NOT EXISTS idx_git_branch_log_repo_time ON git_branch_log(repository_id, observed_at);
				`
				if _, err := tx.ExecContext(ctx, schema); err != nil {
					return fmt.Errorf("creating git memory tables: %w", err)
				}
				return nil
			},
		},
		{
			version: 4,
			name:    "terminal_records_table",
			apply: func(ctx context.Context, tx *sql.Tx) error {
				schema := `
					CREATE TABLE IF NOT EXISTS terminal_records (
						id TEXT PRIMARY KEY,
						working_dir TEXT NOT NULL,
						executable TEXT NOT NULL,
						sanitized_args TEXT NOT NULL,
						sanitized_command TEXT NOT NULL,
						start_time TIMESTAMP NOT NULL,
						end_time TIMESTAMP NOT NULL,
						duration_ms INTEGER NOT NULL,
						exit_code INTEGER,
						collection_mode TEXT NOT NULL,
						is_redacted BOOLEAN NOT NULL DEFAULT 0,
						is_omitted BOOLEAN NOT NULL DEFAULT 0,
						omission_reason TEXT NOT NULL DEFAULT '',
						created_at TIMESTAMP NOT NULL
					);
					CREATE INDEX IF NOT EXISTS idx_terminal_records_dir ON terminal_records(working_dir);
					CREATE INDEX IF NOT EXISTS idx_terminal_records_start ON terminal_records(start_time);
					CREATE INDEX IF NOT EXISTS idx_terminal_records_created ON terminal_records(created_at);
				`
				if _, err := tx.ExecContext(ctx, schema); err != nil {
					return fmt.Errorf("creating terminal_records table: %w", err)
				}
				return nil
			},
		},
		{
			version: 5,
			name:    "search_index_fts5",
			apply: func(ctx context.Context, tx *sql.Tx) error {
				schema := `
					CREATE VIRTUAL TABLE IF NOT EXISTS search_index USING fts5(
						domain UNINDEXED,
						record_id UNINDEXED,
						title,
						content,
						working_path,
						timestamp UNINDEXED
					);

					-- 1. Populate search_index from existing filesystem_events
					INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
					SELECT 'filesystem', id, path, event_type || ' ' || source || ' ' || path, path, created_at
					FROM filesystem_events;

					-- 2. Populate search_index from existing git_commits
					INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
					SELECT 'git', commit_hash, subject, author_name || ' ' || commit_hash || ' ' || subject || ' ' || r.path, r.path, c.author_time
					FROM git_commits c
					JOIN git_repositories r ON c.repository_id = r.id;

					-- 3. Populate search_index from existing git_branches
					INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
					SELECT 'git', b.repository_id || ':' || b.name, 'branch: ' || b.name, b.name || ' ' || b.commit_hash || ' ' || r.path, r.path, b.observed_at
					FROM git_branches b
					JOIN git_repositories r ON b.repository_id = r.id;

					-- 4. Populate search_index from existing git_branch_log
					INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
					SELECT 'git', l.id, 'branch transition: ' || l.branch_name, l.branch_name || ' ' || coalesce(l.old_commit_hash, '') || ' ' || l.new_commit_hash || ' ' || r.path, r.path, l.observed_at
					FROM git_branch_log l
					JOIN git_repositories r ON l.repository_id = r.id;

					-- 5. Populate search_index from existing git_repositories
					INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
					SELECT 'git', id, 'repository: ' || path, path, path, created_at
					FROM git_repositories;

					-- 6. Populate search_index from existing terminal_records
					INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
					SELECT 'terminal', id, sanitized_command, executable || ' ' || sanitized_command || ' ' || working_dir, working_dir, start_time
					FROM terminal_records;

					-- 7. Triggers for filesystem_events
					CREATE TRIGGER IF NOT EXISTS trg_fs_events_insert AFTER INSERT ON filesystem_events BEGIN
						INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
						VALUES ('filesystem', new.id, new.path, new.event_type || ' ' || new.source || ' ' || new.path, new.path, new.created_at);
					END;

					CREATE TRIGGER IF NOT EXISTS trg_fs_events_delete AFTER DELETE ON filesystem_events BEGIN
						DELETE FROM search_index WHERE record_id = old.id;
					END;

					-- 8. Triggers for git_commits
					CREATE TRIGGER IF NOT EXISTS trg_git_commits_insert AFTER INSERT ON git_commits BEGIN
						INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
						SELECT 'git', new.commit_hash, new.subject, new.author_name || ' ' || new.commit_hash || ' ' || new.subject || ' ' || r.path, r.path, new.author_time
						FROM git_repositories r WHERE r.id = new.repository_id;
					END;

					-- 9. Triggers for git_branches
					CREATE TRIGGER IF NOT EXISTS trg_git_branches_insert AFTER INSERT ON git_branches BEGIN
						INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
						SELECT 'git', new.repository_id || ':' || new.name, 'branch: ' || new.name, new.name || ' ' || new.commit_hash || ' ' || r.path, r.path, new.observed_at
						FROM git_repositories r WHERE r.id = new.repository_id;
					END;

					CREATE TRIGGER IF NOT EXISTS trg_git_branches_update AFTER UPDATE ON git_branches BEGIN
						DELETE FROM search_index WHERE record_id = old.repository_id || ':' || old.name;
						INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
						SELECT 'git', new.repository_id || ':' || new.name, 'branch: ' || new.name, new.name || ' ' || new.commit_hash || ' ' || r.path, r.path, new.observed_at
						FROM git_repositories r WHERE r.id = new.repository_id;
					END;

					-- 10. Triggers for git_branch_log
					CREATE TRIGGER IF NOT EXISTS trg_git_branch_log_insert AFTER INSERT ON git_branch_log BEGIN
						INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
						SELECT 'git', new.id, 'branch transition: ' || new.branch_name, new.branch_name || ' ' || coalesce(new.old_commit_hash, '') || ' ' || new.new_commit_hash || ' ' || r.path, r.path, new.observed_at
						FROM git_repositories r WHERE r.id = new.repository_id;
					END;

					-- 11. Triggers for git_repositories
					CREATE TRIGGER IF NOT EXISTS trg_git_repos_insert AFTER INSERT ON git_repositories BEGIN
						INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
						VALUES ('git', new.id, 'repository: ' || new.path, new.path, new.path, new.created_at);
					END;

					CREATE TRIGGER IF NOT EXISTS trg_git_repos_delete AFTER DELETE ON git_repositories BEGIN
						DELETE FROM search_index WHERE record_id = old.id;
					END;

					-- 12. Triggers for terminal_records
					CREATE TRIGGER IF NOT EXISTS trg_terminal_records_insert AFTER INSERT ON terminal_records BEGIN
						INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
						VALUES ('terminal', new.id, new.sanitized_command, new.executable || ' ' || new.sanitized_command || ' ' || new.working_dir, new.working_dir, new.start_time);
					END;

					CREATE TRIGGER IF NOT EXISTS trg_terminal_records_delete AFTER DELETE ON terminal_records BEGIN
						DELETE FROM search_index WHERE record_id = old.id;
					END;
				`
				if _, err := tx.ExecContext(ctx, schema); err != nil {
					return fmt.Errorf("creating search_index_fts5 and triggers: %w", err)
				}
				return nil
			},
		},
		{
			version: 6,
			name:    "checkpoints_table",
			apply: func(ctx context.Context, tx *sql.Tx) error {
				schema := `
					CREATE TABLE IF NOT EXISTS checkpoints (
						id TEXT PRIMARY KEY,
						project_path TEXT NOT NULL,
						label TEXT NOT NULL,
						created_at TIMESTAMP NOT NULL,
						format_version TEXT NOT NULL DEFAULT '1.0.0',
						payload TEXT NOT NULL
					);
					CREATE INDEX IF NOT EXISTS idx_checkpoints_project_created ON checkpoints(project_path, created_at);
				`
				if _, err := tx.ExecContext(ctx, schema); err != nil {
					return fmt.Errorf("creating checkpoints table: %w", err)
				}
				return nil
			},
		},
		{
			version: 7,
			name:    "fix_search_delete_triggers",
			apply: func(ctx context.Context, tx *sql.Tx) error {
				schema := `
					-- Trigger for git_commits delete
					CREATE TRIGGER IF NOT EXISTS trg_git_commits_delete AFTER DELETE ON git_commits BEGIN
						DELETE FROM search_index WHERE domain = 'git' AND record_id = old.commit_hash;
					END;

					-- Trigger for git_branches delete
					CREATE TRIGGER IF NOT EXISTS trg_git_branches_delete AFTER DELETE ON git_branches BEGIN
						DELETE FROM search_index WHERE domain = 'git' AND record_id = old.repository_id || ':' || old.name;
					END;

					-- Trigger for git_branch_log delete
					CREATE TRIGGER IF NOT EXISTS trg_git_branch_log_delete AFTER DELETE ON git_branch_log BEGIN
						DELETE FROM search_index WHERE domain = 'git' AND record_id = old.id;
					END;

					-- Trigger for git_repositories cascade delete on search index
					CREATE TRIGGER IF NOT EXISTS trg_git_repos_cascade_delete AFTER DELETE ON git_repositories BEGIN
						DELETE FROM search_index WHERE domain = 'git' AND working_path = old.path;
					END;
				`
				if _, err := tx.ExecContext(ctx, schema); err != nil {
					return fmt.Errorf("creating search delete triggers: %w", err)
				}
				return nil
			},
		},
		{
			version: 8,
			name:    "app_sessions_table",
			apply: func(ctx context.Context, tx *sql.Tx) error {
				schema := `
					CREATE TABLE IF NOT EXISTS app_sessions (
						id TEXT PRIMARY KEY,
						app_bundle_id TEXT NOT NULL,
						app_name TEXT NOT NULL,
						start_time TIMESTAMP NOT NULL,
						end_time TIMESTAMP NOT NULL,
						duration_ms INTEGER NOT NULL CHECK (duration_ms >= 0),
						is_active BOOLEAN NOT NULL DEFAULT 1,
						created_at TIMESTAMP NOT NULL
					);
					CREATE INDEX IF NOT EXISTS idx_app_sessions_start ON app_sessions(start_time);
					CREATE INDEX IF NOT EXISTS idx_app_sessions_bundle ON app_sessions(app_bundle_id);
					CREATE INDEX IF NOT EXISTS idx_app_sessions_name ON app_sessions(app_name);
					CREATE INDEX IF NOT EXISTS idx_app_sessions_active ON app_sessions(is_active);

					-- Search Index Triggers for app_sessions
					CREATE TRIGGER IF NOT EXISTS trg_app_sessions_insert AFTER INSERT ON app_sessions BEGIN
						INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
						VALUES ('app', new.id, new.app_name, new.app_name || ' ' || new.app_bundle_id, new.app_name, new.start_time);
					END;

					CREATE TRIGGER IF NOT EXISTS trg_app_sessions_update AFTER UPDATE ON app_sessions BEGIN
						DELETE FROM search_index WHERE domain = 'app' AND record_id = old.id;
						INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
						VALUES ('app', new.id, new.app_name, new.app_name || ' ' || new.app_bundle_id, new.app_name, new.start_time);
					END;

					CREATE TRIGGER IF NOT EXISTS trg_app_sessions_delete AFTER DELETE ON app_sessions BEGIN
						DELETE FROM search_index WHERE domain = 'app' AND record_id = old.id;
					END;
				`
				if _, err := tx.ExecContext(ctx, schema); err != nil {
					return fmt.Errorf("creating app_sessions table and triggers: %w", err)
				}
				return nil
			},
		},
		{
			version: 9,
			name:    "system_and_privacy_management",
			apply: func(ctx context.Context, tx *sql.Tx) error {
				schema := `
					CREATE TABLE IF NOT EXISTS system_events (
						id TEXT PRIMARY KEY,
						event_type TEXT NOT NULL,
						timestamp TIMESTAMP NOT NULL,
						created_at TIMESTAMP NOT NULL
					);
					CREATE INDEX IF NOT EXISTS idx_system_events_time ON system_events(timestamp);
					CREATE INDEX IF NOT EXISTS idx_system_events_type ON system_events(event_type);

					CREATE TRIGGER IF NOT EXISTS trg_system_events_insert AFTER INSERT ON system_events BEGIN
						INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
						VALUES ('system', new.id, 'System ' || new.event_type, 'System activity: ' || new.event_type, 'system', new.timestamp);
					END;

					CREATE TRIGGER IF NOT EXISTS trg_system_events_delete AFTER DELETE ON system_events BEGIN
						DELETE FROM search_index WHERE domain = 'system' AND record_id = old.id;
					END;

					CREATE TABLE IF NOT EXISTS browser_domains (
						id TEXT PRIMARY KEY,
						domain TEXT NOT NULL UNIQUE,
						visit_count INTEGER NOT NULL DEFAULT 1,
						last_visited TIMESTAMP NOT NULL,
						created_at TIMESTAMP NOT NULL
					);
					CREATE INDEX IF NOT EXISTS idx_browser_domains_domain ON browser_domains(domain);
					CREATE INDEX IF NOT EXISTS idx_browser_domains_visited ON browser_domains(last_visited);

					CREATE TRIGGER IF NOT EXISTS trg_browser_domains_insert AFTER INSERT ON browser_domains BEGIN
						INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
						VALUES ('browser', new.id, new.domain, 'Browser domain: ' || new.domain, new.domain, new.last_visited);
					END;

					CREATE TRIGGER IF NOT EXISTS trg_browser_domains_delete AFTER DELETE ON browser_domains BEGIN
						DELETE FROM search_index WHERE domain = 'browser' AND record_id = old.id;
					END;

					CREATE TABLE IF NOT EXISTS system_preferences (
						key TEXT PRIMARY KEY,
						value TEXT NOT NULL,
						updated_at TIMESTAMP NOT NULL
					);
				`
				if _, err := tx.ExecContext(ctx, schema); err != nil {
					return fmt.Errorf("creating system_and_privacy tables and triggers: %w", err)
				}
				return nil
			},
		},
	}

	for _, m := range migrations {
		if m.version <= currentVersion {
			continue
		}

		tx, err := db.sql.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("starting transaction for migration %d: %w", m.version, err)
		}

		if err := m.apply(ctx, tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("applying migration %d (%s): %w", m.version, m.name, err)
		}

		recordQuery := `INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`
		if _, err := tx.ExecContext(ctx, recordQuery, m.version, m.name, time.Now().UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", m.version, err)
		}
	}

	return nil
}
