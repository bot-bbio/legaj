package pdf

import "strings"

// Wrap breaks text into lines no wider than maxWidth, greedily, at
// whitespace. A word wider than a whole line (a long URL) is broken
// between characters rather than overflowing the margin. Whitespace runs
// collapse to single spaces; blank text yields no lines.
func Wrap(measure func(string) (float64, error), text string, maxWidth float64) ([]string, error) {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		candidate := word
		if line != "" {
			candidate = line + " " + word
		}
		w, err := measure(candidate)
		if err != nil {
			return nil, err
		}
		if w <= maxWidth {
			line = candidate
			continue
		}
		if line != "" {
			lines = append(lines, line)
			line = ""
		}
		// The word alone: fits, or must be broken.
		w, err = measure(word)
		if err != nil {
			return nil, err
		}
		if w <= maxWidth {
			line = word
			continue
		}
		pieces, err := breakWord(measure, word, maxWidth)
		if err != nil {
			return nil, err
		}
		lines = append(lines, pieces[:len(pieces)-1]...)
		line = pieces[len(pieces)-1]
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines, nil
}

// breakWord splits one overlong word into pieces that each fit.
func breakWord(measure func(string) (float64, error), word string, maxWidth float64) ([]string, error) {
	var pieces []string
	cur := []rune{}
	for _, r := range word {
		next := append(cur, r)
		w, err := measure(string(next))
		if err != nil {
			return nil, err
		}
		if w > maxWidth && len(cur) > 0 {
			pieces = append(pieces, string(cur))
			cur = []rune{r}
			continue
		}
		cur = next
	}
	return append(pieces, string(cur)), nil
}
