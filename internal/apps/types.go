package apps

import (
	"time"

	"github.com/lifelog/lifelog/internal/storage"
)

// Helper event names.
const (
	EventAppActivated = "app_activated"
	EventSystemSleep  = "system_sleep"
	EventSystemWake   = "system_wake"
	EventSystemLock   = "system_lock"
	EventSystemUnlock = "system_unlock"
	EventHeartbeat    = "heartbeat"
)

// HelperMessage represents incoming JSON from the native macOS helper.
type HelperMessage struct {
	Version   string `json:"version"`
	Event     string `json:"event"`
	BundleID  string `json:"bundle_id"`
	AppName   string `json:"app_name"`
	PID       int    `json:"pid,omitempty"`
	Timestamp string `json:"timestamp"`
}

// TrackingStatus represents the runtime state of application usage tracking.
type TrackingStatus struct {
	Enabled          bool       `json:"enabled"`
	Running          bool       `json:"running"`
	Status           string     `json:"status"` // "active", "paused", "crashed", "unavailable"
	ActiveApp        string     `json:"active_app,omitempty"`
	ActiveBundle     string     `json:"active_bundle,omitempty"`
	ActiveSessionID  string     `json:"active_session_id,omitempty"`
	ActiveSince      *time.Time `json:"active_since,omitempty"`
	ActiveDurationMs int64      `json:"active_duration_ms,omitempty"`
}

// Re-export storage models for consumers.
type (
	AppSession       = storage.AppSession
	AppUsageFilter   = storage.AppUsageFilter
	AppUsageStat     = storage.AppUsageStat
	AppUsageSummary  = storage.AppUsageSummary
	AppSessionFilter = storage.AppSessionFilter
)
