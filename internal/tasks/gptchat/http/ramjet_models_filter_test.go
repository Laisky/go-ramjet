package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// TestIsOneAPIModelListRequest verifies model-list endpoint detection for OneAPI proxy paths.
func TestIsOneAPIModelListRequest(t *testing.T) {
	t.Parallel()

	t.Run("match gptchat prefixed path", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/gptchat/oneapi/v1/models", nil)
		require.True(t, isOneAPIModelListRequest(req))
	})

	t.Run("match oneapi relative path", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/oneapi/v1/models", nil)
		require.True(t, isOneAPIModelListRequest(req))
	})

	t.Run("non get method should not match", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodPost, "/gptchat/oneapi/v1/models", nil)
		require.False(t, isOneAPIModelListRequest(req))
	})

	t.Run("different path should not match", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/gptchat/oneapi/v1/chat/completions", nil)
		require.False(t, isOneAPIModelListRequest(req))
	})
}

// TestFilterOneAPIModelListPayloadByUser ensures free users only receive allowlisted models.
func TestFilterOneAPIModelListPayloadByUser(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"object":"list","data":[{"id":"gpt-5"},{"id":"gpt-4o-mini"},{"id":"claude-4-sonnet"}]}`)

	t.Run("restricted user gets filtered model list", func(t *testing.T) {
		t.Parallel()

		user := &config.UserConfig{
			UserName:      "free-user",
			AllowedModels: []string{"gpt-4o-mini", "gpt-5"},
		}

		filtered, filteredCnt, upstreamCnt, err := filterOneAPIModelListPayloadByUser(user, payload)
		require.NoError(t, err)
		require.Equal(t, 3, upstreamCnt)
		require.Equal(t, 2, filteredCnt)
		require.JSONEq(t,
			`{"object":"list","data":[{"id":"gpt-5"},{"id":"gpt-4o-mini"}]}`,
			string(filtered),
		)
	})

	t.Run("wildcard user keeps upstream model list", func(t *testing.T) {
		t.Parallel()

		user := &config.UserConfig{
			UserName:      "paid-user",
			AllowedModels: []string{"*"},
		}

		filtered, filteredCnt, upstreamCnt, err := filterOneAPIModelListPayloadByUser(user, payload)
		require.NoError(t, err)
		require.Equal(t, 3, upstreamCnt)
		require.Equal(t, 3, filteredCnt)
		require.JSONEq(t, string(payload), string(filtered))
	})

	t.Run("byok user keeps upstream model list", func(t *testing.T) {
		t.Parallel()

		user := &config.UserConfig{
			UserName:      "byok-user",
			BYOK:          true,
			AllowedModels: []string{"gpt-4o-mini"},
		}

		filtered, filteredCnt, upstreamCnt, err := filterOneAPIModelListPayloadByUser(user, payload)
		require.NoError(t, err)
		require.Equal(t, 3, upstreamCnt)
		require.Equal(t, 3, filteredCnt)
		require.JSONEq(t, string(payload), string(filtered))
	})

	t.Run("invalid payload returns parse error", func(t *testing.T) {
		t.Parallel()

		user := &config.UserConfig{
			UserName:      "free-user",
			AllowedModels: []string{"gpt-4o-mini"},
		}

		_, _, _, err := filterOneAPIModelListPayloadByUser(user, []byte("not-json"))
		require.Error(t, err)
	})
}
