package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lifelog/lifelog/internal/terminal"
)

// RecordTerminalExecution persists a terminal execution event to the database.
func (db *DB) RecordTerminalExecution(ctx context.Context, rec *terminal.Record) error {
	if rec == nil {
		return errors.New("cannot record nil terminal record")
	}
	if rec.ID == "" {
		return errors.New("terminal record ID cannot be empty")
	}
	if rec.WorkingDir == "" {
		return errors.New("terminal record working directory cannot be empty")
	}

	argsJSON, err := json.Marshal(rec.SanitizedArgs)
	if err != nil {
		argsJSON = []byte("[]")
	}

	var exitCodeNull sql.NullInt64
	if rec.ExitCode != nil {
		exitCodeNull = sql.NullInt64{Int64: int64(*rec.ExitCode), Valid: true}
	}

	startStr := rec.StartTime.UTC().Format(time.RFC3339Nano)
	endStr := rec.EndTime.UTC().Format(time.RFC3339Nano)
	createdStr := rec.CreatedAt.UTC().Format(time.RFC3339Nano)

	query := `
		INSERT INTO terminal_records (
			id, working_dir, executable, sanitized_args, sanitized_command,
			start_time, end_time, duration_ms, exit_code, collection_mode,
			is_redacted, is_omitted, omission_reason, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = db.sql.ExecContext(ctx, query,
		rec.ID,
		rec.WorkingDir,
		rec.Executable,
		string(argsJSON),
		rec.SanitizedCommand,
		startStr,
		endStr,
		rec.DurationMs,
		exitCodeNull,
		rec.CollectionMode,
		rec.IsRedacted,
		rec.IsOmitted,
		rec.OmissionReason,
		createdStr,
	)
	if err != nil {
		return fmt.Errorf("inserting terminal record %s: %w", rec.ID, err)
	}

	return nil
}

// ListTerminalRecords queries terminal records according to the specified filter.
func (db *DB) ListTerminalRecords(ctx context.Context, filter terminal.RecordFilter) ([]terminal.Record, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT id, working_dir, executable, sanitized_args, sanitized_command,
		       start_time, end_time, duration_ms, exit_code, collection_mode,
		       is_redacted, is_omitted, omission_reason, created_at
		FROM terminal_records
	`

	var args []interface{}
	if filter.WorkingDir != "" {
		query += " WHERE working_dir = ? OR working_dir LIKE ?"
		args = append(args, filter.WorkingDir, filter.WorkingDir+"/%")
	}

	if filter.Chronological {
		query += " ORDER BY start_time ASC, id ASC"
	} else {
		query += " ORDER BY start_time DESC, id DESC"
	}

	query += " LIMIT ?"
	args = append(args, limit)

	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying terminal records: %w", err)
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating terminal records: %w", err)
	}

	return records, nil
}

// GetTerminalRecord retrieves a single terminal execution record by ID.
func (db *DB) GetTerminalRecord(ctx context.Context, id string) (*terminal.Record, error) {
	query := `
		SELECT id, working_dir, executable, sanitized_args, sanitized_command,
		       start_time, end_time, duration_ms, exit_code, collection_mode,
		       is_redacted, is_omitted, omission_reason, created_at
		FROM terminal_records
		WHERE id = ?
		LIMIT 1
	`

	var r terminal.Record
	var argsJSON string
	var exitCodeNull sql.NullInt64
	var startStr, endStr, createdStr string

	err := db.sql.QueryRowContext(ctx, query, id).Scan(
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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying terminal record %s: %w", id, err)
	}

	if exitCodeNull.Valid {
		code := int(exitCodeNull.Int64)
		r.ExitCode = &code
	}

	_ = json.Unmarshal([]byte(argsJSON), &r.SanitizedArgs)
	r.StartTime, _ = parseTime(startStr)
	r.EndTime, _ = parseTime(endStr)
	r.CreatedAt, _ = parseTime(createdStr)

	return &r, nil
}

// DeleteTerminalRecord deletes a specific terminal execution record by ID.
// It returns true if a record was deleted, or false if not found.
func (db *DB) DeleteTerminalRecord(ctx context.Context, id string) (bool, error) {
	res, err := db.sql.ExecContext(ctx, "DELETE FROM terminal_records WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting terminal record %s: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// ClearTerminalRecords removes all recorded terminal executions.
// It returns the count of removed records.
func (db *DB) ClearTerminalRecords(ctx context.Context) (int64, error) {
	res, err := db.sql.ExecContext(ctx, "DELETE FROM terminal_records")
	if err != nil {
		return 0, fmt.Errorf("clearing terminal records: %w", err)
	}
	return res.RowsAffected()
}

func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
