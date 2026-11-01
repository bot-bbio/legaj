package logx_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/logx"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

const (
	sentinelKey   = "AIzaSyD-SENTINEL-KEY-0123456789abcdefgh"
	sentinelEmail = "lukasz.sentinel@example.com"
	sentinelPhone = "+48 601 234 567"
)

func newLogger(t *testing.T) (*slog.Logger, *bytes.Buffer, *logx.Redactor) {
	t.Helper()
	var buf bytes.Buffer
	r := logx.NewRedactor()
	return logx.New(&buf, r, slog.LevelDebug), &buf, r
}

func assertClean(t *testing.T, out string) {
	t.Helper()
	for _, s := range []string{sentinelKey, sentinelEmail, sentinelPhone, "601 234 567"} {
		if strings.Contains(out, s) {
			t.Fatalf("log output contains %q:\n%s", s, out)
		}
	}
}

func TestR34_LogRedactsKeyEmailPhone(t *testing.T) {
	log, buf, r := newLogger(t)
	r.Register(sentinelKey)

	log.Info("call with key "+sentinelKey+" for "+sentinelEmail,
		"phone", sentinelPhone,
		"err", errors.New("upstream said: bad key "+sentinelKey),
		slog.Group("user", "email", sentinelEmail),
		"any", struct{ K string }{sentinelKey},
	)
	log.With("contact", sentinelEmail).WithGroup("g").Warn("x", "p", sentinelPhone)

	out := buf.String()
	assertClean(t, out)
	for _, want := range []string{logx.SecretMark, logx.EmailMark, logx.PhoneMark} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %s in output:\n%s", want, out)
		}
	}
}

func TestR34_AppErrorCauseLoggedButRedacted(t *testing.T) {
	log, buf, r := newLogger(t)
	r.Register(sentinelKey)
	err := apperr.Wrap(errors.New("gemini 401 for key "+sentinelKey), apperr.CodeAIKeyRejected, "Gemini rejected the key.")
	log.Error("generate failed", "err", err)

	out := buf.String()
	assertClean(t, out)
	for _, want := range []string{`"cause"`, string(apperr.CodeAIKeyRejected), "gemini 401"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log should keep the redacted cause for debugging; missing %s:\n%s", want, out)
		}
	}
}

func TestR34_RedactionAppliesToSecretsRegisteredLater(t *testing.T) {
	log, buf, r := newLogger(t)
	child := log.With("component", "ai")
	r.Register(sentinelKey) // e.g. the user replaces the key while running
	child.Info("using " + sentinelKey)
	assertClean(t, buf.String())
}

func TestR34_ShortSecretsIgnored(t *testing.T) {
	r := logx.NewRedactor()
	r.Register("ab") // would shred every log line
	if got := r.Redact("about tables"); got != "about tables" {
		t.Fatalf("short secret redacted ordinary text: %q", got)
	}
}

func TestR34_DatesAndSmallNumbersNotMangled(t *testing.T) {
	r := logx.NewRedactor()
	for _, s := range []string{"applied 2026-11-01", "3 of 40 clips", "port 47321", "v2.0.0"} {
		if got := r.Redact(s); got != s {
			t.Errorf("Redact(%q) = %q; ordinary text must survive", s, got)
		}
	}
}

func TestR34_LongValuesOmitted(t *testing.T) {
	log, buf, _ := newLogger(t)
	body := strings.Repeat("resume line ", 100) // 1,200 chars, like a prompt
	log.Info(body, "body", body)
	if strings.Contains(buf.String(), "resume line resume line") {
		t.Fatalf("long value was logged:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "omitted") {
		t.Fatalf("expected an omitted marker:\n%s", buf.String())
	}
}

func TestR34_AIBodiesNeverLogged(t *testing.T) {
	// The only AI logging API takes this struct; it has no field that could
	// carry a prompt or a response.
	allowed := map[string]reflect.Kind{
		"Provider": reflect.String,
		"Model":    reflect.String,
		"Status":   reflect.Int,
		"Attempt":  reflect.Int,
		"Duration": reflect.Int64, // time.Duration
		"Outcome":  reflect.String,
	}
	typ := reflect.TypeFor[logx.AICall]()
	if typ.NumField() != len(allowed) {
		t.Fatalf("AICall has %d fields, want exactly %d (status, model and timings only)", typ.NumField(), len(allowed))
	}
	for i := range typ.NumField() {
		f := typ.Field(i)
		kind, ok := allowed[f.Name]
		if !ok || f.Type.Kind() != kind {
			t.Fatalf("AICall field %s %s is not allowed", f.Name, f.Type)
		}
	}

	log, buf, _ := newLogger(t)
	logx.LogAICall(log, logx.AICall{Provider: "gemini", Model: "gemini-3.1-flash-lite", Status: 429, Attempt: 2, Duration: 1500 * time.Millisecond, Outcome: "retry"})
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range rec {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"attempt", "duration_ms", "level", "model", "msg", "outcome", "provider", "status", "time"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("AI call log keys = %v, want %v", keys, want)
	}
}
