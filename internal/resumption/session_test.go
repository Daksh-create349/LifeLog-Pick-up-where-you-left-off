package resumption

import (
	"testing"
	"time"
)

func TestSynthesizeSessions_Empty(t *testing.T) {
	sessions := SynthesizeSessions(nil, 30*time.Minute, 10)
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for nil input, got %d", len(sessions))
	}

	sessions = SynthesizeSessions([]ActivityItem{}, 30*time.Minute, 10)
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for empty slice, got %d", len(sessions))
	}
}

func TestSynthesizeSessions_SingleEvent(t *testing.T) {
	now := time.Now().UTC()
	activities := []ActivityItem{
		{
			ID:        "evt-1",
			Domain:    "filesystem",
			Type:      "fs_modified",
			Timestamp: now,
			Summary:   "main.go",
		},
	}

	sessions := SynthesizeSessions(activities, 30*time.Minute, 10)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.ActivityCount != 1 {
		t.Errorf("expected 1 activity, got %d", s.ActivityCount)
	}
	if s.Duration != 0 {
		t.Errorf("expected 0 duration for single event, got %v", s.Duration)
	}
	if !s.StartTime.Equal(now) || !s.EndTime.Equal(now) {
		t.Errorf("expected start/end time equal to event timestamp")
	}
	if s.DomainCounts["filesystem"] != 1 {
		t.Errorf("expected 1 filesystem domain count, got %d", s.DomainCounts["filesystem"])
	}
}

func TestSynthesizeSessions_InactivityGapBoundaries(t *testing.T) {
	base := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	gapThreshold := 30 * time.Minute

	activities := []ActivityItem{
		// Session 1: 10:00, 10:15, 10:30 (all <= 30 min from previous)
		{ID: "1", Domain: "filesystem", Timestamp: base, Summary: "file1.go"},
		{ID: "2", Domain: "filesystem", Timestamp: base.Add(15 * time.Minute), Summary: "file2.go"},
		{ID: "3", Domain: "terminal", Timestamp: base.Add(30 * time.Minute), Summary: "go test"},

		// Gap: 10:30 to 11:05 is 35 minutes (> 30 min) -> Session 2
		{ID: "4", Domain: "git", Timestamp: base.Add(65 * time.Minute), Summary: "commit 1"},
		{ID: "5", Domain: "filesystem", Timestamp: base.Add(80 * time.Minute), Summary: "file3.go"},

		// Gap: 11:20 to 12:00 is 40 minutes (> 30 min) -> Session 3
		{ID: "6", Domain: "terminal", Timestamp: base.Add(120 * time.Minute), Summary: "git status"},
	}

	sessions := SynthesizeSessions(activities, gapThreshold, 10)
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}

	// Verify Session 1
	if sessions[0].ActivityCount != 3 {
		t.Errorf("session 1: expected 3 activities, got %d", sessions[0].ActivityCount)
	}
	if sessions[0].Duration != 30*time.Minute {
		t.Errorf("session 1: expected 30m duration, got %v", sessions[0].Duration)
	}

	// Verify Session 2
	if sessions[1].ActivityCount != 2 {
		t.Errorf("session 2: expected 2 activities, got %d", sessions[1].ActivityCount)
	}
	if sessions[1].Duration != 15*time.Minute {
		t.Errorf("session 2: expected 15m duration, got %v", sessions[1].Duration)
	}

	// Verify Session 3
	if sessions[2].ActivityCount != 1 {
		t.Errorf("session 3: expected 1 activity, got %d", sessions[2].ActivityCount)
	}
	if sessions[2].Duration != 0 {
		t.Errorf("session 3: expected 0 duration, got %v", sessions[2].Duration)
	}
}

func TestSynthesizeSessions_OutOfOrderAndDuplicates(t *testing.T) {
	base := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

	activities := []ActivityItem{
		// Deliberately out of order and with duplicate IDs
		{ID: "3", Domain: "terminal", Timestamp: base.Add(20 * time.Minute), Summary: "go test"},
		{ID: "1", Domain: "filesystem", Timestamp: base, Summary: "file1.go"},
		{ID: "2", Domain: "filesystem", Timestamp: base.Add(10 * time.Minute), Summary: "file2.go"},
		{ID: "1", Domain: "filesystem", Timestamp: base, Summary: "file1.go"},                       // duplicate
		{ID: "2", Domain: "filesystem", Timestamp: base.Add(10 * time.Minute), Summary: "file2.go"}, // duplicate
	}

	sessions := SynthesizeSessions(activities, 30*time.Minute, 10)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.ActivityCount != 3 {
		t.Errorf("expected 3 deduplicated activities, got %d", s.ActivityCount)
	}
	// Verify chronological order within session
	if s.Activities[0].ID != "1" || s.Activities[1].ID != "2" || s.Activities[2].ID != "3" {
		t.Errorf("activities not sorted chronologically: %+v", s.Activities)
	}
}

func TestSynthesizeSessions_RepeatedTimestamps(t *testing.T) {
	base := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

	activities := []ActivityItem{
		{ID: "1", Domain: "filesystem", Timestamp: base, Summary: "file1.go"},
		{ID: "2", Domain: "git", Timestamp: base, Summary: "commit A"},
		{ID: "3", Domain: "terminal", Timestamp: base, Summary: "go build"},
	}

	sessions := SynthesizeSessions(activities, 30*time.Minute, 10)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.ActivityCount != 3 {
		t.Errorf("expected 3 activities with identical timestamp, got %d", s.ActivityCount)
	}
	if s.Duration != 0 {
		t.Errorf("expected 0 duration for identical timestamps, got %v", s.Duration)
	}
}

func TestSynthesizeSessions_MaxSessionsLimit(t *testing.T) {
	base := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	gapThreshold := 10 * time.Minute

	var activities []ActivityItem
	for i := 0; i < 5; i++ {
		activities = append(activities, ActivityItem{
			ID:        string(rune('A' + i)),
			Domain:    "filesystem",
			Timestamp: base.Add(time.Duration(i*30) * time.Minute),
			Summary:   "file.go",
		})
	}

	// 5 distinct sessions created (each 30 mins apart, gap is 10 mins)
	// With maxSessions = 2, only newest 2 sessions should be returned
	sessions := SynthesizeSessions(activities, gapThreshold, 2)
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions due to maxSessions limit, got %d", len(sessions))
	}
	if sessions[0].ID != "session-004" || sessions[1].ID != "session-005" {
		t.Errorf("expected newest sessions 4 and 5, got %s and %s", sessions[0].ID, sessions[1].ID)
	}
}
