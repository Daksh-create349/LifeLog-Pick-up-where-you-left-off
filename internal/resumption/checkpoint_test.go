package resumption_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/resumption"
)

func TestCheckpoint_CreateListShowDelete(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()

	projectDir := t.TempDir()

	// 1. Validation: empty label rejected
	_, err := resumption.CreateCheckpoint(ctx, db, projectDir, "", resumption.Options{})
	if err == nil {
		t.Errorf("expected error on empty checkpoint label, got nil")
	}

	// 2. Create checkpoint with valid label
	label := "Pre-refactor baseline snapshot"
	chk, err := resumption.CreateCheckpoint(ctx, db, projectDir, label, resumption.Options{})
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}

	if chk.ID == "" {
		t.Errorf("expected non-empty checkpoint ID")
	}
	if chk.Label != label {
		t.Errorf("got label %q, want %q", chk.Label, label)
	}

	// 3. List checkpoints
	list, err := db.ListCheckpoints(ctx, projectDir, 10)
	if err != nil {
		t.Fatalf("ListCheckpoints failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(list))
	}
	if list[0].ID != chk.ID {
		t.Errorf("expected checkpoint ID %q, got %q", chk.ID, list[0].ID)
	}

	// 4. Render checkpoint in different formats
	textOut, err := resumption.RenderCheckpoint(chk, "text")
	if err != nil {
		t.Fatalf("RenderCheckpoint text failed: %v", err)
	}
	if !strings.Contains(string(textOut), "LIFELOG RESUME CHECKPOINT") {
		t.Errorf("missing header in text checkpoint output: %s", string(textOut))
	}
	if !strings.Contains(string(textOut), label) {
		t.Errorf("missing label in text checkpoint output: %s", string(textOut))
	}

	jsonOut, err := resumption.RenderCheckpoint(chk, "json")
	if err != nil {
		t.Fatalf("RenderCheckpoint json failed: %v", err)
	}
	if !strings.Contains(string(jsonOut), "schema_version") {
		t.Errorf("missing schema_version in json checkpoint output: %s", string(jsonOut))
	}

	mdOut, err := resumption.RenderCheckpoint(chk, "markdown")
	if err != nil {
		t.Fatalf("RenderCheckpoint markdown failed: %v", err)
	}
	if !strings.Contains(string(mdOut), "LIFELOG Checkpoint") {
		t.Errorf("missing title in markdown checkpoint output: %s", string(mdOut))
	}

	// 5. Delete checkpoint
	if err := db.DeleteCheckpoint(ctx, chk.ID); err != nil {
		t.Fatalf("DeleteCheckpoint failed: %v", err)
	}

	// 6. Verify deleted
	_, err = db.GetCheckpoint(ctx, chk.ID)
	if err == nil {
		t.Errorf("expected error getting deleted checkpoint, got nil")
	}
}

func TestCheckpoint_SnapshotImmutability(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()

	projectDir := t.TempDir()
	now := time.Now().UTC()

	// 1. Record an initial filesystem event
	initialEvent := events.Event{
		ID:        "fs-chk-init",
		Type:      events.EventCreated,
		Path:      filepath.Join(projectDir, "initial.go"),
		Size:      100,
		ModTime:   now,
		Source:    "watch",
		CreatedAt: now,
	}
	if err := db.RecordEvents(ctx, []events.Event{initialEvent}); err != nil {
		t.Fatal(err)
	}

	// 2. Create checkpoint
	chk, err := resumption.CreateCheckpoint(ctx, db, projectDir, "Baseline with initial.go", resumption.Options{})
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}

	// 3. Add subsequent activity after checkpoint creation
	subsequentEvent := events.Event{
		ID:        "fs-chk-subsequent",
		Type:      events.EventCreated,
		Path:      filepath.Join(projectDir, "newly_added_after_checkpoint.go"),
		Size:      500,
		ModTime:   now.Add(1 * time.Hour),
		Source:    "watch",
		CreatedAt: now.Add(1 * time.Hour),
	}
	if err := db.RecordEvents(ctx, []events.Event{subsequentEvent}); err != nil {
		t.Fatal(err)
	}

	// 4. Retrieve stored checkpoint from DB and verify it still only contains initial.go!
	storedChk, err := db.GetCheckpoint(ctx, chk.ID)
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}

	rendered, err := resumption.RenderCheckpoint(storedChk, "text")
	if err != nil {
		t.Fatal(err)
	}
	renderedStr := string(rendered)

	if !strings.Contains(renderedStr, "initial.go") {
		t.Errorf("expected checkpoint to contain initial.go: %s", renderedStr)
	}
	// The subsequent activity must NOT be present in historical checkpoint!
	if strings.Contains(renderedStr, "newly_added_after_checkpoint.go") {
		t.Errorf("checkpoint mutated! contains newly_added_after_checkpoint.go: %s", renderedStr)
	}
}
