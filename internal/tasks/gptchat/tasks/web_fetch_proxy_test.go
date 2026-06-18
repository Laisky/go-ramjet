package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	iconfig "github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// stubWebFetchProxy is a test double for webFetchProxy.
type stubWebFetchProxy struct {
	name     string
	priority int
	delay    time.Duration
	body     string
	err      error
}

// Name returns the stub proxy name.
func (p stubWebFetchProxy) Name() string {
	return p.name
}

// Priority returns the stub proxy priority.
func (p stubWebFetchProxy) Priority() int {
	return p.priority
}

// Fetch waits for the configured delay and then returns the stubbed result.
func (p stubWebFetchProxy) Fetch(ctx context.Context, _ string) (string, error) {
	if p.delay > 0 {
		select {
		case <-time.After(p.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	if p.err != nil {
		return "", p.err
	}

	return p.body, nil
}

// boolPtr returns a pointer to the provided boolean.
func boolPtr(v bool) *bool {
	return &v
}

// intPtr returns a pointer to the provided int.
func intPtr(v int) *int {
	return &v
}

// Test_fetchByWebFetchProxyPriorityPrefersHigherPriority verifies the highest-priority proxy wins.
func Test_fetchByWebFetchProxyPriorityPrefersHigherPriority(t *testing.T) {
	original := webFetchProxies
	webFetchProxies = []webFetchProxy{
		stubWebFetchProxy{name: "low", priority: 10, body: "low-body"},
		stubWebFetchProxy{name: "high", priority: 100, body: "high-body"},
		stubWebFetchProxy{name: "mid", priority: 50, body: "mid-body"},
	}
	t.Cleanup(func() {
		webFetchProxies = original
	})

	body, err := fetchByWebFetchProxyPriority(context.Background(), "https://example.com")
	require.NoError(t, err)
	require.Equal(t, "high-body", body)
}

// Test_fetchByWebFetchProxyPriorityFallsBackToLowerPriority verifies fallback when higher tiers fail.
func Test_fetchByWebFetchProxyPriorityFallsBackToLowerPriority(t *testing.T) {
	original := webFetchProxies
	webFetchProxies = []webFetchProxy{
		stubWebFetchProxy{name: "high", priority: 100, err: errors.New("high fail")},
		stubWebFetchProxy{name: "low", priority: 10, body: "low-body"},
	}
	t.Cleanup(func() {
		webFetchProxies = original
	})

	body, err := fetchByWebFetchProxyPriority(context.Background(), "https://example.com")
	require.NoError(t, err)
	require.Equal(t, "low-body", body)
}

// Test_fetchByWebFetchProxyPriorityRandomizesSamePriority verifies same-priority proxies are both selected over many runs.
func Test_fetchByWebFetchProxyPriorityRandomizesSamePriority(t *testing.T) {
	original := webFetchProxies
	webFetchProxies = []webFetchProxy{
		stubWebFetchProxy{name: "a", priority: 100, body: "a"},
		stubWebFetchProxy{name: "b", priority: 100, body: "b"},
	}
	t.Cleanup(func() {
		webFetchProxies = original
	})

	seen := map[string]int{}
	for range 200 {
		body, err := fetchByWebFetchProxyPriority(context.Background(), "https://example.com")
		require.NoError(t, err)
		seen[body]++
	}
	require.Positive(t, seen["a"], "proxy a should win at least once")
	require.Positive(t, seen["b"], "proxy b should win at least once")
}

// Test_fetchByWebFetchProxyPriorityReturnsErrorWhenAllFail verifies aggregated failure reporting when no proxy succeeds.
func Test_fetchByWebFetchProxyPriorityReturnsErrorWhenAllFail(t *testing.T) {
	original := webFetchProxies
	webFetchProxies = []webFetchProxy{
		stubWebFetchProxy{name: "jina", err: errors.New("jina fail")},
		stubWebFetchProxy{name: "defuddle", err: errors.New("defuddle fail")},
	}
	t.Cleanup(func() {
		webFetchProxies = original
	})

	body, err := fetchByWebFetchProxyPriority(context.Background(), "https://example.com")
	require.Error(t, err)
	require.Empty(t, body)
	require.Contains(t, err.Error(), "all web fetch proxies failed")
	require.Contains(t, err.Error(), "jina")
	require.Contains(t, err.Error(), "defuddle")
}

// Test_registeredWebFetchProxiesUsesConfig verifies config-driven providers are built as sibling providers.
func Test_registeredWebFetchProxiesUsesConfig(t *testing.T) {
	originalOverrides := webFetchProxies
	originalConfig := iconfig.Config
	webFetchProxies = nil
	iconfig.Config = &iconfig.OpenAI{
		WebFetch: iconfig.WebFetchConfig{
			Jina: iconfig.PrefixWebFetchProxyConfig{
				Enabled: boolPtr(true),
				Prefix:  "https://r.jina.ai/",
				// Priority unset: the build path should default it to 100.
			},
			Defuddle: iconfig.PrefixWebFetchProxyConfig{
				Enabled: boolPtr(false),
				Prefix:  "https://defuddle.md/",
			},
			Scrapeless: iconfig.ScrapelessWebFetchProxyConfig{
				Enabled:      boolPtr(true),
				API:          "https://api.scrapeless.com/api/v2/unlocker/request",
				APIKey:       "test-key",
				Actor:        "unlocker.webunlocker",
				ProxyCountry: "ANY",
				// Priority unset: the build path should default it to 50.
			},
			Firecrawl: iconfig.FirecrawlWebFetchProxyConfig{
				Enabled:  boolPtr(true),
				API:      "https://api.firecrawl.dev/v2/scrape",
				APIKey:   "fc-test-key",
				Priority: intPtr(0), // explicit 0 must be honored, not defaulted to 50.
			},
		},
	}
	t.Cleanup(func() {
		webFetchProxies = originalOverrides
		iconfig.Config = originalConfig
	})

	proxies, err := registeredWebFetchProxies()
	require.NoError(t, err)
	require.Len(t, proxies, 3)
	require.Equal(t, "jina", proxies[0].Name())
	require.Equal(t, 100, proxies[0].Priority())
	require.Equal(t, "scrapeless", proxies[1].Name())
	require.Equal(t, 50, proxies[1].Priority())
	require.Equal(t, "firecrawl", proxies[2].Name())
	require.Equal(t, 0, proxies[2].Priority())
}

// Test_webFetchProviderPriority verifies nil falls back to the default while explicit values are honored.
func Test_webFetchProviderPriority(t *testing.T) {
	require.Equal(t, 100, webFetchProviderPriority(nil, 100))
	require.Equal(t, 0, webFetchProviderPriority(intPtr(0), 100))
	require.Equal(t, -5, webFetchProviderPriority(intPtr(-5), 100))
	require.Equal(t, 7, webFetchProviderPriority(intPtr(7), 100))
}

// Test_extractFirecrawlContent verifies Firecrawl scrape responses produce markdown content.
func Test_extractFirecrawlContent(t *testing.T) {
	t.Run("markdown", func(t *testing.T) {
		body, err := extractFirecrawlContent([]byte(`{"success":true,"data":{"markdown":"# Hello\nworld","metadata":{"statusCode":200}}}`))
		require.NoError(t, err)
		require.Equal(t, "# Hello\nworld", body)
	})

	t.Run("target failure status", func(t *testing.T) {
		_, err := extractFirecrawlContent([]byte(`{"success":true,"data":{"markdown":"Not Found","metadata":{"statusCode":404}}}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "status 404")
	})

	t.Run("error response", func(t *testing.T) {
		_, err := extractFirecrawlContent([]byte(`{"success":false,"error":"Unauthorized: Invalid token"}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "Unauthorized")
	})

	t.Run("empty content", func(t *testing.T) {
		_, err := extractFirecrawlContent([]byte(`{"success":true,"data":{"markdown":""}}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty content")
	})
}

// Test_extractScrapelessContent verifies nested Scrapeless JSON payloads can produce content.
func Test_extractScrapelessContent(t *testing.T) {
	body, err := extractScrapelessContent([]byte(`{"data":{"content":"<html><body>ok</body></html>"}}`))
	require.NoError(t, err)
	require.Equal(t, "<html><body>ok</body></html>", body)
}
