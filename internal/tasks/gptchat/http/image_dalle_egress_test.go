package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/egress"
	"github.com/Laisky/go-ramjet/library/log"
)

// egressTestKey is a synthetic BYOK credential; it is never a real key.
const egressTestKey = "sk-SYNTHETIC-ONLY-EGRESS-0123456789"

// egressProviderHost is a public-looking provider name that tests route to a loopback fixture.
const egressProviderHost = "provider.example"

// internalService is a loopback HTTP server standing in for an internal endpoint.
type internalService struct {
	*httptest.Server
	hits atomic.Int32
}

// newInternalService starts an internal endpoint that counts requests and
// answers every one with status and body.
func newInternalService(t *testing.T, status int, body string) *internalService {
	t.Helper()
	service := &internalService{}
	service.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		service.hits.Add(1)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(service.Close)
	return service
}

// egressProviderAddress is the public address the fixture resolver answers for egressProviderHost.
const egressProviderAddress = "93.184.216.34"

// fixturePeerConn reports a synthetic public peer for a connection to a loopback fixture.
type fixturePeerConn struct {
	net.Conn
	peer net.Addr
}

// RemoteAddr returns the synthetic public peer instead of the fixture's loopback address.
func (c *fixturePeerConn) RemoteAddr() net.Addr { return c.peer }

// egressFixtureDeps resolves egressProviderHost to egressProviderAddress and
// connects that pinned address to provider. Every other name is unknown and
// every other dial fails, so caller-selected egress never touches the network.
func egressFixtureDeps(provider *httptest.Server) egress.Dependencies {
	return egress.Dependencies{
		Resolve: func(_ context.Context, host string) ([]net.IPAddr, error) {
			if host != egressProviderHost || provider == nil {
				return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
			}
			return []net.IPAddr{{IP: net.ParseIP(egressProviderAddress)}}, nil
		},
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			if provider == nil || host != egressProviderAddress {
				return nil, errors.Errorf("unexpected pinned dial to %s", address)
			}
			var dialer net.Dialer
			conn, err := dialer.DialContext(ctx, network, provider.Listener.Addr().String())
			if err != nil {
				return nil, errors.WithStack(err)
			}
			portNumber, err := net.LookupPort("tcp", port)
			if err != nil {
				return nil, errors.Join(errors.WithStack(err), conn.Close())
			}
			return &fixturePeerConn{Conn: conn, peer: &net.TCPAddr{IP: net.ParseIP(host), Port: portNumber}}, nil
		},
	}
}

// egressTestSetup isolates gptchat globals, refuses default-transport traffic
// and installs a server client that reaches loopback fixtures only. Requests
// for egressProviderHost on port 80 are routed to provider when it is not nil,
// both through the server client and through caller-selected egress.
func egressTestSetup(t *testing.T, provider *httptest.Server) {
	t.Helper()
	originalConfig, originalClient, originalLogger := config.Config, httpcli, log.Logger
	originalTransport, originalEgress := http.DefaultTransport, tenantEgressDeps
	limiterInitialized := freeModelRateLimiter != nil || expensiveModelRateLimiter != nil
	t.Cleanup(func() {
		config.Config, httpcli, log.Logger = originalConfig, originalClient, originalLogger
		http.DefaultTransport, tenantEgressDeps = originalTransport, originalEgress
		onceLimiter = sync.Once{}
		if limiterInitialized {
			onceLimiter.Do(func() {})
		}
	})
	setupTestConfig()
	onceLimiter = sync.Once{}
	onceLimiter.Do(func() {})
	http.DefaultTransport = byokAuditTransport(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("unmocked HTTP request prohibited")
	})
	logger, _ := embeddingTestLogger(t)
	log.Logger = logger
	tenantEgressDeps = egressFixtureDeps(provider)

	routes := map[string]string{}
	if provider != nil {
		routes[net.JoinHostPort(egressProviderHost, "80")] = provider.Listener.Addr().String()
	}
	httpcli = &http.Client{Transport: &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if routed, ok := routes[address]; ok {
				address = routed
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
				return nil, errors.Errorf("unexpected dial to %s", address)
			}
			var dialer net.Dialer
			conn, err := dialer.DialContext(ctx, network, address)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return conn, nil
		},
	}}
}

// egressRequestContext returns a handler context for a BYOK caller that sends
// payload to path and selects apiBase through X-Laisky-Api-Base when not empty.
func egressRequestContext(t *testing.T, path, apiBase string, payload any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("Authorization", "Bearer "+egressTestKey)
	if apiBase != "" {
		ctx.Request.Header.Set("X-Laisky-Api-Base", apiBase)
	}
	return ctx, recorder
}

// testPNGBase64 returns a one-pixel PNG encoded as base64.
func testPNGBase64(t *testing.T) string {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 1, 1))
	canvas.Set(0, 0, color.White)
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, canvas))
	return base64.StdEncoding.EncodeToString(encoded.Bytes())
}

// TestImageHandlersRefuseTenantSelectedPrivateUpstream proves that a BYOK
// caller cannot point X-Laisky-Api-Base at an internal service: image
// generation (OpenAI and Azure shapes) and image editing must refuse the
// destination before any request reaches it, and never echo its response.
func TestImageHandlersRefuseTenantSelectedPrivateUpstream(t *testing.T) {
	for _, tc := range []struct {
		name    string
		path    string
		suffix  string
		handler gin.HandlerFunc
		payload func(t *testing.T) any
	}{
		{name: "generation", path: "/images/generations", handler: DrawByDalleHandler, payload: func(*testing.T) any {
			return map[string]any{"prompt": "synthetic", "model": "dall-e-3", "n": 1}
		}},
		{name: "azure_generation", path: "/images/generations", suffix: "/openai.azure.com", handler: DrawByDalleHandler, payload: func(*testing.T) any {
			return map[string]any{"prompt": "synthetic", "model": "dall-e-3", "n": 1}
		}},
		{name: "edit", path: "/images/edits", handler: EditImageHandler, payload: func(t *testing.T) any {
			return map[string]any{"prompt": "synthetic", "model": "gpt-image-1", "image": testPNGBase64(t)}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			internal := newInternalService(t, http.StatusInternalServerError, "internal-secret")
			egressTestSetup(t, nil)
			ctx, recorder := egressRequestContext(t, tc.path, internal.URL+tc.suffix, tc.payload(t))
			tc.handler(ctx)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, recorder.Body.String(), string(egress.ReasonDestinationDenied))
			require.NotContains(t, recorder.Body.String(), "internal-secret")
			require.Zero(t, internal.hits.Load(), "tenant-selected upstream must not reach an internal address")
		})
	}
}

// TestEditImageRefusesPrivateImageURL proves that an image URL supplied in the
// edit request is admitted before download, for every caller.
func TestEditImageRefusesPrivateImageURL(t *testing.T) {
	internal := newInternalService(t, http.StatusOK, "internal-secret")
	egressTestSetup(t, nil)
	ctx, recorder := egressRequestContext(t, "/images/edits", "", map[string]any{
		"prompt": "synthetic", "model": "gpt-image-1", "image": internal.URL + "/latest/meta-data",
	})
	EditImageHandler(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), string(egress.ReasonDestinationDenied))
	require.NotContains(t, recorder.Body.String(), "internal-secret")
	require.Zero(t, internal.hits.Load(), "a caller-supplied image URL must not reach an internal address")
}

// TestTenantUpstreamCannotRedirectDownloadsInternally proves that an image URL
// returned by a tenant-selected upstream is admitted like the upstream itself,
// so a provider the caller controls cannot make this server fetch internal URLs.
func TestTenantUpstreamCannotRedirectDownloadsInternally(t *testing.T) {
	internal := newInternalService(t, http.StatusOK, "internal-secret")
	var providerHits atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"url":"`+internal.URL+`/latest/meta-data"}]}`)
	}))
	t.Cleanup(provider.Close)
	egressTestSetup(t, provider)
	ctx, _ := egressRequestContext(t, "/images/generations", "http://"+egressProviderHost, nil)
	user, err := getUserByAuthHeader(ctx)
	require.NoError(t, err)

	images, err := fetchImageFromOpenaiDalle(ctx, user, "dall-e-3", "synthetic", 1, "")
	require.Error(t, err)
	require.True(t, egress.IsExpectedRefusal(err), "the internal URL must be refused by admission")
	require.Empty(t, images)
	require.EqualValues(t, 1, providerHits.Load(), "the public tenant upstream itself stays reachable")
	require.Zero(t, internal.hits.Load(), "a URL from a tenant-selected upstream must not reach an internal address")
}

// TestOneShotChatRefusesTenantSelectedPrivateUpstream proves the one-shot chat
// endpoint admits a tenant-selected API base before sending, and never echoes
// an internal response body back to the caller.
func TestOneShotChatRefusesTenantSelectedPrivateUpstream(t *testing.T) {
	internal := newInternalService(t, http.StatusForbidden, "internal-secret")
	egressTestSetup(t, nil)
	ctx, recorder := egressRequestContext(t, "/chat/oneshot", internal.URL, map[string]any{
		"system_prompt": "synthetic", "user_prompt": "synthetic",
	})
	OneShotChatHandler(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), string(egress.ReasonDestinationDenied))
	require.NotContains(t, recorder.Body.String(), "internal-secret")
	require.Zero(t, internal.hits.Load(), "tenant-selected upstream must not reach an internal address")
}
