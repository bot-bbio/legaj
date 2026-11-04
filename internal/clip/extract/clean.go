package extract

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// cleanLine turns a candidate into one plain line: markup stripped (when
// the value may hold it), whitespace collapsed, invisible characters gone.
func cleanLine(s string, markup bool) string {
	if markup && looksLikeMarkup(s) {
		s = htmlToText(s)
	}
	return strings.Join(strings.Fields(dropInvisible(s)), " ")
}

// cleanBlock turns a description candidate into plain text with paragraph
// breaks, bounded to MaxDescription bytes.
func cleanBlock(s string, markup bool) string {
	if markup && looksLikeMarkup(s) {
		s = htmlToText(s)
	}
	return truncateBytes(normalizeText(s), MaxDescription)
}

// fitDetail bounds a location to MaxDetail characters, cutting at a "; "
// between places when it can. "" when nothing is left.
func fitDetail(s string) string {
	if utf8.RuneCountInString(s) <= MaxDetail {
		return s
	}
	r := []rune(s)[:MaxDetail]
	if i := strings.LastIndex(string(r), "; "); i > 0 {
		return string(r)[:i]
	}
	return strings.TrimSpace(string(r))
}

// TruncateBytes cuts s to at most limit bytes without splitting a
// character, and trims the cut end.
func TruncateBytes(s string, limit int) string { return truncateBytes(s, limit) }

func truncateBytes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.TrimSpace(s[:cut])
}

// dropInvisible removes control and zero-width characters, and turns the
// other Unicode spaces and line separators into plain ones.
func dropInvisible(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case r == '\t', r == '\r', r == '\u00a0', r == '\u3000':
			return ' '
		case r == '\u2028', r == '\u2029':
			return '\n'
		case r == '\u200b', r == '\u200c', r == '\u200d', r == '\u2060', r == '\ufeff':
			return -1
		case unicode.IsControl(r), r == utf8.RuneError:
			return -1
		case unicode.IsSpace(r):
			return ' '
		}
		return r
	}, s)
}

// normalizeText trims every line, collapses runs of spaces and keeps at
// most one blank line between paragraphs.
func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(dropInvisible(s), "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, l := range lines {
		l = strings.Join(strings.Fields(l), " ")
		if l == "" {
			blank = len(out) > 0
			continue
		}
		if blank {
			out = append(out, "")
			blank = false
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// knownTag matches common HTML tags; it's how escaped markup is recognised.
// Text like "<Company>" or "std::vector<int>" doesn't match.
var knownTag = regexp.MustCompile(`(?i)</?(?:p|br|div|span|strong|b|i|em|u|ul|ol|li|h[1-6]|a|table|tbody|tr|td|th|section|article|blockquote|pre|hr|font|sup|sub)(?:\s[^<>]*)?/?>`)

// entity matches an HTML character reference.
var entity = regexp.MustCompile(`&(?:#[0-9]{1,7}|#[xX][0-9a-fA-F]{1,6}|[a-zA-Z][a-zA-Z0-9]{1,31});`)

func looksLikeMarkup(s string) bool { return knownTag.MatchString(s) || entity.MatchString(s) }

// htmlToText converts an HTML fragment to text. When the fragment holds no
// tags of its own but decodes to markup, it was escaped twice (LinkedIn's
// JSON-LD, seen 2026-11-03), so it's converted again. A fragment with real
// tags is converted once: text inside it that decodes to "<div>" is the
// author's text.
func htmlToText(s string) string {
	escapedTwice := !knownTag.MatchString(s)
	s = stripTags(s)
	if escapedTwice && knownTag.MatchString(s) {
		s = stripTags(s)
	}
	return s
}

// blockTags start a new paragraph; skipTags hide their content entirely.
var blockTags = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Section: true, atom.Article: true, atom.Header: true, atom.Footer: true,
	atom.Main: true, atom.Aside: true, atom.Nav: true, atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true,
	atom.H5: true, atom.H6: true, atom.Ul: true, atom.Ol: true, atom.Dl: true, atom.Dt: true, atom.Dd: true,
	atom.Table: true, atom.Tr: true, atom.Blockquote: true, atom.Pre: true, atom.Figure: true,
	atom.Figcaption: true, atom.Address: true, atom.Form: true, atom.Fieldset: true,
}

var skipTags = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true, atom.Template: true, atom.Svg: true,
	atom.Math: true, atom.Iframe: true, atom.Object: true, atom.Head: true, atom.Select: true,
}

// textWriter builds text from HTML tokens. Breaks are requested, not
// written, so nested blocks ("<li><p>") give one break, not several.
type textWriter struct {
	b      strings.Builder
	breaks int  // newlines owed before the next text
	space  bool // a space is owed before the next text on the same line
	bullet bool // the next text starts a list item
	inList int  // depth of open <li> elements
}

// block asks for a paragraph break (a line break inside a list item).
func (w *textWriter) block() {
	n := 2
	if w.inList > 0 {
		n = 1
	}
	w.breaks = max(w.breaks, n)
}

// lineBreak adds one line break, so "<br><br>" makes a paragraph.
func (w *textWriter) lineBreak() { w.breaks = min(w.breaks+1, 2) }

// text writes a text token. HTML whitespace collapses to one space, and
// whitespace at a token's edge still separates it from its neighbours
// ("Build <b>AT&T</b> tools").
func (w *textWriter) text(s string) {
	s = dropInvisible(s)
	body := strings.Join(strings.Fields(s), " ")
	if body == "" {
		if s != "" {
			w.space = true
		}
		return
	}
	if r, _ := utf8.DecodeRuneInString(s); unicode.IsSpace(r) {
		w.space = true
	}
	if w.b.Len() > 0 {
		if w.breaks > 0 {
			w.b.WriteString(strings.Repeat("\n", w.breaks))
		} else if w.space {
			w.b.WriteByte(' ')
		}
	}
	w.breaks, w.space = 0, false
	if w.bullet {
		w.b.WriteString("• ")
		w.bullet = false
	}
	w.b.WriteString(body)
	if r, _ := utf8.DecodeLastRuneInString(s); unicode.IsSpace(r) {
		w.space = true
	}
}

// stripTags drops every tag and decodes entities, keeping block structure
// as line breaks and list items as "• " lines.
func stripTags(s string) string {
	z := html.NewTokenizer(strings.NewReader(s))
	var w textWriter
	skip := 0
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return w.b.String()
		case html.TextToken:
			if skip == 0 {
				w.text(string(z.Text()))
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			a := atom.Lookup(name)
			switch {
			case skipTags[a]:
				if tt == html.StartTagToken {
					skip++
				}
			case a == atom.Br:
				w.lineBreak()
			case a == atom.Hr:
				w.breaks = 2
			case a == atom.Li:
				w.breaks = max(w.breaks, 1)
				w.bullet = true
				w.inList++
			case blockTags[a]:
				w.block()
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			a := atom.Lookup(name)
			switch {
			case skipTags[a]:
				if skip > 0 {
					skip--
				}
			case a == atom.Li:
				w.inList = max(w.inList-1, 0)
				w.breaks = max(w.breaks, 1)
			case blockTags[a]:
				w.block()
			}
		}
	}
}
