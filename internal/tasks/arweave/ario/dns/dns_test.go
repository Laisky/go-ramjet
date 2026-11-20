package dns

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/arweave/config"
	"github.com/Laisky/go-ramjet/library/web"
)

// testAliasFileID is a syntactically valid Arweave transaction id.
const testAliasFileID = "bNbA3TEQVL60xlgCcqdz4ZPHFZ711cZ3hmkpGttDt_U"

// aliasRoundTripper answers alias relays in memory without opening sockets.
type aliasRoundTripper func(*http.Request) (*http.Response, error)

// RoundTrip returns the synthetic response for request.
func (fn aliasRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

// useAliasRecord serves one alias record, name -> fileID, from a loopback S3
// stand-in and installs gateway as the relay client, restoring both afterwards.
func useAliasRecord(t *testing.T, name, fileID string, gateway http.RoundTripper) {
	t.Helper()
	record := `{"records":[{"name":"` + name + `","file_id":"` + fileID + `"}]}`
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(record)))
		w.Header().Set("ETag", `"0123456789abcdef"`)
		w.Header().Set("Last-Modified", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Format(http.TimeFormat))
		_, _ = io.WriteString(w, record)
	}))
	t.Cleanup(store.Close)
	cli, err := minio.New(strings.TrimPrefix(store.URL, "http://"), &minio.Options{
		Creds:  credentials.NewStaticV4("synthetic", "synthetic", ""),
		Region: "us-east-1",
	})
	require.NoError(t, err)

	originalConfig, originalClient := config.Instance, httpcli
	t.Cleanup(func() { config.Instance, httpcli = originalConfig, originalClient })
	config.Instance = &config.Config{S3Cli: cli}
	config.Instance.S3.Bucket = "synthetic"
	httpcli = &http.Client{Transport: gateway}
}

// serveAlias sends request through a router that mounts Query like production.
func serveAlias(request *http.Request) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/arweave/alias/:name", Query)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

// TestQueryRelaysAliasedContentAsUntrusted guards the alias relay, which serves
// Arweave content on the origin shared with GPTChat: the content must be
// sandboxed like the gateway's, gateway headers that act on the origin
// (cookies, service workers, site data) must be dropped, and the caller's
// cookies and credentials must not be forwarded to the gateway.
func TestQueryRelaysAliasedContentAsUntrusted(t *testing.T) {
	var mu sync.Mutex
	var forwarded http.Header
	useAliasRecord(t, "docs", testAliasFileID, aliasRoundTripper(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		forwarded = request.Header.Clone()
		mu.Unlock()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":           []string{"text/html"},
				"Set-Cookie":             []string{"session=attacker; Path=/"},
				"Service-Worker-Allowed": []string{"/"},
				"Clear-Site-Data":        []string{`"storage"`},
				"Etag":                   []string{`"tx"`},
				"X-Ar-Io-Hops":           []string{"1"},
			},
			Body:    io.NopCloser(strings.NewReader("<html><script>steal()</script></html>")),
			Request: request,
		}, nil
	}))

	request := httptest.NewRequest(http.MethodGet, "/arweave/alias/docs", nil)
	request.Header.Set("Cookie", "gptchat=secret")
	request.Header.Set("Authorization", "Bearer synthetic-caller-token")
	request.Header.Set("Accept", "text/html")
	recorder := serveAlias(request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "<html><script>steal()</script></html>", recorder.Body.String())
	require.Equal(t, []string{web.UntrustedContentPolicy}, recorder.Header().Values("Content-Security-Policy"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	require.Equal(t, testAliasFileID, recorder.Header().Get("X-Ar-File-Id"))
	require.Equal(t, `"tx"`, recorder.Header().Get("Etag"))
	for _, name := range []string{"Set-Cookie", "Service-Worker-Allowed", "Clear-Site-Data"} {
		require.Empty(t, recorder.Header().Values(name), "%s must not reach the shared origin", name)
	}

	mu.Lock()
	defer mu.Unlock()
	require.Empty(t, forwarded.Values("Cookie"), "the caller's cookies must stay on this origin")
	require.Empty(t, forwarded.Values("Authorization"), "the caller's credentials must stay on this origin")
	require.Equal(t, "text/html", forwarded.Get("Accept"))
}

// TestQueryDoesNotRelayFailedGatewayAnswers keeps the failure path: a gateway
// error is answered with its status and none of the gateway's headers.
func TestQueryDoesNotRelayFailedGatewayAnswers(t *testing.T) {
	useAliasRecord(t, "docs", testAliasFileID, aliasRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Set-Cookie": []string{"session=attacker"}, "Content-Type": []string{"text/html"}},
			Body:       io.NopCloser(strings.NewReader("<html>gateway error</html>")),
			Request:    request,
		}, nil
	}))

	recorder := serveAlias(httptest.NewRequest(http.MethodGet, "/arweave/alias/docs", nil))
	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Empty(t, recorder.Header().Values("Set-Cookie"))
	require.NotContains(t, recorder.Body.String(), "gateway error")
}

// TestQueryRelaysNotModifiedWithValidators keeps conditional requests working:
// If-None-Match reaches the gateway, and its 304 is answered as a 304 that
// carries the validators and caching headers a cache needs to refresh its
// stored response (RFC 9110, section 15.4.5), but no representation metadata
// such as Content-Type, which would rewrite how the cached body is rendered.
func TestQueryRelaysNotModifiedWithValidators(t *testing.T) {
	var mu sync.Mutex
	var forwarded http.Header
	useAliasRecord(t, "docs", testAliasFileID, aliasRoundTripper(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		forwarded = request.Header.Clone()
		mu.Unlock()
		return &http.Response{
			StatusCode: http.StatusNotModified,
			Header: http.Header{
				"Etag":          []string{`"tx"`},
				"Cache-Control": []string{"public, max-age=31536000, immutable"},
				"Expires":       []string{"Fri, 01 Jan 2027 00:00:00 GMT"},
				"Last-Modified": []string{"Thu, 01 Jan 2026 00:00:00 GMT"},
				"Vary":          []string{"Accept-Encoding"},
				"Content-Type":  []string{"text/html"},
				"Set-Cookie":    []string{"session=attacker"},
			},
			Body:    http.NoBody,
			Request: request,
		}, nil
	}))

	request := httptest.NewRequest(http.MethodGet, "/arweave/alias/docs", nil)
	request.Header.Set("If-None-Match", `"tx"`)
	recorder := serveAlias(request)

	require.Equal(t, http.StatusNotModified, recorder.Code)
	require.Empty(t, recorder.Body.String())
	require.Equal(t, `"tx"`, recorder.Header().Get("Etag"))
	require.Equal(t, "public, max-age=31536000, immutable", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "Fri, 01 Jan 2027 00:00:00 GMT", recorder.Header().Get("Expires"))
	require.Equal(t, "Thu, 01 Jan 2026 00:00:00 GMT", recorder.Header().Get("Last-Modified"))
	require.Equal(t, "Accept-Encoding", recorder.Header().Get("Vary"))
	require.Equal(t, testAliasFileID, recorder.Header().Get("X-Ar-File-Id"))
	require.Empty(t, recorder.Header().Values("Content-Type"), "a 304 must not change the cached type")
	require.Empty(t, recorder.Header().Values("Set-Cookie"))

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, `"tx"`, forwarded.Get("If-None-Match"))
}
