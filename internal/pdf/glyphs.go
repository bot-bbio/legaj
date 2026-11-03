package pdf

import (
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/signintech/gopdf/fontmaker/core"

	"github.com/bot-bbio/legaj/internal/apperr"
)

// coverage is the set of characters both embedded fonts can draw. gopdf
// draws anything else as nothing, silently, so a name in another script
// would vanish from the letter (the A11 bug class R24 exists to prevent).
var coverage = sync.OnceValues(func() (map[rune]bool, error) {
	both := map[rune]bool{}
	for i, data := range [][]byte{regularTTF, boldTTF} {
		var p core.TTFParser
		if err := p.ParseFontData(data); err != nil {
			return nil, err
		}
		set := map[rune]bool{}
		for code, glyph := range p.Chars() {
			if glyph != 0 {
				set[rune(code)] = true
			}
		}
		if i == 0 {
			both = set
			continue
		}
		for r := range both {
			if !set[r] {
				delete(both, r)
			}
		}
	}
	return both, nil
})

// unprintable lists, in order of first appearance, the characters of the
// letter that the fonts can't draw. Whitespace and control characters are
// layout, not glyphs.
func unprintable(l Letter) ([]rune, error) {
	cov, err := coverage()
	if err != nil {
		return nil, err
	}
	texts := append([]string{l.Name, l.Contact, l.Date, l.Salutation, l.Closing, l.Signature}, l.Recipient...)
	texts = append(texts, l.Paragraphs...)
	seen := map[rune]bool{}
	var out []rune
	for _, t := range texts {
		for _, r := range t {
			if unicode.IsSpace(r) || unicode.IsControl(r) || cov[r] || seen[r] {
				continue
			}
			seen[r] = true
			out = append(out, r)
		}
	}
	return out, nil
}

// checkPrintable refuses a letter with characters the fonts can't draw,
// naming them, so the user can replace them instead of getting a PDF with
// gaps.
func checkPrintable(l Letter) error {
	bad, err := unprintable(l)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	if len(bad) == 0 {
		return nil
	}
	names := make([]string, 0, len(bad))
	for i, r := range bad {
		if i == 10 {
			names = append(names, "…")
			break
		}
		names = append(names, string(r))
	}
	return apperr.New(apperr.CodeInvalid, fmt.Sprintf(
		"The letter has characters LeGaJ's PDF font can't print: %s. Replace them and try again.", strings.Join(names, " ")))
}
