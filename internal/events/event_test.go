package events_test

import (
	"errors"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/events"
)

func TestEvent_Validate(t *testing.T) {
	validEvent := events.Event{
		ID:        "evt_123",
		Type:      events.EventCreated,
		Path:      "/Users/test/project/file.go",
		Size:      100,
		ModTime:   time.Now().UTC(),
		IsDir:     false,
		Source:    "fs_poll",
		CreatedAt: time.Now().UTC(),
	}

	if err := validEvent.Validate(); err != nil {
		t.Fatalf("expected valid event, got: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(e *events.Event)
		wantErr error
	}{
		{
			name:    "empty ID",
			mutate:  func(e *events.Event) { e.ID = "" },
			wantErr: events.ErrEmptyID,
		},
		{
			name:    "invalid type",
			mutate:  func(e *events.Event) { e.Type = "unknown" },
			wantErr: events.ErrInvalidEventType,
		},
		{
			name:    "empty path",
			mutate:  func(e *events.Event) { e.Path = "" },
			wantErr: events.ErrEmptyPath,
		},
		{
			name:    "relative path",
			mutate:  func(e *events.Event) { e.Path = "relative/file.go" },
			wantErr: events.ErrRelativePath,
		},
		{
			name:    "empty source",
			mutate:  func(e *events.Event) { e.Source = "" },
			wantErr: events.ErrEmptySource,
		},
		{
			name:    "zero timestamp",
			mutate:  func(e *events.Event) { e.CreatedAt = time.Time{} },
			wantErr: events.ErrZeroTimestamp,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evt := validEvent
			tc.mutate(&evt)
			err := evt.Validate()
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}
