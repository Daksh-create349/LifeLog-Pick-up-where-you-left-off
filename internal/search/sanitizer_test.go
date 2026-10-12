package search

import (
	"strings"
	"testing"
)

func TestBuildFTSQuery(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantFTS    string
		wantTokens int
	}{
		{
			name:       "empty query",
			input:      "",
			wantFTS:    "",
			wantTokens: 0,
		},
		{
			name:       "single term",
			input:      "migration",
			wantFTS:    `"migration"*`,
			wantTokens: 1,
		},
		{
			name:       "multiple terms",
			input:      "database migration",
			wantFTS:    `"database"* AND "migration"*`,
			wantTokens: 2,
		},
		{
			name:       "phrase with quotes",
			input:      `"database migration"`,
			wantFTS:    `"database migration"*`,
			wantTokens: 1,
		},
		{
			name:       "special characters and punctuation",
			input:      `feat: add-auth (v2.0) [test]`,
			wantFTS:    `"feat:"* AND "add-auth"* AND "(v2.0)"* AND "[test]"*`,
			wantTokens: 4,
		},
		{
			name:       "unicode terms",
			input:      "commit 🚀 世界",
			wantFTS:    `"commit"* AND "🚀"* AND "世界"*`,
			wantTokens: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fts, tokens := BuildFTSQuery(tt.input)
			if fts != tt.wantFTS {
				t.Errorf("BuildFTSQuery(%q) = %q, want %q", tt.input, fts, tt.wantFTS)
			}
			if len(tokens) != tt.wantTokens {
				t.Errorf("BuildFTSQuery(%q) token count = %d, want %d", tt.input, len(tokens), tt.wantTokens)
			}
		})
	}
}

func TestCalculateScore(t *testing.T) {
	exact := CalculateScore("database migration", "full content", "/var/log", "database migration", []string{"database", "migration"}, -0.5)
	partial := CalculateScore("other commit", "mentions database", "/var/log", "database migration", []string{"database", "migration"}, -0.5)

	if exact <= partial {
		t.Fatalf("expected exact match score (%f) to be significantly higher than partial match score (%f)", exact, partial)
	}

	// Recency / rank scale check
	scoreHighRank := CalculateScore("title", "content", "/path", "title", []string{"title"}, -0.01)
	scoreLowRank := CalculateScore("title", "content", "/path", "title", []string{"title"}, -5.0)

	if scoreHighRank <= scoreLowRank {
		t.Fatalf("expected higher rank score (%f) to exceed lower rank score (%f)", scoreHighRank, scoreLowRank)
	}

	if !strings.Contains("database migration", "database") {
		t.Fatal("sanity check failed")
	}
}
