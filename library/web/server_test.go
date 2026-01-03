package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNormalizeHandlerDuplicatePrefix verifies duplicated task prefixes are normalized.
func TestNormalizeHandlerDuplicatePrefix(t *testing.T) {
	var gotPath string
	h := &normalizeHandler{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/gptchat/gptchat/favicon.ico", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "/gptchat/favicon.ico", gotPath)
}
