// Package fuzzy ranks strings against a query.
package fuzzy

import (
	"sort"
	"strings"
	"unicode"
)

// Score returns how well name matches query; 0 means no match. Exact >
// prefix > word prefix > substring > in-order subsequence.
func Score(query, name string) int {
	q := strings.ToLower(strings.TrimSpace(query))
	n := strings.ToLower(name)
	if q == "" {
		return 1
	}
	switch {
	case n == q:
		return 1000
	case strings.HasPrefix(n, q):
		return 800 - len(n)
	case wordPrefix(n, q):
		return 600 - len(n)
	case strings.Contains(n, q):
		return 400 - strings.Index(n, q)
	}
	// Subsequence: reward consecutive runs, penalise gaps.
	score, qi, run, last := 200, 0, 0, -1
	qr := []rune(q)
	for i, r := range []rune(n) {
		if qi < len(qr) && r == qr[qi] {
			if last == i-1 {
				run++
				score += 5 * run
			} else {
				run = 0
				if last >= 0 {
					score -= i - last
				}
			}
			last = i
			qi++
		}
	}
	if qi < len(qr) {
		return 0
	}
	if score < 1 {
		score = 1
	}
	return score
}

func wordPrefix(n, q string) bool {
	words := strings.FieldsFunc(n, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, w := range words {
		if strings.HasPrefix(w, q) {
			return true
		}
	}
	return false
}

// Filter returns the indexes of names matching query, best first. Ties keep
// input order.
func Filter(query string, names []string) []int {
	type hit struct{ idx, score int }
	var hits []hit
	for i, n := range names {
		if s := Score(query, n); s > 0 {
			hits = append(hits, hit{i, s})
		}
	}
	if strings.TrimSpace(query) != "" {
		sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	}
	idx := make([]int, len(hits))
	for i, h := range hits {
		idx[i] = h.idx
	}
	return idx
}
