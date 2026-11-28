package http

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// recordedUpstream is a model API fixture that records what it received.
type recordedUpstream struct {
	*httptest.Server
	hits          atomic.Int32
	path          atomic.Value
	authorization atomic.Value
}

// newRecordedUpstream starts a fixture answering every request with body.
func newRecordedUpstream(t *testing.T, body func(r *http.Request) string) *recordedUpstream {
	t.Helper()
	upstream := &recordedUpstream{}
	upstream.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstream.hits.Add(1)
		upstream.path.Store(r.URL.Path)
		upstream.authorization.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body(r))
	}))
	t.Cleanup(upstream.Close)
	return upstream
}

// TestTenantSelectedPublicUpstreamStillWorks keeps BYOK image generation and
// one-shot chat working against a public provider the caller selected; the
// request is pinned to the admitted address and carries the caller's key.
func TestTenantSelectedPublicUpstreamStillWorks(t *testing.T) {
	image := testPNGBase64(t)
	provider := newRecordedUpstream(t, func(r *http.Request) string {
		if strings.HasSuffix(r.URL.Path, "/chat/completions") {
			return `{"choices":[{"message":{"content":"synthetic-answer"}}]}`
		}
		return `{"data":[{"b64_json":"` + image + `"}]}`
	})
	egressTestSetup(t, provider.Server)

	ctx, _ := egressRequestContext(t, "/images/generations", "http://"+egressProviderHost, nil)
	user, err := getUserByAuthHeader(ctx)
	require.NoError(t, err)
	require.True(t, user.APIBaseUserSelected)
	images, err := fetchImageFromOpenaiDalle(ctx, user, "dall-e-3", "synthetic", 1, "")
	require.NoError(t, err)
	require.Len(t, images, 1)
	require.Equal(t, "/v1/images/generations", provider.path.Load())
	require.Equal(t, "Bearer "+egressTestKey, provider.authorization.Load())

	ctx, recorder := egressRequestContext(t, "/chat/oneshot", "http://"+egressProviderHost, map[string]any{"user_prompt": "synthetic"})
	OneShotChatHandler(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "synthetic-answer")
	require.Equal(t, "/v1/chat/completions", provider.path.Load())
}

// TestOperatorConfiguredPrivateUpstreamUnchanged preserves server-configured
// upstreams on private networks: they are operator trust, not caller input,
// so neither the upstream nor the image URLs it returns are admitted.
func TestOperatorConfiguredPrivateUpstreamUnchanged(t *testing.T) {
	asset := newRecordedUpstream(t, func(*http.Request) string { return "private-image-bytes" })
	operator := newRecordedUpstream(t, func(r *http.Request) string {
		if strings.HasSuffix(r.URL.Path, "/chat/completions") {
			return `{"choices":[{"message":{"content":"operator-answer"}}]}`
		}
		return `{"data":[{"url":"` + asset.URL + `/generated.png"}]}`
	})
	egressTestSetup(t, nil)
	config.Config.OneAPIBase = operator.URL
	config.Config.UserTokens[0].APIBase = operator.URL
	config.Config.UserTokens[0].ImageUrl = operator.URL + "/v1/images/generations"

	ctx, recorder := egressRequestContext(t, "/chat/oneshot", "", map[string]any{"user_prompt": "synthetic"})
	ctx.Request.Header.Del("Authorization")
	ctx.Request.Header.Set("X-Laisky-Api-Base", "http://ignored-for-free-tier.example")
	user, err := getUserByAuthHeader(ctx)
	require.NoError(t, err)
	require.True(t, user.IsFree)
	require.False(t, user.APIBaseUserSelected)

	OneShotChatHandler(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "operator-answer")

	images, err := fetchImageFromOpenaiDalle(ctx, user, "dall-e-3", "synthetic", 1, "")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("private-image-bytes")}, images)
	require.EqualValues(t, 1, asset.hits.Load())
}

// TestDownloadImageBoundsBody stops a download at maxDownloadedImageBytes
// instead of reading an unbounded body into memory.
func TestDownloadImageBoundsBody(t *testing.T) {
	egressTestSetup(t, nil)
	for _, tc := range []struct {
		name string
		size int
		ok   bool
	}{
		{name: "at_limit", size: maxDownloadedImageBytes, ok: true},
		{name: "over_limit", size: maxDownloadedImageBytes + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(bytes.Repeat([]byte{'x'}, tc.size))
			}))
			t.Cleanup(server.Close)
			content, err := downloadImage(t.Context(), httpcli, server.URL+"/image.png")
			if tc.ok {
				require.NoError(t, err)
				require.Len(t, content, tc.size)
				return
			}
			require.ErrorContains(t, err, "image exceeds")
			require.Nil(t, content)
		})
	}
}
