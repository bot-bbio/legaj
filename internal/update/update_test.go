package update_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/testenv"
	"github.com/bot-bbio/legaj/internal/update"
)

func TestMain(m *testing.M) { testenv.Main(m) }

var ctx = context.Background()

func server(t *testing.T, body string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/repos/bot-bbio/legaj/releases/latest" || !strings.HasPrefix(r.Header.Get("User-Agent"), "LeGaJ/") {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func checker(base string, enabled bool, current string) *update.Checker {
	return update.New(update.Config{
		APIBase: base, Current: current, Timeout: 5 * time.Second,
		Enabled: func(context.Context) bool { return enabled },
	})
}

const release = `{"tag_name": "v2.1.0", "html_url": "https://github.com/bot-bbio/legaj/releases/tag/v2.1.0", "draft": false, "prerelease": false}`

func TestR40_NewerReleaseShowsBanner(t *testing.T) {
	var hits atomic.Int32
	s := server(t, release, &hits)

	got, err := checker(s.URL, true, "2.0.0").Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Available || got.Latest != "2.1.0" || got.URL != "https://github.com/bot-bbio/legaj/releases/tag/v2.1.0" {
		t.Fatalf("result = %+v", got)
	}

	same, err := checker(s.URL, true, "2.1.0").Check(ctx)
	if err != nil || same.Available {
		t.Fatalf("up to date: %+v %v", same, err)
	}

	// The answer is cached for the session: one request per launch.
	c := checker(s.URL, true, "2.0.0")
	before := hits.Load()
	for range 3 {
		if _, err := c.Check(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n := hits.Load() - before; n != 1 {
		t.Fatalf("%d requests for three checks, want 1", n)
	}
}

func TestR40_ToggleOffNoRequest(t *testing.T) {
	var hits atomic.Int32
	s := server(t, release, &hits)
	got, err := checker(s.URL, false, "2.0.0").Check(ctx)
	if err != nil || got.Available {
		t.Fatalf("off: %+v %v", got, err)
	}
	if hits.Load() != 0 {
		t.Fatalf("made %d requests with the update check off", hits.Load())
	}
}

func TestR40_UntrustedOrBrokenReleasesAreIgnored(t *testing.T) {
	for name, body := range map[string]string{
		"foreign link": `{"tag_name": "v9.0.0", "html_url": "https://evil.example.com/legaj.exe"}`,
		"http link":    `{"tag_name": "v9.0.0", "html_url": "http://github.com/bot-bbio/legaj/releases/tag/v9.0.0"}`,
		"bad tag":      `{"tag_name": "latest", "html_url": "https://github.com/bot-bbio/legaj/releases/tag/latest"}`,
		"not json":     `<html>rate limited</html>`,
		"prerelease":   `{"tag_name": "v9.0.0-rc.1", "prerelease": true, "html_url": "https://github.com/bot-bbio/legaj/releases/tag/v9.0.0-rc.1"}`,
	} {
		var hits atomic.Int32
		s := server(t, body, &hits)
		got, _ := checker(s.URL, true, "2.0.0").Check(ctx)
		if got.Available {
			t.Errorf("%s: offered %+v", name, got)
		}
	}
}

func TestR40_SemverCompare(t *testing.T) {
	ordered := []string{
		"1.9.9", "2.0.0-alpha", "2.0.0-alpha.1", "2.0.0-alpha.beta", "2.0.0-beta", "2.0.0-beta.2",
		"2.0.0-beta.11", "2.0.0-dev", "2.0.0-rc.1", "2.0.0", "2.0.1", "2.1.0", "10.0.0",
	}
	for i := range ordered {
		for j := range ordered {
			got, err := update.Compare(ordered[i], ordered[j])
			if err != nil {
				t.Fatalf("Compare(%s, %s): %v", ordered[i], ordered[j], err)
			}
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got != want {
				t.Errorf("Compare(%s, %s) = %d, want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
	if c, err := update.Compare("v2.0.0", "2.0.0"); err != nil || c != 0 {
		t.Fatalf("a leading v must be ignored: %d %v", c, err)
	}
	for _, bad := range []string{"", "2", "2.0", "2.0.0.0", "a.b.c", "2.0.0-", "02.0.0"} {
		if _, err := update.Compare(bad, "1.0.0"); err == nil {
			t.Errorf("Compare(%q) accepted", bad)
		}
	}
}
