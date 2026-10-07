//go:build browser_integration

package tasks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/log"
)

// TestBrowserDynamicHTMLPreservesHeadersAndScripts verifies the real crawler retains request headers and rendered HTML after the typed-CDP migration.
func TestBrowserDynamicHTMLPreservesHeadersAndScripts(t *testing.T) {
	t.Parallel()
	headers := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		select {
		case headers <- request.Header.Clone():
		default:
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, err := fmt.Fprint(w, `<!doctype html><html><head><title>Migration fixture</title></head><body>
<div id="rendered">Waiting</div><script>setTimeout(() => {
document.getElementById('rendered').textContent = 'Rendered by JavaScript';
}, 25);</script></body></html>`)
		if err != nil {
			t.Errorf("write fixture: %v", err)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	ctx = gmw.SetLogger(ctx, log.Logger.Named("browser_dependency_integration"))
	body, err := fetchDynamicHTMLByChromedp(ctx, server.URL)
	require.NoError(t, err)
	require.Contains(t, body, `<div id="rendered">Rendered by JavaScript</div>`)
	require.Contains(t, body, "<title>Migration fixture</title>")
	select {
	case received := <-headers:
		require.Equal(t, "en-US,en;q=0.8", received.Get("Accept-Language"))
		require.Contains(t, received.Get("User-Agent"), "Mozilla/5.0")
	case <-ctx.Done():
		t.Fatal("crawler did not request the local fixture")
	}
}
