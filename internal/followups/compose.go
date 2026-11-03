package followups

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/bot-bbio/legaj/internal/apperr"
)

// Client is where the user writes email (D10): LeGaJ opens a compose
// window there with the draft filled in, and never holds credentials.
type Client string

// Compose targets.
const (
	Gmail      Client = "gmail"
	OutlookCom Client = "outlook"    // personal Outlook.com
	Outlook365 Client = "outlook365" // work or school Microsoft 365
	MailApp    Client = "mailto"     // the default mail app
)

// LongBody is the body length past which some clients may truncate the
// link (R29).
const LongBody = 2000

// Message is a follow-up ready to open in a mail client.
type Message struct {
	To      string `json:"to"` // optional for web clients
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Warnings lists things the user should know before sending, without
// blocking it.
func (m Message) Warnings() []string {
	if utf8.RuneCountInString(m.Body) > LongBody {
		return []string{"This message is over 2,000 characters. Some mail apps cut long messages from links short: check it arrived whole, or shorten it."}
	}
	return []string{}
}

// recipientRE accepts one plain address. Anything that could add a header
// or a second recipient (?, &, comma, spaces, angle brackets) is refused.
var recipientRE = regexp.MustCompile(`^[A-Za-z0-9._+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)

// ComposeURL builds a compose link with every field percent-encoded
// (R29). Web links keep newlines as %0A; mailto uses CRLF (RFC 6068).
func ComposeURL(c Client, m Message) (string, error) {
	to := strings.TrimSpace(m.To)
	if to != "" && (len(to) > 254 || !recipientRE.MatchString(to)) {
		return "", apperr.New(apperr.CodeInvalid, "Enter one email address, like name@example.com, or leave it empty.")
	}
	q := func(pairs ...string) string {
		var parts []string
		for i := 0; i+1 < len(pairs); i += 2 {
			if pairs[i+1] == "" && pairs[i] == "to" {
				continue
			}
			parts = append(parts, pairs[i]+"="+escape(pairs[i+1]))
		}
		return strings.Join(parts, "&")
	}
	switch c {
	case Gmail:
		return "https://mail.google.com/mail/?view=cm&fs=1&" + q("to", to, "su", m.Subject, "body", m.Body), nil
	case OutlookCom:
		return "https://outlook.live.com/mail/0/deeplink/compose?" + q("to", to, "subject", m.Subject, "body", m.Body), nil
	case Outlook365:
		return "https://outlook.office.com/mail/deeplink/compose?" + q("to", to, "subject", m.Subject, "body", m.Body), nil
	case MailApp:
		if to == "" {
			return "", apperr.New(apperr.CodeInvalid, "Enter the recipient's email address to open it in your mail app.")
		}
		body := strings.ReplaceAll(strings.ReplaceAll(m.Body, "\r\n", "\n"), "\n", "\r\n")
		return "mailto:" + to + "?" + q("subject", m.Subject, "body", body), nil
	}
	return "", apperr.New(apperr.CodeInvalid, fmt.Sprintf("Unknown mail client %q.", string(c)))
}

// escape percent-encodes everything but RFC 3986 unreserved characters,
// byte by byte, so spaces are %20 (never "+"), "&" is %26 and UTF-8 is kept.
func escape(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}
