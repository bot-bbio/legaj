package pipeline

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/bot-bbio/legaj/internal/apperr"
)

// maxURL bounds a pasted or clipped URL.
const maxURL = 2048

// trackingParams are query parameters that only say how a link was shared.
// utm_* is matched by prefix. Parameters that identify the job (gh_jid,
// jk) are deliberately absent.
var trackingParams = map[string]bool{
	"gclid": true, "dclid": true, "gbraid": true, "wbraid": true, "fbclid": true, "msclkid": true,
	"yclid": true, "igshid": true, "mc_cid": true, "mc_eid": true, "_hsenc": true, "_hsmi": true,
	"ref": true, "refid": true, "ref_src": true, "referer": true, "referrer": true,
	"trk": true, "trkinfo": true, "trackingid": true, "lipi": true,
	"src": true, "source": true, "from": true, "gh_src": true, "lever-source": true, "lever-origin": true,
}

var (
	linkedInViewPath = regexp.MustCompile(`^/jobs/view/(?:[^/]*-)?(\d+)/?$`)
	digits           = regexp.MustCompile(`^\d{1,20}$`)
	indeedKey        = regexp.MustCompile(`^[A-Za-z0-9]{1,32}$`)
)

func invalidURL() error {
	return apperr.New(apperr.CodeInvalid, "That doesn't look like a job posting's web address (it should start with https://).")
}

// CanonicalURL reduces the many ways of linking to the same posting to one
// form for dedupe (R15): https, lower-case host without www or a default
// port, no fragment, no trailing slash, no tracking parameters, the rest
// sorted. LinkedIn and Indeed links collapse to their job ID. "" stays "".
func CanonicalURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) > maxURL {
		return "", invalidURL()
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return "", invalidURL()
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if port := u.Port(); port != "" && port != "80" && port != "443" {
		host += ":" + port
	}
	q := u.Query()

	switch {
	case host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com"):
		if id := q.Get("currentJobId"); digits.MatchString(id) {
			return "https://linkedin.com/jobs/view/" + id, nil
		}
		if m := linkedInViewPath.FindStringSubmatch(u.Path); m != nil {
			return "https://linkedin.com/jobs/view/" + m[1], nil
		}
	case host == "indeed.com" || strings.HasSuffix(host, ".indeed.com"):
		for _, key := range []string{"jk", "vjk"} {
			if id := q.Get(key); indeedKey.MatchString(id) {
				return "https://" + host + "/viewjob?jk=" + id, nil
			}
		}
	}

	for k := range q {
		lk := strings.ToLower(k)
		if trackingParams[lk] || strings.HasPrefix(lk, "utm_") {
			q.Del(k)
		}
	}
	path := u.Path
	if path != "/" {
		path = strings.TrimSuffix(path, "/")
	} else {
		path = ""
	}
	out := url.URL{Scheme: "https", Host: host, Path: path, RawQuery: q.Encode()} // Encode sorts by key
	return out.String(), nil
}
