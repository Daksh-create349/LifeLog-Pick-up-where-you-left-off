package resumption

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lifelog/lifelog/internal/storage"
)

// GenerateCheckpointID creates a unique, sortable identifier for resume checkpoints.
func GenerateCheckpointID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("chk_%d_%s", time.Now().UTC().UnixNano(), hex.EncodeToString(b))
}

// CreateCheckpoint captures an immutable snapshot of project resumption state.
func CreateCheckpoint(ctx context.Context, db *storage.DB, projectPath string, label string, opts Options) (*storage.CheckpointRecord, error) {
	cleanLabel := strings.TrimSpace(label)
	if cleanLabel == "" {
		return nil, errors.New("checkpoint label cannot be empty")
	}
	if len(cleanLabel) > 256 {
		return nil, errors.New("checkpoint label cannot exceed 256 characters")
	}

	report, err := BuildResumptionReport(ctx, db, projectPath, opts)
	if err != nil {
		return nil, fmt.Errorf("generating resumption report for checkpoint: %w", err)
	}

	payloadBytes, err := json.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("serializing checkpoint payload: %w", err)
	}

	chk := &storage.CheckpointRecord{
		ID:            GenerateCheckpointID(),
		ProjectPath:   report.CanonicalPath,
		Label:         cleanLabel,
		CreatedAt:     time.Now().UTC(),
		FormatVersion: ExportSchemaVersion,
		Payload:       string(payloadBytes),
	}

	if err := db.CreateCheckpoint(ctx, chk); err != nil {
		return nil, fmt.Errorf("persisting checkpoint: %w", err)
	}

	return chk, nil
}

// RenderCheckpoint formats a stored historical checkpoint snapshot into the requested format.
// It strictly uses the snapshot payload recorded at creation time without re-querying current activity.
func RenderCheckpoint(chk *storage.CheckpointRecord, format string) ([]byte, error) {
	if chk == nil {
		return nil, errors.New("cannot render nil checkpoint")
	}

	version := strings.TrimSpace(chk.FormatVersion)
	if version == "" {
		version = ExportSchemaVersion
	}
	if !strings.HasPrefix(version, "1.") {
		return nil, fmt.Errorf("unsupported checkpoint format version %q (expected %s)", chk.FormatVersion, ExportSchemaVersion)
	}

	if strings.TrimSpace(chk.Payload) == "" {
		return nil, errors.New("checkpoint payload is empty")
	}

	var report ResumptionReport
	if err := json.Unmarshal([]byte(chk.Payload), &report); err != nil {
		return nil, fmt.Errorf("unmarshaling checkpoint payload: %w", err)
	}

	switch strings.ToLower(format) {
	case "json":
		return FormatJSON(&report)
	case "markdown", "md":
		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("# LIFELOG Checkpoint: %s\n\n", escapeMarkdown(chk.Label)))
		buf.WriteString(fmt.Sprintf("- **Checkpoint ID:** `%s`\n", chk.ID))
		buf.WriteString(fmt.Sprintf("- **Captured At:** `%s`\n", chk.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC")))
		buf.WriteString(fmt.Sprintf("- **Project Path:** `%s`\n", escapeMarkdown(chk.ProjectPath)))
		buf.WriteString("- **Snapshot Notice:** *Historical snapshot captured at checkpoint creation time.*\n\n---\n\n")

		md, err := FormatMarkdown(&report)
		if err != nil {
			return nil, err
		}
		buf.Write(md)
		return buf.Bytes(), nil
	default:
		// Text format
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "================================================================================\n")
		fmt.Fprintf(&buf, "LIFELOG RESUME CHECKPOINT: %s\n", chk.ID)
		fmt.Fprintf(&buf, "================================================================================\n")
		fmt.Fprintf(&buf, "Label:          %s\n", chk.Label)
		fmt.Fprintf(&buf, "Captured At:    %s\n", chk.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"))
		fmt.Fprintf(&buf, "Project Path:   %s\n", chk.ProjectPath)
		fmt.Fprintf(&buf, "Format Version: %s\n", chk.FormatVersion)
		fmt.Fprintf(&buf, "Status:         Historical snapshot (observed at checkpoint creation time)\n")
		fmt.Fprintf(&buf, "================================================================================\n\n")

		FormatReport(&buf, &report)
		return buf.Bytes(), nil
	}
}
