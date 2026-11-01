package logx

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Replacement marks written in place of redacted text.
const (
	SecretMark = "[REDACTED]"
	EmailMark  = "[email]"
	PhoneMark  = "[phone]"
)

// minSecretLen stops a tiny registered "secret" from shredding every line.
const minSecretLen = 8

// maxValueLen: any longer string (message or attribute) is dropped, not
// truncated. Prompts, model responses and resume text are all longer, so an
// accidental slog.String("body", …) still never reaches the log (R34).
const maxValueLen = 512

var (
	emailRE = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	// Phone candidates: a digit run with common separators. A candidate is
	// only redacted when it holds 9–15 digits, so dates (8 digits), ports
	// and counts survive.
	phoneRE = regexp.MustCompile(`\+?\(?\d[\d\s().\-]{6,}\d`)
)

// Redactor removes registered secrets, emails and phone numbers from text.
// It is safe for concurrent use.
type Redactor struct {
	mu      sync.RWMutex
	secrets []string
}

// NewRedactor returns a Redactor with no registered secrets.
func NewRedactor() *Redactor {
	return &Redactor{}
}

// Register adds a secret (e.g. the Gemini key on load, R4). Secrets are
// never unregistered: an old key stays redacted after it is replaced.
func (r *Redactor) Register(secret string) {
	secret = strings.TrimSpace(secret)
	if len(secret) < minSecretLen {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.secrets {
		if s == secret {
			return
		}
	}
	r.secrets = append(r.secrets, secret)
}

// Redact replaces secrets, emails and phone numbers in s.
func (r *Redactor) Redact(s string) string {
	r.mu.RLock()
	for _, secret := range r.secrets {
		s = strings.ReplaceAll(s, secret, SecretMark)
	}
	r.mu.RUnlock()
	s = emailRE.ReplaceAllString(s, EmailMark)
	return phoneRE.ReplaceAllStringFunc(s, func(m string) string {
		digits := 0
		for _, c := range m {
			if c >= '0' && c <= '9' {
				digits++
			}
		}
		if digits >= 9 && digits <= 15 {
			return PhoneMark
		}
		return m
	})
}

// scrub drops over-long values and redacts the rest.
func (r *Redactor) scrub(s string) string {
	if len(s) > maxValueLen {
		return fmt.Sprintf("[omitted: %d chars]", len(s))
	}
	return r.Redact(s)
}
