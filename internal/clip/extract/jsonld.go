package extract

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	maxBlocks     = 20      // the collector sends at most 20 blocks
	maxBlockBytes = 1 << 20 // a bigger block isn't a job posting
	maxDepth      = 8       // JobPosting nests inside @graph or mainEntity, never deeply
	maxNodes      = 5000    // values visited per block while searching
	maxPlaces     = 3       // places listed before "(+N more)"
)

// fromJSONLD offers the first JobPosting found in the blocks (C32).
func fromJSONLD(blocks []string) candidates {
	for i, raw := range blocks {
		if i >= maxBlocks {
			break
		}
		v, ok := parseBlock(raw)
		if !ok {
			continue
		}
		budget := maxNodes
		jp := findJobPosting(v, 0, &budget)
		if jp == nil {
			continue
		}
		return candidates{
			company:     []string{orgName(jp["hiringOrganization"])},
			role:        []string{text(jp["title"])},
			location:    []string{jobLocation(jp)},
			description: []string{text(jp["description"])},
			markup:      true,
		}
	}
	return candidates{}
}

// parseBlock decodes one JSON-LD block, tolerating the wrappers and raw
// line breaks some sites leave in it.
func parseBlock(raw string) (any, bool) {
	s := strings.TrimSpace(raw)
	if len(s) > maxBlockBytes {
		return nil, false
	}
	for _, w := range [][2]string{{"<!--", "-->"}, {"//<![CDATA[", "//]]>"}, {"<![CDATA[", "]]>"}} {
		s = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, w[0]), w[1]))
	}
	var v any
	if json.Unmarshal([]byte(s), &v) == nil {
		return v, true
	}
	// JSON forbids raw control characters inside strings; some sites emit
	// line breaks there anyway. Outside strings a space is equivalent.
	fixed := strings.Map(func(r rune) rune {
		if r < 0x20 {
			return ' '
		}
		return r
	}, s)
	if json.Unmarshal([]byte(fixed), &v) == nil {
		return v, true
	}
	return nil, false
}

// findJobPosting searches v depth-first, in a stable key order.
func findJobPosting(v any, depth int, budget *int) map[string]any {
	if depth > maxDepth || *budget <= 0 {
		return nil
	}
	*budget--
	switch t := v.(type) {
	case map[string]any:
		if isType(t["@type"], "JobPosting") {
			return t
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			if jp := findJobPosting(t[k], depth+1, budget); jp != nil {
				return jp
			}
		}
	case []any:
		for _, x := range t {
			if jp := findJobPosting(x, depth+1, budget); jp != nil {
				return jp
			}
		}
	}
	return nil
}

// isType reports whether a JSON-LD @type (a string or a list) names want,
// with or without a schema.org prefix.
func isType(v any, want string) bool {
	match := func(s string) bool {
		s = strings.TrimSpace(s)
		return strings.EqualFold(s, want) || strings.HasSuffix(s, "/"+want) || strings.HasSuffix(s, ":"+want)
	}
	switch t := v.(type) {
	case string:
		return match(t)
	case []any:
		for _, x := range t {
			if s, ok := x.(string); ok && match(s) {
				return true
			}
		}
	}
	return false
}

// text returns a string value, or the "name" of an object, or the first
// such value of a list. Anything else is "".
func text(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		if s, ok := t["name"].(string); ok {
			return s
		}
	case []any:
		for _, x := range t {
			if s := text(x); strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

// orgName reads hiringOrganization: a name, an Organization, or a list.
func orgName(v any) string {
	if m, ok := v.(map[string]any); ok {
		if s := text(m["name"]); s != "" {
			return s
		}
		return text(m["legalName"])
	}
	return text(v)
}

// jobLocation describes where the job is: "Remote" (with the countries
// applicants must live in) and up to three places.
func jobLocation(jp map[string]any) string {
	var parts []string
	if isType(jp["jobLocationType"], "TELECOMMUTE") {
		remote := "Remote"
		if req := names(jp["applicantLocationRequirements"]); len(req) > 0 {
			remote += " (" + strings.Join(req, ", ") + ")"
		}
		parts = append(parts, remote)
	}
	var places []string
	var raw []any
	switch t := jp["jobLocation"].(type) {
	case []any:
		raw = t
	case nil:
	default:
		raw = []any{t}
	}
	for _, p := range raw {
		if s := place(p); s != "" && !containsFold(places, s) {
			places = append(places, s)
		}
	}
	if len(places) > maxPlaces {
		more := len(places) - maxPlaces
		places = places[:maxPlaces]
		places[maxPlaces-1] += fmt.Sprintf(" (+%d more)", more)
	}
	return strings.Join(append(parts, places...), "; ")
}

// place formats a Place: its address parts, its address text, or its name.
func place(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return text(v)
	}
	switch a := m["address"].(type) {
	case string:
		return a
	case map[string]any:
		return joinUnique(text(a["addressLocality"]), text(a["addressRegion"]), text(a["addressCountry"]))
	}
	return text(m["name"])
}

// names lists the distinct names in a value (a name, an object or a list).
func names(v any) []string {
	var out []string
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" && !containsFold(out, s) {
			out = append(out, s)
		}
	}
	if list, ok := v.([]any); ok {
		for _, x := range list {
			add(text(x))
		}
	} else {
		add(text(v))
	}
	return out
}

// joinUnique joins the comma-separated segments of parts, dropping blanks
// and repeats ("Singapore, Singapore" is one place).
func joinUnique(parts ...string) string {
	var segs []string
	for _, p := range parts {
		for _, s := range strings.Split(p, ",") {
			if s = strings.TrimSpace(s); s != "" && !containsFold(segs, s) {
				segs = append(segs, s)
			}
		}
	}
	return strings.Join(segs, ", ")
}

func containsFold(list []string, s string) bool {
	return slices.ContainsFunc(list, func(x string) bool { return strings.EqualFold(x, s) })
}
