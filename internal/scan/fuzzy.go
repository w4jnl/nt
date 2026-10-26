package scan

import (
	"sort"
	"strings"
	"unicode"
)

// Match scores how well query matches s as a subsequence, ignoring case: every query character
// must appear in order. Matches at the start of a word and runs of consecutive characters score
// higher; ok is false when query is not a subsequence of s.
func Match(query, s string) (score int, ok bool) {
	q := []rune(strings.ToLower(strings.ReplaceAll(query, " ", "")))
	if len(q) == 0 {
		return 0, true
	}
	r := []rune(s)
	lower := []rune(strings.ToLower(s))
	qi, run := 0, 0
	for i := 0; i < len(lower) && qi < len(q); i++ {
		if lower[i] != q[qi] {
			run = 0
			continue
		}
		pts := 1
		if i == 0 || !unicode.IsLetter(r[i-1]) && !unicode.IsDigit(r[i-1]) || unicode.IsUpper(r[i]) && unicode.IsLower(r[i-1]) {
			pts += 8 // a word start
		}
		run++
		pts += 4 * (run - 1)
		score += pts
		qi++
	}
	if qi < len(q) {
		return 0, false
	}
	return score - len(r)/8, true // shorter strings win ties
}

// Find returns the notes whose title or path matches query, best first, at most limit.
func (ix *Index) Find(query string, limit int) []Note {
	type hit struct {
		n     Note
		score int
	}
	var hits []hit
	for _, n := range ix.Notes {
		st, okT := Match(query, n.Title)
		sp, okP := Match(query, n.Path)
		if !okT && !okP {
			continue
		}
		hits = append(hits, hit{n, max(st+4, sp)})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].n.Path > hits[j].n.Path // newer dated names first
	})
	var out []Note
	for i := 0; i < len(hits) && i < limit; i++ {
		out = append(out, hits[i].n)
	}
	return out
}
