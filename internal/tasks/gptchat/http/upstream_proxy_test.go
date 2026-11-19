package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/egress"
)

// recordingProxy is a forward-proxy fixture that records the absolute
// targets it was asked for and answers them itself.
type recordingProxy struct {
	*httptest.Server
	mu      sync.Mutex
	targets []string
}

// newRecordingProxy starts a forward proxy answering every target with body.
func newRecordingProxy(t *testing.T, body string) *recordingProxy {
	t.Helper()
	proxy := &recordingProxy{}
	proxy.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.mu.Lock()
		proxy.targets = append(proxy.targets, r.URL.String())
		proxy.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(proxy.Close)
	return proxy
}

// seen returns the targets the proxy received so far.
func (p *recordingProxy) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.targets...)
}

// TestTenantEgressUsesOperatorProxy keeps a deployment's outbound proxy in
// the path of caller-selected requests that used the server client before:
// the target is still admitted first, and refused targets never reach the proxy.
func TestTenantEgressUsesOperatorProxy(t *testing.T) {
	direct := newInternalService(t, http.StatusOK, "direct")
	egressTestSetup(t, direct.Server)
	proxy := newRecordingProxy(t, "via-proxy")
	t.Setenv("HTTP_PROXY", proxy.URL)
	require.NoError(t, SetupHTTPCli())

	content, err := fetchStaticURLContent(t.Context(), "http://"+egressProviderHost+"/notes.txt")
	require.NoError(t, err)
	require.Equal(t, "via-proxy", string(content))
	require.Equal(t, []string{"http://" + egressProviderHost + "/notes.txt"}, proxy.seen())
	require.Zero(t, direct.hits.Load(), "a proxied deployment does not connect directly")

	_, err = fetchStaticURLContent(t.Context(), direct.URL+"/notes.txt")
	require.Error(t, err)
	require.True(t, egress.IsExpectedRefusal(err), err.Error())
	require.Len(t, proxy.seen(), 1, "a refused target never reaches the proxy")
}

// TestTenantEgressDirectWithoutProxy keeps the pinned direct path when the
// deployment configures no proxy.
func TestTenantEgressDirectWithoutProxy(t *testing.T) {
	direct := newInternalService(t, http.StatusOK, "direct")
	egressTestSetup(t, direct.Server)
	t.Setenv("HTTP_PROXY", "")
	require.NoError(t, SetupHTTPCli())

	content, err := fetchStaticURLContent(t.Context(), "http://"+egressProviderHost+"/notes.txt")
	require.NoError(t, err)
	require.Equal(t, "direct", string(content))
	require.EqualValues(t, 1, direct.hits.Load())
}

// TestOneshotChatStaysDirectWithProxy keeps one-shot chat on its historic
// proxy-less client: it never used the operator proxy, so admitted requests
// still connect directly to the pinned address.
func TestOneshotChatStaysDirectWithProxy(t *testing.T) {
	direct := newInternalService(t, http.StatusOK, `{"choices":[{"message":{"content":"direct-answer"}}]}`)
	egressTestSetup(t, direct.Server)
	proxy := newRecordingProxy(t, `{"choices":[{"message":{"content":"proxy-answer"}}]}`)
	t.Setenv("HTTP_PROXY", proxy.URL)
	require.NoError(t, SetupHTTPCli())

	answer, err := userOneshotChat(t.Context(), tenantSelectedUser("http://"+egressProviderHost), "", "", "hi")
	require.NoError(t, err)
	require.Equal(t, "direct-answer", answer)
	require.Empty(t, proxy.seen())
}

// TestTenantClientsKeepTheServerTimeouts gives caller-selected requests the
// server client's budget: the response-header wait is not shorter than the
// whole exchange, so non-streaming model calls keep their 30 minutes.
func TestTenantClientsKeepTheServerTimeouts(t *testing.T) {
	egressTestSetup(t, nil)
	client := pinnedTenantClient()
	require.Equal(t, 30*time.Minute, client.Timeout)
	transport, ok := client.Transport.(egress.PinnedTransport)
	require.True(t, ok)
	require.Equal(t, client.Timeout, transport.ResponseHeaderTimeout)
	require.Equal(t, client.Timeout, tenantEgressClient().Timeout)
}
