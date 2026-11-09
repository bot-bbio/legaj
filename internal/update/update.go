// Package update checks GitHub for a newer LeGaJ release (R40). It only
// notifies: nothing is downloaded or installed, and with the Settings
// toggle off no request is made at all. The check is disclosed in the
// privacy notice (R41).
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Repo is the GitHub repository releases come from.
const Repo = "bot-bbio/legaj"

// DefaultAPIBase is GitHub's REST API.
const DefaultAPIBase = "https://api.github.com"

// Result is what the banner needs.
type Result struct {
	Available bool   `json:"available"`
	Latest    string `json:"latest"` // e.g. "2.1.0"
	URL       string `json:"url"`    // the release page, always on github.com
}

// Config wires a Checker.
type Config struct {
	APIBase string // default DefaultAPIBase; tests point it at httptest
	Current string // this build's version
	Timeout time.Duration
	// Enabled reports the Settings toggle; when false no request is made.
	Enabled func(ctx context.Context) bool
}

// Checker asks once per session and remembers the answer.
type Checker struct {
	cfg    Config
	client *http.Client

	mu   sync.Mutex
	done bool
	last Result
}

// New creates a Checker.
func New(cfg Config) *Checker {
	if cfg.APIBase == "" {
		cfg.APIBase = DefaultAPIBase
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &Checker{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}
}

// Check returns whether a newer release exists. Network or parse problems
// mean "no update" plus an error for the log; the UI never shows them.
func (c *Checker) Check(ctx context.Context) (Result, error) {
	if c.cfg.Enabled == nil || !c.cfg.Enabled(ctx) {
		return Result{}, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		return c.last, nil
	}
	res, err := c.fetch(ctx)
	if err != nil {
		return Result{}, err // not cached: a later launch tries again
	}
	c.done, c.last = true, res
	return res, nil
}

type release struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func (c *Checker) fetch(ctx context.Context) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.APIBase+"/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "LeGaJ/"+c.cfg.Current)
	resp, err := c.client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("update check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("update check: HTTP %d", resp.StatusCode)
	}
	var r release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return Result{}, fmt.Errorf("update check: %w", err)
	}
	if r.Draft || r.Prerelease {
		return Result{}, nil
	}
	if !trustedURL(r.HTMLURL) {
		return Result{}, errors.New("update check: release link isn't a github.com page for this repository")
	}
	latest := strings.TrimPrefix(r.TagName, "v")
	newer, err := Compare(latest, c.cfg.Current)
	if err != nil {
		return Result{}, fmt.Errorf("update check: %w", err)
	}
	return Result{Available: newer > 0, Latest: latest, URL: r.HTMLURL}, nil
}

// trustedURL accepts only https pages of this repository on github.com,
// since the banner opens the link in the browser.
func trustedURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "github.com" && u.User == nil &&
		strings.HasPrefix(u.Path, "/"+Repo+"/releases/")
}

// version is a parsed semantic version (semver.org 2.0.0, no build part).
type version struct {
	core [3]int
	pre  []string
}

func parse(s string) (version, error) {
	s = strings.TrimPrefix(s, "v")
	main, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(main, ".")
	var v version
	if len(parts) != 3 {
		return v, fmt.Errorf("version %q isn't MAJOR.MINOR.PATCH", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || (len(p) > 1 && p[0] == '0') {
			return v, fmt.Errorf("version %q has a bad number %q", s, p)
		}
		v.core[i] = n
	}
	if hasPre {
		if pre == "" {
			return v, fmt.Errorf("version %q has an empty pre-release", s)
		}
		v.pre = strings.Split(pre, ".")
		for _, id := range v.pre {
			if id == "" {
				return v, fmt.Errorf("version %q has an empty pre-release part", s)
			}
		}
	}
	return v, nil
}

// Compare orders two versions by semver precedence: -1 if a < b, 0 if
// equal, 1 if a > b. A pre-release sorts before its release.
func Compare(a, b string) (int, error) {
	va, err := parse(a)
	if err != nil {
		return 0, err
	}
	vb, err := parse(b)
	if err != nil {
		return 0, err
	}
	for i := range va.core {
		if c := cmpInt(va.core[i], vb.core[i]); c != 0 {
			return c, nil
		}
	}
	switch {
	case len(va.pre) == 0 && len(vb.pre) == 0:
		return 0, nil
	case len(va.pre) == 0:
		return 1, nil
	case len(vb.pre) == 0:
		return -1, nil
	}
	for i := 0; i < len(va.pre) && i < len(vb.pre); i++ {
		if c := cmpIdent(va.pre[i], vb.pre[i]); c != 0 {
			return c, nil
		}
	}
	return cmpInt(len(va.pre), len(vb.pre)), nil
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// cmpIdent compares pre-release identifiers: numbers numerically, below
// any alphanumeric identifier, which compare as text.
func cmpIdent(a, b string) int {
	na, errA := strconv.Atoi(a)
	nb, errB := strconv.Atoi(b)
	switch {
	case errA == nil && errB == nil:
		return cmpInt(na, nb)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	}
	return strings.Compare(a, b)
}
