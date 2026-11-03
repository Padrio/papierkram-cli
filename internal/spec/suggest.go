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

// Nearest returns the n candidates closest to in (Levenshtein distance,
// case-insensitive), closest first, without a distance limit: there is always
// a suggestion as long as there are candidates. Ties keep the candidate
// order; duplicates count once. Huge enums (the expense categories, spec 6.2)
// use it so a typo always yields three values.
func Nearest(in string, candidates []string, n int) []string {
	lin := []rune(strings.ToLower(in))
	type match struct {
		s    string
		dist int
	}
	var ms []match
	seen := map[string]bool{}
	for _, c := range candidates {
		if !seen[c] {
			seen[c] = true
			ms = append(ms, match{c, levenshtein(lin, []rune(strings.ToLower(c)))})
		}
	}
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].dist < ms[j].dist })
	var out []string
	for _, m := range ms[:min(max(n, 0), len(ms))] {
		out = append(out, m.s)
	}
	return out
}
