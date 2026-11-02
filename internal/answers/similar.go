package answers

import (
	"strings"
	"unicode"
)

// SuggestThreshold is the token-Jaccard similarity at which a banked answer
// is offered for a new question (R25).
const SuggestThreshold = 0.5

// tokenSet splits text into lower-case words: runs of letters and digits.
// Case, punctuation and word order don't matter.
func tokenSet(s string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		set[w] = struct{}{}
	}
	return set
}

// Similarity is the Jaccard index of two questions' word sets: shared
// words over all words. Two questions without words score 0.
func Similarity(a, b string) float64 {
	sa, sb := tokenSet(a), tokenSet(b)
	shared := 0
	for w := range sa {
		if _, ok := sb[w]; ok {
			shared++
		}
	}
	union := len(sa) + len(sb) - shared
	if union == 0 {
		return 0
	}
	return float64(shared) / float64(union)
}

// sameQuestion reports whether two questions are the same text, ignoring
// case and spacing. Regenerating such a question replaces its draft.
func sameQuestion(a, b string) bool {
	norm := func(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }
	return norm(a) == norm(b)
}
