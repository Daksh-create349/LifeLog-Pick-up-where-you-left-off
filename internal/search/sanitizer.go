package search

import (
	"math"
	"strings"
	"unicode"
)

// BuildFTSQuery constructs a safe FTS5 query string from user input,
// preventing FTS syntax errors caused by punctuation, wildcards, or operators.
// It also returns the extracted individual keywords for scoring.
func BuildFTSQuery(raw string) (string, []string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	var tokens []string
	var current strings.Builder
	inQuotes := false

	for _, r := range trimmed {
		if r == '"' {
			inQuotes = !inQuotes
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}

		if unicode.IsSpace(r) && !inQuotes {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteRune(r)
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	if len(tokens) == 0 {
		return "", nil
	}

	var ftsClauses []string
	var cleanTokens []string

	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}

		// Strip characters that break FTS tokens if standalone
		clean := strings.Map(func(r rune) rune {
			if r == '"' || r == '*' || r == '^' {
				return -1
			}
			return r
		}, t)

		clean = strings.TrimSpace(clean)
		if clean == "" {
			continue
		}

		cleanTokens = append(cleanTokens, clean)
		escaped := strings.ReplaceAll(clean, `"`, `""`)

		// Treat as prefix token for flexibility: "term"*
		ftsClauses = append(ftsClauses, `"`+escaped+`"*`)
	}

	if len(ftsClauses) == 0 {
		return "", nil
	}

	ftsQuery := strings.Join(ftsClauses, " AND ")
	return ftsQuery, cleanTokens
}

// CalculateScore computes an explainable, deterministic relevance score.
func CalculateScore(title, content, workingPath, queryText string, tokens []string, bm25Rank float64) float64 {
	// Base score from SQLite FTS5 rank (rank is <= 0 in SQLite; more negative = more relevant)
	score := 10.0 / (1.0 + math.Abs(bm25Rank))

	lowerTitle := strings.ToLower(title)
	lowerContent := strings.ToLower(content)
	lowerPath := strings.ToLower(workingPath)
	lowerQuery := strings.ToLower(queryText)

	// 1. Exact title match bonus
	if lowerTitle == lowerQuery {
		score += 100.0
	} else if strings.Contains(lowerTitle, lowerQuery) {
		score += 50.0
	}

	// 2. Working path exact/prefix match bonus
	if lowerPath == lowerQuery || strings.HasPrefix(lowerPath, lowerQuery) {
		score += 35.0
	} else if strings.Contains(lowerPath, lowerQuery) {
		score += 20.0
	}

	// 3. Content match bonus
	if strings.Contains(lowerContent, lowerQuery) {
		score += 15.0
	}

	// 4. Token matches across title
	for _, tok := range tokens {
		lt := strings.ToLower(tok)
		if strings.Contains(lowerTitle, lt) {
			score += 10.0
		}
		if strings.Contains(lowerPath, lt) {
			score += 5.0
		}
	}

	return score
}
