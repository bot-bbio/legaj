package secrets_test

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/logx"
	"github.com/bot-bbio/legaj/internal/secrets"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

const sentinelKey = "AIzaSyD-SENTINEL-KEY-0123456789abcdWXYZ"

func TestR4_SetGetDelete(t *testing.T) {
	s := secrets.NewMemory()
	if _, err := s.Get(); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("Get on empty store: err = %v, want ErrNotFound", err)
	}
	if err := s.Set(sentinelKey); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get()
	if err != nil || got != sentinelKey {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("Get after Delete: err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(); err != nil {
		t.Fatalf("Delete with nothing saved must succeed: %v", err)
	}
}

func TestR4_MaskShowsLast4(t *testing.T) {
	cases := map[string]string{
		sentinelKey: "••••WXYZ",
		"abcd":      "••••",
		"":          "",
	}
	for in, want := range cases {
		if got := secrets.Mask(in); got != want {
			t.Errorf("Mask(%q) = %q, want %q", in, got, want)
		}
	}
	if strings.Contains(secrets.Mask(sentinelKey), "SENTINEL") {
		t.Fatal("mask reveals more than the last 4 characters")
	}
}

func TestR4_ManagerMaskedNeverReturnsKey(t *testing.T) {
	m := secrets.NewManager(secrets.NewMemory(), logx.NewRedactor())
	st, err := m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Saved || st.Masked != "" {
		t.Fatalf("empty store status = %+v", st)
	}
	if err := m.Save("  " + sentinelKey + "\n"); err != nil {
		t.Fatal(err)
	}
	st, err = m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Saved || st.Masked != "••••WXYZ" {
		t.Fatalf("status = %+v", st)
	}
	if err := m.Remove(); err != nil {
		t.Fatal(err)
	}
	if st, _ := m.Status(); st.Saved {
		t.Fatal("key still reported after Remove")
	}
}

func TestR4_KeyRegisteredWithRedactorOnLoadAndSave(t *testing.T) {
	for _, path := range []string{"load", "save"} {
		t.Run(path, func(t *testing.T) {
			store := secrets.NewMemory()
			r := logx.NewRedactor()
			var buf bytes.Buffer
			log := logx.New(&buf, r, slog.LevelInfo)
			m := secrets.NewManager(store, r)
			if path == "load" {
				if err := store.Set(sentinelKey); err != nil {
					t.Fatal(err)
				}
				if _, err := m.Load(); err != nil {
					t.Fatal(err)
				}
			} else if err := m.Save(sentinelKey); err != nil {
				t.Fatal(err)
			}
			log.Info("oops " + sentinelKey)
			if strings.Contains(buf.String(), sentinelKey) {
				t.Fatalf("key reached the log after %s", path)
			}
		})
	}
}

func TestR4_SaveRejectsMalformedKeys(t *testing.T) {
	m := secrets.NewManager(secrets.NewMemory(), logx.NewRedactor())
	for _, bad := range []string{"", "   ", "short", "has space inside-0123456789", "tab\there-0123456789", strings.Repeat("k", 600)} {
		err := m.Save(bad)
		if err == nil {
			t.Errorf("Save(%q) accepted", bad)
			continue
		}
		if apperr.CodeOf(err) != apperr.CodeInvalid {
			t.Errorf("Save(%q) code = %q, want invalid", bad, apperr.CodeOf(err))
		}
		if strings.Contains(err.Error(), bad) && bad != "" && strings.TrimSpace(bad) != "" {
			t.Errorf("error message echoes the input: %q", err.Error())
		}
	}
}

func TestR4_LoadMissingKeyIsTypedError(t *testing.T) {
	m := secrets.NewManager(secrets.NewMemory(), logx.NewRedactor())
	_, err := m.Load()
	if apperr.CodeOf(err) != apperr.CodeAIKeyMissing {
		t.Fatalf("code = %q, want %q", apperr.CodeOf(err), apperr.CodeAIKeyMissing)
	}
}
