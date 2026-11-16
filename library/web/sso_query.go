package web

import (
	"net/http"
	"net/url"
	"strings"
)

// redactSSOQuery removes callback credentials before diagnostics and reports whether privacy headers are needed.
// The browser retains its original navigation URL for PKCE; no server route consumes these fields.
func redactSSOQuery(r *http.Request) bool {
	if r.URL == nil || r.URL.RawQuery == "" {
		return false
	}
	query, changed := redactSSOQueryValues(r.URL.RawQuery)
	if changed {
		r.URL.RawQuery = query
		r.RequestURI = r.URL.RequestURI()
	}
	return changed
}

// redactSSOQueryValues redacts sensitive query values without re-encoding unrelated bytes.
func redactSSOQueryValues(query string) (string, bool) {
	parts := strings.Split(query, "&")
	changed := false
	for i, part := range parts {
		name, _, _ := strings.Cut(part, "=")
		decoded, err := url.QueryUnescape(name)
		if err != nil {
			// Malformed names cannot be classified safely; retain other well-formed parameters.
			parts[i] = "redacted_invalid_query=REDACTED"
			changed = true
			continue
		}
		switch strings.ToLower(decoded) {
		case "sso_code", "sso_token":
			parts[i] = name + "=REDACTED"
			changed = true
		}
	}
	if !changed {
		return query, false
	}
	return strings.Join(parts, "&"), true
}

// redactSSOReferer removes callback query credentials from every Referer value before middleware sees requests.
func redactSSOReferer(r *http.Request) {
	values := r.Header.Values("Referer")
	for i, value := range values {
		prefix, rest, found := strings.Cut(value, "?")
		if !found {
			continue
		}
		query, fragment, hasFragment := strings.Cut(rest, "#")
		redacted, changed := redactSSOQueryValues(query)
		if !changed {
			continue
		}
		values[i] = prefix + "?" + redacted
		if hasFragment {
			values[i] += "#" + fragment
		}
	}
}
