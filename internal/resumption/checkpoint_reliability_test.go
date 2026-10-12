package resumption_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/resumption"
	"github.com/lifelog/lifelog/internal/storage"
)

func TestRenderCheckpoint_UnsupportedVersion(t *testing.T) {
	chk := &storage.CheckpointRecord{
		ID:            "chk_unsupported",
		ProjectPath:   "/project/alpha",
		Label:         "Future Milestone",
		CreatedAt:     time.Now().UTC(),
		FormatVersion: "2.0.0", // Unsupported major version
		Payload:       "{}",
	}

	_, err := resumption.RenderCheckpoint(chk, "text")
	if err == nil {
		t.Fatal("expected error rendering unsupported checkpoint version, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported checkpoint format version") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRenderCheckpoint_MalformedPayload(t *testing.T) {
	chk := &storage.CheckpointRecord{
		ID:            "chk_corrupted",
		ProjectPath:   "/project/beta",
		Label:         "Corrupted Snapshot",
		CreatedAt:     time.Now().UTC(),
		FormatVersion: "1.0.0",
		Payload:       "{invalid-json-data",
	}

	_, err := resumption.RenderCheckpoint(chk, "text")
	if err == nil {
		t.Fatal("expected error unmarshaling corrupted checkpoint payload, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshaling checkpoint payload") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRenderCheckpoint_EmptyPayload(t *testing.T) {
	chk := &storage.CheckpointRecord{
		ID:            "chk_empty",
		ProjectPath:   "/project/gamma",
		Label:         "Empty Snapshot",
		CreatedAt:     time.Now().UTC(),
		FormatVersion: "1.0.0",
		Payload:       "",
	}

	_, err := resumption.RenderCheckpoint(chk, "text")
	if err == nil {
		t.Fatal("expected error with empty checkpoint payload, got nil")
	}
	if !strings.Contains(err.Error(), "checkpoint payload is empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}
