package web

import (
	"net/http"
	"net/url"
	"strings"
)

// redactSSOQuery removes callback credentials from server-side request diagnostics before middleware runs.
// The browser retains its original navigation URL for the PKCE bootstrap; no server route consumes these fields.
func redactSSOQuery(r *http.Request) {
	if r.URL == nil || r.URL.RawQuery == "" {
		return
	}
	parts := strings.Split(r.URL.RawQuery, "&")
	for i, part := range parts {
		name, _, _ := strings.Cut(part, "=")
		decoded, err := url.QueryUnescape(name)
		if err != nil {
			// Malformed names cannot be classified safely. Preserve other well-formed parameters.
			parts[i] = "redacted_invalid_query=REDACTED"
			continue
		}
		switch strings.ToLower(decoded) {
		case "sso_code", "sso_token":
			parts[i] = name + "=REDACTED"
		}
	}
	r.URL.RawQuery = strings.Join(parts, "&")
	r.RequestURI = r.URL.RequestURI()
}
