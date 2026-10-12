package resumption

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	// DefaultInactivityGap is the default threshold for separating activity sessions.
	DefaultInactivityGap = 30 * time.Minute
)

// SynthesizeSessions clusters chronological activities into distinct sessions separated
// by gaps greater than gapThreshold.
func SynthesizeSessions(activities []ActivityItem, gapThreshold time.Duration, maxSessions int) []Session {
	if len(activities) == 0 {
		return nil
	}

	if gapThreshold <= 0 {
		gapThreshold = DefaultInactivityGap
	}

	// 1. Deduplicate activities deterministically
	deduped := deduplicateActivities(activities)
	if len(deduped) == 0 {
		return nil
	}

	// 2. Sort deterministically in chronological order
	sort.Slice(deduped, func(i, j int) bool {
		if !deduped[i].Timestamp.Equal(deduped[j].Timestamp) {
			return deduped[i].Timestamp.Before(deduped[j].Timestamp)
		}
		if deduped[i].Domain != deduped[j].Domain {
			return deduped[i].Domain < deduped[j].Domain
		}
		return deduped[i].ID < deduped[j].ID
	})

	// 3. Cluster activities into sessions based on inactivity gap
	var sessions []Session
	var currentSession *Session

	for _, item := range deduped {
		if currentSession == nil {
			currentSession = &Session{
				StartTime:    item.Timestamp,
				EndTime:      item.Timestamp,
				DomainCounts: make(map[string]int),
			}
			currentSession.Activities = append(currentSession.Activities, item)
			currentSession.DomainCounts[item.Domain]++
			continue
		}

		// Check inactivity gap
		gap := item.Timestamp.Sub(currentSession.EndTime)
		if gap > gapThreshold {
			// Finalize current session
			finalizeSession(currentSession)
			sessions = append(sessions, *currentSession)

			// Start new session
			currentSession = &Session{
				StartTime:    item.Timestamp,
				EndTime:      item.Timestamp,
				DomainCounts: make(map[string]int),
			}
		} else if item.Timestamp.After(currentSession.EndTime) {
			currentSession.EndTime = item.Timestamp
		}

		currentSession.Activities = append(currentSession.Activities, item)
		currentSession.DomainCounts[item.Domain]++
	}

	if currentSession != nil {
		finalizeSession(currentSession)
		sessions = append(sessions, *currentSession)
	}

	// Assign sequential session IDs
	for i := range sessions {
		sessions[i].ID = fmt.Sprintf("session-%03d", i+1)
	}

	// Limit to maxSessions if positive (retaining newest sessions)
	if maxSessions > 0 && len(sessions) > maxSessions {
		sessions = sessions[len(sessions)-maxSessions:]
	}

	return sessions
}

func finalizeSession(s *Session) {
	s.ActivityCount = len(s.Activities)
	if s.EndTime.After(s.StartTime) {
		s.Duration = s.EndTime.Sub(s.StartTime)
	} else {
		s.Duration = 0
	}
	s.Summary = buildSessionSummary(s)
}

func deduplicateActivities(activities []ActivityItem) []ActivityItem {
	seen := make(map[string]bool)
	var result []ActivityItem

	for _, a := range activities {
		var key string
		if a.ID != "" {
			key = a.Domain + ":" + a.ID
		} else {
			key = fmt.Sprintf("%s:%d:%s:%s", a.Domain, a.Timestamp.UnixNano(), a.Type, a.Summary)
		}

		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, a)
	}

	return result
}

func buildSessionSummary(s *Session) string {
	var parts []string

	fsCount := s.DomainCounts["filesystem"]
	gitCount := s.DomainCounts["git"]
	termCount := s.DomainCounts["terminal"]

	if fsCount > 0 {
		parts = append(parts, fmt.Sprintf("%d file event(s)", fsCount))
	}
	if gitCount > 0 {
		parts = append(parts, fmt.Sprintf("%d Git observation(s)", gitCount))
	}
	if termCount > 0 {
		parts = append(parts, fmt.Sprintf("%d terminal command(s)", termCount))
	}

	summary := strings.Join(parts, ", ")

	// Highlight primary concrete actions if available
	var highlights []string
	for _, act := range s.Activities {
		if act.Type == "git_commit" && act.Summary != "" {
			highlights = append(highlights, fmt.Sprintf("commit %q", truncate(act.Summary, 35)))
			break
		}
	}
	for _, act := range s.Activities {
		if act.Type == "terminal_exec" && act.Summary != "" {
			highlights = append(highlights, fmt.Sprintf("ran %q", truncate(act.Summary, 30)))
			break
		}
	}

	if len(highlights) > 0 {
		summary += " [" + strings.Join(highlights, "; ") + "]"
	}

	return summary
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
