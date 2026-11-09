package gemini

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/logx"
)

// maxModelPages bounds the listing walk, in case a page token repeats.
const maxModelPages = 10

// maxDisplayName caps a listed display name; the listing is untrusted.
const maxDisplayName = 80

// stableFlash is the whole eligibility rule for listed models (R67): a
// stable Flash or Flash-Lite text model, named exactly gemini-<version>-flash
// or gemini-<version>-flash-lite. Anything longer, such as previews, experiments,
// image, speech, live and audio variants, dated snapshots, and "-latest"
// aliases (which can move to a preview without notice), fails the pattern, so
// no list of exclusions has to keep up with new model families.
var stableFlash = regexp.MustCompile(`^gemini-(\d{1,3}(?:\.\d{1,3})?)-flash(-lite)?$`)

// SeedModels is the built-in fallback when the listing can't be had: the one
// model ID LeGaJ ships with (R67).
func SeedModels() []string { return []string{DefaultModel} }

type listedModel struct {
	info    ai.ModelInfo
	lite    bool
	version []int
}

// ListModels returns the models this key can use that LeGaJ works with, in
// Automatic order: Flash-Lite before Flash (the cheaper quota first), newest
// version first (R67). It walks every page of Gemini's models.list.
func (c *Client) ListModels(ctx context.Context) ([]ai.ModelInfo, error) {
	key, err := c.key()
	if err != nil {
		return nil, apperr.Public(err)
	}
	var found []listedModel
	seenID := map[string]bool{}
	seenToken := map[string]bool{}
	token := ""
	for range maxModelPages {
		q := url.Values{"pageSize": {"1000"}}
		if token != "" {
			q.Set("pageToken", token)
		}
		hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+"?"+q.Encode(), nil)
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
		}
		start := time.Now()
		res, err := c.send(ctx, hreq, key)
		outcome := "ok"
		if err != nil {
			outcome = "error"
		}
		logx.LogAICall(c.log, logx.AICall{Provider: "gemini", Model: "(model list)", Status: res.status, Attempt: 1, Duration: time.Since(start), Outcome: outcome})
		if err != nil {
			return nil, err
		}
		var page struct {
			Models []struct {
				Name                       string   `json:"name"`
				DisplayName                string   `json:"displayName"`
				SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(res.body, &page); err != nil {
			return nil, apperr.Wrap(err, apperr.CodeAIUnavailable, "Gemini's list of models couldn't be read. Try again shortly.")
		}
		for _, m := range page.Models {
			id := strings.TrimPrefix(m.Name, "models/")
			match := stableFlash.FindStringSubmatch(id)
			if match == nil || seenID[id] || !slices.Contains(m.SupportedGenerationMethods, "generateContent") {
				continue
			}
			seenID[id] = true
			found = append(found, listedModel{
				info:    ai.ModelInfo{ID: id, DisplayName: displayName(m.DisplayName, id)},
				lite:    match[2] != "",
				version: versionParts(match[1]),
			})
		}
		if page.NextPageToken == "" || seenToken[page.NextPageToken] {
			break
		}
		seenToken[page.NextPageToken] = true
		token = page.NextPageToken
	}
	slices.SortFunc(found, func(a, b listedModel) int {
		if a.lite != b.lite {
			if a.lite {
				return -1
			}
			return 1
		}
		if c := slices.Compare(b.version, a.version); c != 0 {
			return c // newest first
		}
		return cmp.Compare(a.info.ID, b.info.ID)
	})
	out := make([]ai.ModelInfo, len(found))
	for i, m := range found {
		out[i] = m.info
	}
	return out, nil
}

// versionParts turns "3.1" into [3 1], so 10 sorts above 3.1.
func versionParts(v string) []int {
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, _ := strconv.Atoi(p) // the pattern only lets digits through
		out = append(out, n)
	}
	return out
}

// displayName keeps a listed display name short and printable, falling back
// to the ID.
func displayName(name, id string) string {
	name = strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(name))
	if name == "" {
		return id
	}
	if utf8.RuneCountInString(name) > maxDisplayName {
		name = string([]rune(name)[:maxDisplayName])
	}
	return name
}
