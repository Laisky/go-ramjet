package http

import (
	stdjson "encoding/json"
	"fmt"
	"strings"
)

const base64RedactionMarker = "[base64 len=%d truncated]"

// sanitizePayloadForLog converts payload into a JSON-compatible structure with sensitive data redacted.
// It truncates large values and replaces base64 content with a compact marker to prevent log bloat.
func sanitizePayloadForLog(payload any) any {
	if payload == nil {
		return nil
	}

	raw, err := stdjson.Marshal(payload)
	if err != nil {
		return map[string]any{"_error": "marshal payload", "detail": err.Error()}
	}

	var decoded any
	if err := stdjson.Unmarshal(raw, &decoded); err != nil {
		return map[string]any{"_error": "unmarshal payload", "detail": err.Error()}
	}

	return sanitizeValueForLog(decoded, maxLogValueLen)
}

// sanitizeValueForLog walks maps/slices and sanitizes string values for logging.
func sanitizeValueForLog(v any, maxLen int) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case string:
		return sanitizeStringForLog(val, maxLen)
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, vv := range val {
			if isSensitiveKeyForLog(k) {
				result[k] = "[redacted]"
				continue
			}
			result[k] = sanitizeValueForLog(vv, maxLen)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = sanitizeValueForLog(item, maxLen)
		}
		return result
	default:
		return truncateValue(v, maxLen)
	}
}

// sanitizeStringForLog redacts base64 payloads and truncates long strings.
func sanitizeStringForLog(s string, maxLen int) string {
	if strings.Contains(s, "base64,") {
		idx := strings.Index(s, "base64,")
		prefixEnd := idx + len("base64,")
		b64 := s[prefixEnd:]
		return s[:prefixEnd] + redactBase64StringForLog(b64)
	}

	if isLikelyBase64ForLog(s) {
		return redactBase64StringForLog(s)
	}

	return truncateForLog(s, maxLen)
}

// redactBase64StringForLog replaces base64 data with a compact marker.
func redactBase64StringForLog(b64 string) string {
	return fmt.Sprintf(base64RedactionMarker, len(b64))
}

// isLikelyBase64ForLog heuristically detects base64 strings to avoid logging raw file contents.
func isLikelyBase64ForLog(s string) bool {
	if len(s) < 64 || len(s)%4 != 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=' {
			continue
		}
		return false
	}
	return true
}

// isSensitiveKeyForLog returns true when the key likely contains credentials.
func isSensitiveKeyForLog(k string) bool {
	key := strings.ToLower(strings.TrimSpace(k))
	if key == "" {
		return false
	}
	for _, token := range []string{"api_key", "apikey", "token", "authorization", "secret", "password"} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}
