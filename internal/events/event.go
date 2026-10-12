package events

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

// EventType represents the category of filesystem modification.
type EventType string

const (
	EventCreated  EventType = "created"
	EventModified EventType = "modified"
	EventDeleted  EventType = "deleted"
)

var (
	ErrInvalidEventType = errors.New("invalid event type")
	ErrEmptyPath        = errors.New("path cannot be empty")
	ErrRelativePath     = errors.New("path must be absolute")
	ErrEmptyID          = errors.New("event id cannot be empty")
	ErrEmptySource      = errors.New("source cannot be empty")
	ErrZeroTimestamp    = errors.New("timestamp cannot be zero")
)

// Event represents a single observed filesystem change.
type Event struct {
	ID        string    `json:"id"`
	Type      EventType `json:"type"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"mod_time"`
	IsDir     bool      `json:"is_dir"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

// Validate verifies that the Event contains valid, non-empty, and secure fields.
func (e *Event) Validate() error {
	if e.ID == "" {
		return ErrEmptyID
	}
	switch e.Type {
	case EventCreated, EventModified, EventDeleted:
		// Valid
	default:
		return fmt.Errorf("%w: %q", ErrInvalidEventType, e.Type)
	}
	if e.Path == "" {
		return ErrEmptyPath
	}
	if !filepath.IsAbs(e.Path) {
		return fmt.Errorf("%w: %q", ErrRelativePath, e.Path)
	}
	if e.Source == "" {
		return ErrEmptySource
	}
	if e.CreatedAt.IsZero() {
		return ErrZeroTimestamp
	}
	return nil
}
