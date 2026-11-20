package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// oneshotRoundTripper answers requests in memory without opening sockets.
type oneshotRoundTripper func(*http.Request) (*http.Response, error)

// RoundTrip returns the synthetic response for request.
func (fn oneshotRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

// oneshotAnswer is a minimal chat completion response.
const oneshotAnswer = `{"choices":[{"message":{"content":"synthetic-answer"}}]}`

// TestOneshotChatUsesSuppliedClient sends through the WithHTTPClient client,
// which is how callers apply the egress policy to user-selected API bases.
func TestOneshotChatUsesSuppliedClient(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: oneshotRoundTripper(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		require.Equal(t, "https://provider.example/v1/chat/completions", request.URL.String())
		require.Equal(t, "Bearer SYNTHETIC-KEY", request.Header.Get("Authorization"))
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(oneshotAnswer)), Request: request}, nil
	})}
	answer, err := OneshotChat(t.Context(), "https://provider.example/", "SYNTHETIC-KEY", "", "", "hi", WithHTTPClient(client))
	require.NoError(t, err)
	require.Equal(t, "synthetic-answer", answer)
	require.EqualValues(t, 1, calls.Load())

	refusing := &http.Client{Transport: oneshotRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("refused by policy")
	})}
	_, err = OneshotChat(t.Context(), "https://provider.example", "SYNTHETIC-KEY", "", "", "hi", WithHTTPClient(refusing))
	require.ErrorContains(t, err, "refused by policy")
}

// TestOneshotChatDefaultClientUnchanged keeps the default client for callers
// with operator-configured bases, which may be private, and for a nil option.
func TestOneshotChatDefaultClientUnchanged(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		_, err := io.WriteString(w, oneshotAnswer)
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	for _, opts := range [][]OneshotOption{nil, {WithHTTPClient(nil)}} {
		answer, err := OneshotChat(t.Context(), server.URL, "SYNTHETIC-KEY", "", "", "hi", opts...)
		require.NoError(t, err)
		require.Equal(t, "synthetic-answer", answer)
	}
	require.EqualValues(t, 2, calls.Load())
}
