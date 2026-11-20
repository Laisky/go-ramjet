package http

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/egress"
)

// tenantSelectedUser returns a BYOK user whose API base came from X-Laisky-Api-Base.
func tenantSelectedUser(base string) *config.UserConfig {
	return &config.UserConfig{UserName: "byok", Token: egressTestKey, OpenaiToken: egressTestKey,
		BYOK: true, APIBase: base, APIBaseUserSelected: true}
}

// operatorUser returns a user whose API base came from server configuration.
func operatorUser(base string) *config.UserConfig {
	return &config.UserConfig{UserName: "operator", Token: "SYNTHETIC", OpenaiToken: "SYNTHETIC", APIBase: base}
}

// responsesCall invokes one of the two upstream Responses entry points.
type responsesCall func(ctx context.Context, user *config.UserConfig) error

// responsesCalls lists the non-streaming and streaming Responses entry points.
func responsesCalls() map[string]responsesCall {
	return map[string]responsesCall{
		"call": func(ctx context.Context, user *config.UserConfig) error {
			_, _, err := CallUpstreamResponsesCtx(ctx, UpstreamDeps{User: user}, &OpenAIResponsesReq{Model: "synthetic"})
			return err
		},
		"stream": func(ctx context.Context, user *config.UserConfig) error {
			_, _, err := StreamUpstreamResponsesEventsCtx(ctx, UpstreamDeps{User: user}, &OpenAIResponsesReq{Model: "synthetic"},
				func(ResponsesRawEvent) error { return nil })
			return err
		},
	}
}

// TestResponsesUpstreamRefusesTenantSelectedPrivateBase proves the main chat
// path admits a caller-selected API base before sending, so the upstream body
// of an internal service is neither fetched nor relayed in the error.
func TestResponsesUpstreamRefusesTenantSelectedPrivateBase(t *testing.T) {
	for name, call := range responsesCalls() {
		t.Run(name, func(t *testing.T) {
			internal := newInternalService(t, http.StatusInternalServerError, "internal-secret")
			egressTestSetup(t, nil)
			err := call(t.Context(), tenantSelectedUser(internal.URL))
			require.Error(t, err)
			require.True(t, egress.IsExpectedRefusal(err), err.Error())
			require.NotContains(t, err.Error(), "internal-secret")
			require.Zero(t, internal.hits.Load(), "tenant-selected upstream must not reach an internal address")
		})
	}
}

// TestResponsesUpstreamOperatorPrivateBaseUnchanged keeps server-configured
// private upstreams on the server client.
func TestResponsesUpstreamOperatorPrivateBaseUnchanged(t *testing.T) {
	for name, call := range responsesCalls() {
		t.Run(name, func(t *testing.T) {
			operator := newInternalService(t, http.StatusOK, `{"id":"resp_operator","output":[]}`)
			egressTestSetup(t, nil)
			require.NoError(t, call(t.Context(), operatorUser(operator.URL)))
			require.EqualValues(t, 1, operator.hits.Load())
		})
	}
}

// TestTranscriptRefusesTenantSelectedPrivateBase admits the transcription upstream like the chat upstream.
func TestTranscriptRefusesTenantSelectedPrivateBase(t *testing.T) {
	internal := newInternalService(t, http.StatusForbidden, "internal-secret")
	egressTestSetup(t, nil)
	_, err := Transcript(t.Context(), tenantSelectedUser(internal.URL), &TranscriptRequest{File: nopFile{bytes.NewReader([]byte("RIFF"))}, Model: "whisper-1"})
	require.Error(t, err)
	require.True(t, egress.IsExpectedRefusal(err), err.Error())
	require.NotContains(t, err.Error(), "internal-secret")
	require.Zero(t, internal.hits.Load())
}

// nopFile adapts a bytes.Reader to multipart.File.
type nopFile struct{ *bytes.Reader }

// Close implements io.Closer without side effects.
func (nopFile) Close() error { return nil }

// trustLoopbackTLS makes the server client trust server's certificate while
// keeping egressTestSetup's loopback-only dialer.
func trustLoopbackTLS(t *testing.T, server *httptest.Server) {
	t.Helper()
	transport, ok := httpcli.Transport.(*http.Transport)
	require.True(t, ok)
	transport = transport.Clone()
	base, ok := server.Client().Transport.(*http.Transport)
	require.True(t, ok)
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: base.TLSClientConfig.RootCAs}
	httpcli = &http.Client{Transport: transport}
}

// TestPromptURLFetchRefusesInternalTargets proves URLs found in a prompt, and
// search-result URLs fetched statically, are admitted before any request, even
// when the internal service presents a certificate this server trusts.
func TestPromptURLFetchRefusesInternalTargets(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "internal-secret")
	}))
	t.Cleanup(internal.Close)
	egressTestSetup(t, nil)
	trustLoopbackTLS(t, internal)
	target := internal.URL + "/notes.txt"
	require.Equal(t, []string{target}, urlRegexp.FindAllString("summarize "+target+" please", -1),
		"the URL must be reachable from prompt text")

	gin.SetMode(gin.TestMode)
	gctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	gctx.Request = httptest.NewRequest(http.MethodPost, "/chat", nil)
	content, err := FetchURLContent(gctx, target)
	require.Error(t, err)
	require.True(t, egress.IsExpectedRefusal(err), err.Error())
	require.Empty(t, content)

	content, err = fetchStaticURLContent(t.Context(), target)
	require.Error(t, err)
	require.True(t, egress.IsExpectedRefusal(err), err.Error())
	require.Empty(t, content)
	require.Zero(t, hits.Load(), "a prompt-derived URL must not reach an internal address")
}

// TestPromptURLFetchStillReachesPublicTargets keeps static fetches of public URLs working.
func TestPromptURLFetchStillReachesPublicTargets(t *testing.T) {
	provider := newInternalService(t, http.StatusOK, "public-notes")
	egressTestSetup(t, provider.Server)
	content, err := fetchStaticURLContent(t.Context(), "http://"+egressProviderHost+"/notes.txt")
	require.NoError(t, err)
	require.Equal(t, "public-notes", string(content))
}

// TestPromptURLFetchBoundsBody stops a URL from a prompt or a search result
// from streaming an unbounded body into memory: like the crawler, a static
// fetch reads at most crawleregress.MaxResponseBytes, for documents and pages.
func TestPromptURLFetchBoundsBody(t *testing.T) {
	oversized := strings.Repeat("a", crawleregress.MaxResponseBytes+1)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".txt") {
			_, _ = io.WriteString(w, oversized)
			return
		}
		_, _ = io.WriteString(w, "<html><body>"+oversized+"</body></html>")
	}))
	t.Cleanup(provider.Close)
	egressTestSetup(t, provider)

	for _, path := range []string{"/notes.txt", "/page"} {
		content, err := fetchStaticURLContent(t.Context(), "http://"+egressProviderHost+path)
		require.Error(t, err, path)
		require.Contains(t, err.Error(), "exceeds", path)
		require.Empty(t, content, path)
	}

	bounded := strings.Repeat("b", 1024)
	small := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html><body>"+bounded+"</body></html>")
	}))
	t.Cleanup(small.Close)
	egressTestSetup(t, small)
	content, err := fetchStaticURLContent(t.Context(), "http://"+egressProviderHost+"/page")
	require.NoError(t, err)
	require.Contains(t, string(content), bounded)
}

// sseEvent frames one Responses streaming event and flushes it to the client.
// It runs on the server goroutine, so it reports failure instead of asserting.
func sseEvent(w http.ResponseWriter, payload string) bool {
	if _, err := io.WriteString(w, "data: "+payload+"\n\n"); err != nil {
		return false
	}
	flusher, ok := w.(http.Flusher)
	if ok {
		flusher.Flush()
	}
	return ok
}

// TestResponsesStreamThroughPinnedTransport checks that a caller-selected
// public upstream still streams through the pinned transport: each event
// reaches the handler before the upstream sends the next one (no buffering),
// and cancelling the caller's context aborts the upstream request.
func TestResponsesStreamThroughPinnedTransport(t *testing.T) {
	t.Run("incremental", func(t *testing.T) {
		release := make(chan struct{})
		var path atomic.Value
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path.Store(r.URL.Path)
			w.Header().Set("Content-Type", "text/event-stream")
			if !sseEvent(w, `{"type":"response.output_text.delta","delta":"first"}`) {
				return
			}
			// The next event is only sent once the client has observed the
			// first, so any buffering in the transport deadlocks into a failure.
			select {
			case <-release:
			case <-time.After(5 * time.Second):
				return
			}
			_ = sseEvent(w, `{"type":"response.output_text.delta","delta":"second"}`) &&
				sseEvent(w, `{"type":"response.completed","response":{"id":"resp_stream","output":[]}}`) &&
				sseEvent(w, `[DONE]`)
		}))
		t.Cleanup(provider.Close)
		egressTestSetup(t, provider)

		var deltas []string
		final, _, err := StreamUpstreamResponsesEventsCtx(t.Context(),
			UpstreamDeps{User: tenantSelectedUser("http://" + egressProviderHost)},
			&OpenAIResponsesReq{Model: "synthetic"},
			func(event ResponsesRawEvent) error {
				if event.Delta == "first" {
					close(release)
				}
				if event.Delta != "" {
					deltas = append(deltas, event.Delta)
				}
				return nil
			})
		require.NoError(t, err)
		require.Equal(t, "/v1/responses", path.Load())
		require.Equal(t, []string{"first", "second"}, deltas)
		require.Equal(t, "resp_stream", final.ID)
	})

	t.Run("cancellation", func(t *testing.T) {
		upstreamCanceled := make(chan struct{})
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// A model API consumes the request before streaming. Go's server
			// also only notices a closed client connection after that point.
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			if !sseEvent(w, `{"type":"response.output_text.delta","delta":"first"}`) {
				return
			}
			select {
			case <-r.Context().Done():
				close(upstreamCanceled)
			case <-time.After(5 * time.Second):
			}
		}))
		t.Cleanup(provider.Close)
		egressTestSetup(t, provider)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		_, _, err := StreamUpstreamResponsesEventsCtx(ctx,
			UpstreamDeps{User: tenantSelectedUser("http://" + egressProviderHost)},
			&OpenAIResponsesReq{Model: "synthetic"},
			func(ResponsesRawEvent) error {
				cancel()
				return nil
			})
		require.Error(t, err)
		select {
		case <-upstreamCanceled:
		case <-time.After(5 * time.Second):
			require.Fail(t, "caller cancellation must abort the upstream request")
		}
	})
}

// TestMCPToolCallRefusesInternalServer proves a caller-supplied MCP server URL
// (frontend `mcp_servers[].url`) is admitted before any tool call or probe.
func TestMCPToolCallRefusesInternalServer(t *testing.T) {
	internal := newInternalService(t, http.StatusOK, `{"content":[{"type":"text","text":"internal-secret"}]}`)
	egressTestSetup(t, nil)
	resetMCPTransportCache()
	t.Cleanup(resetMCPTransportCache)
	server := &MCPServerConfig{URL: internal.URL, APIKey: "SYNTHETIC", Enabled: true}

	result, err := callMCPTool(t.Context(), server, "tool", `{}`, nil)
	require.Error(t, err)
	require.NotContains(t, result, "internal-secret")
	require.Zero(t, internal.hits.Load(), "a caller-supplied MCP server must not reach an internal address")
}

// mcpFixtureSeq numbers the public-looking hosts handed out by mcpFixtureURL.
var mcpFixtureSeq atomic.Int32

// mcpFixtureURL serves server under a unique public-looking host through
// caller-selected egress and returns that host's base URL. Earlier routes in
// the same test stay reachable; the previous dependencies return on cleanup.
func mcpFixtureURL(t *testing.T, server *httptest.Server) string {
	t.Helper()
	seq := mcpFixtureSeq.Add(1)
	host := fmt.Sprintf("mcp%d.example", seq)
	address := fmt.Sprintf("93.184.216.%d", 10+seq%200)
	previous := tenantEgressDeps
	t.Cleanup(func() { tenantEgressDeps = previous })
	tenantEgressDeps = egress.Dependencies{
		Resolve: func(ctx context.Context, name string) ([]net.IPAddr, error) {
			if name == host {
				return []net.IPAddr{{IP: net.ParseIP(address)}}, nil
			}
			if previous.Resolve != nil {
				return previous.Resolve(ctx, name)
			}
			return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
		},
		Dial: func(ctx context.Context, network, target string) (net.Conn, error) {
			pinned, port, err := net.SplitHostPort(target)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			if pinned != address {
				if previous.Dial != nil {
					return previous.Dial(ctx, network, target)
				}
				return nil, errors.Errorf("unexpected pinned dial to %s", target)
			}
			var dialer net.Dialer
			conn, err := dialer.DialContext(ctx, network, server.Listener.Addr().String())
			if err != nil {
				return nil, errors.WithStack(err)
			}
			portNumber, err := net.LookupPort("tcp", port)
			if err != nil {
				return nil, errors.Join(errors.WithStack(err), conn.Close())
			}
			return &fixturePeerConn{Conn: conn, peer: &net.TCPAddr{IP: net.ParseIP(address), Port: portNumber}}, nil
		},
	}
	return "http://" + host
}
