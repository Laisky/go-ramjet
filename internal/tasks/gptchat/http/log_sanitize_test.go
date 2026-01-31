package http

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizePayloadForLog_RedactsBase64AndSecrets(t *testing.T) {
	b64 := strings.Repeat("A", 128)
	payload := map[string]any{
		"image":   "data:image/png;base64," + b64,
		"api_key": "secret-key",
		"nested": map[string]any{
			"token": "secret-token",
			"value": "ok",
		},
		"raw": b64,
	}

	sanitized := sanitizePayloadForLog(payload).(map[string]any)
	require.Equal(t, "[redacted]", sanitized["api_key"])

	imageVal := sanitized["image"].(string)
	require.Contains(t, imageVal, "base64,")
	require.Contains(t, imageVal, "[base64 len=")
	require.NotContains(t, imageVal, b64)

	nested := sanitized["nested"].(map[string]any)
	require.Equal(t, "[redacted]", nested["token"])
	require.Equal(t, "ok", nested["value"])

	require.Contains(t, sanitized["raw"].(string), "[base64 len=")
}
