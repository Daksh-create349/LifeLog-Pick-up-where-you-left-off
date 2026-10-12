package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

var (
	// ErrCheckpointNotFound is returned when a requested checkpoint record does not exist.
	ErrCheckpointNotFound = errors.New("checkpoint not found")
)

// CheckpointRecord represents an immutable snapshot of a project's resumption state.
type CheckpointRecord struct {
	ID            string    `json:"id"`
	ProjectPath   string    `json:"project_path"`
	Label         string    `json:"label"`
	CreatedAt     time.Time `json:"created_at"`
	FormatVersion string    `json:"format_version"`
	Payload       string    `json:"payload"`
}

// CreateCheckpoint persists a new checkpoint record.
func (db *DB) CreateCheckpoint(ctx context.Context, chk *CheckpointRecord) error {
	if chk == nil {
		return errors.New("cannot create nil checkpoint")
	}
	if chk.ID == "" {
		return errors.New("checkpoint ID cannot be empty")
	}
	if chk.ProjectPath == "" {
		return errors.New("checkpoint project path cannot be empty")
	}
	if chk.Label == "" {
		return errors.New("checkpoint label cannot be empty")
	}
	if chk.Payload == "" {
		return errors.New("checkpoint payload cannot be empty")
	}
	if chk.FormatVersion == "" {
		chk.FormatVersion = "1.0.0"
	}
	if chk.CreatedAt.IsZero() {
		chk.CreatedAt = time.Now().UTC()
	}

	createdStr := chk.CreatedAt.UTC().Format(time.RFC3339Nano)

	query := `
		INSERT INTO checkpoints (id, project_path, label, created_at, format_version, payload)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := db.sql.ExecContext(ctx, query,
		chk.ID,
		chk.ProjectPath,
		chk.Label,
		createdStr,
		chk.FormatVersion,
		chk.Payload,
	)
	if err != nil {
		return fmt.Errorf("inserting checkpoint %s: %w", chk.ID, err)
	}

	return nil
}

// GetCheckpoint retrieves a checkpoint record by its unique ID.
func (db *DB) GetCheckpoint(ctx context.Context, id string) (*CheckpointRecord, error) {
	if id == "" {
		return nil, errors.New("checkpoint ID cannot be empty")
	}

	query := `
		SELECT id, project_path, label, created_at, format_version, payload
		FROM checkpoints
		WHERE id = ?
		LIMIT 1
	`
	var chk CheckpointRecord
	var createdStr string

	err := db.sql.QueryRowContext(ctx, query, id).Scan(
		&chk.ID,
		&chk.ProjectPath,
		&chk.Label,
		&createdStr,
		&chk.FormatVersion,
		&chk.Payload,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCheckpointNotFound
		}
		return nil, fmt.Errorf("retrieving checkpoint %s: %w", id, err)
	}

	chk.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	if chk.CreatedAt.IsZero() {
		chk.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	}

	return &chk, nil
}

// ListCheckpoints lists checkpoints in reverse-chronological order.
// If projectPath is non-empty, only checkpoints matching the project path are returned.
func (db *DB) ListCheckpoints(ctx context.Context, projectPath string, limit int) ([]CheckpointRecord, error) {
	if limit <= 0 {
		limit = 20
	} else if limit > 500 {
		limit = 500
	}

	query := `
		SELECT id, project_path, label, created_at, format_version, payload
		FROM checkpoints
	`
	var args []interface{}

	if projectPath != "" {
		pathA := filepath.Clean(projectPath)
		pathB, _ := filepath.EvalSymlinks(pathA)
		if pathB == "" {
			pathB = pathA
		}

		if pathA == pathB {
			query += " WHERE project_path = ?"
			args = append(args, pathA)
		} else {
			query += " WHERE (project_path = ? OR project_path = ?)"
			args = append(args, pathA, pathB)
		}
	}

	query += " ORDER BY created_at DESC, id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing checkpoints: %w", err)
	}
	defer rows.Close()

	var checkpoints []CheckpointRecord
	for rows.Next() {
		var chk CheckpointRecord
		var createdStr string

		if err := rows.Scan(
			&chk.ID,
			&chk.ProjectPath,
			&chk.Label,
			&createdStr,
			&chk.FormatVersion,
			&chk.Payload,
		); err != nil {
			return nil, fmt.Errorf("scanning checkpoint row: %w", err)
		}

		chk.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		if chk.CreatedAt.IsZero() {
			chk.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		}

		checkpoints = append(checkpoints, chk)
	}

	return checkpoints, rows.Err()
}

// DeleteCheckpoint removes a checkpoint record by its unique ID.
func (db *DB) DeleteCheckpoint(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("checkpoint ID cannot be empty")
	}

	res, err := db.sql.ExecContext(ctx, "DELETE FROM checkpoints WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting checkpoint %s: %w", id, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking deleted rows: %w", err)
	}
	if affected == 0 {
		return ErrCheckpointNotFound
	}

	return nil
}
