package clip_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/bot-bbio/legaj/internal/ai"
)

func TestR20_40ConcurrentClipsAllSavedViaHTTP(t *testing.T) {
	// A7: V1 lost clips that arrived together. Every save goes through the
	// real server: Host check, Origin check, session, rate limiter, the
	// ingester and the database.
	f := newFixture(t, 0)
	f.ai.Respond = func(ai.Request) (string, error) { return reply("", "", ""), nil }
	s := startServer(t, f.in)

	const n = 40
	sessions := make([]string, n)
	for i := range sessions {
		sessions[i] = s.session(t)
	}
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			b, _ := json.Marshal(map[string]any{"session": sessions[i], "page": validPage(fmt.Sprintf("https://careers.example.com/job/%d", i))})
			req, _ := http.NewRequest(http.MethodPost, s.base+"/api/clips", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", s.origin)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				errs <- err
				return
			}
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)
			if res.StatusCode != http.StatusOK {
				errs <- fmt.Errorf("clip %d: %d %s", i, res.StatusCode, body)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if apps, err := f.p.List(ctx); err != nil || len(apps) != n {
		t.Fatalf("%d rows (%v), want %d", len(apps), err, n)
	}
}
