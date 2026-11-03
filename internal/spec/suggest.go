package spec

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// Suggest returns up to n candidates that are close to in (Levenshtein
// distance, case-insensitive), closest first; equally close candidates keep
// their order in candidates. Candidates further away than max(2, len(in)/3)
// edits are never suggested, so nonsense input yields no suggestion.
func Suggest(in string, candidates []string, n int) []string {
	if n <= 0 {
		return nil
	}
	limit := max(2, utf8.RuneCountInString(in)/3)
	lin := []rune(strings.ToLower(in))
	type match struct {
		s    string
		dist int
	}
	var ms []match
	seen := map[string]bool{}
	for _, c := range candidates {
		if seen[c] {
			continue
		}
		seen[c] = true
		if d := levenshtein(lin, []rune(strings.ToLower(c))); d <= limit {
			ms = append(ms, match{c, d})
		}
	}
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].dist < ms[j].dist })
	var out []string
	for _, m := range ms[:min(n, len(ms))] {
		out = append(out, m.s)
	}
	return out
}

// levenshtein is the edit distance between a and b (insert, delete, replace).
func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
