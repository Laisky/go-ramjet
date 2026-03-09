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
	name  string
	delay time.Duration
	body  string
	err   error
}

// Name returns the stub proxy name.
func (p stubWebFetchProxy) Name() string {
	return p.name
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

// Test_fetchByWebFetchProxyRaceReturnsFirstSuccess verifies the first successful proxy wins the race.
func Test_fetchByWebFetchProxyRaceReturnsFirstSuccess(t *testing.T) {
	original := webFetchProxies
	webFetchProxies = []webFetchProxy{
		stubWebFetchProxy{name: "fast-fail", delay: 5 * time.Millisecond, err: errors.New("boom")},
		stubWebFetchProxy{name: "first-success", delay: 15 * time.Millisecond, body: "first"},
		stubWebFetchProxy{name: "later-success", delay: 30 * time.Millisecond, body: "later"},
	}
	t.Cleanup(func() {
		webFetchProxies = original
	})

	body, err := fetchByWebFetchProxyRace(context.Background(), "https://example.com")
	require.NoError(t, err)
	require.Equal(t, "first", body)
}

// Test_fetchByWebFetchProxyRaceReturnsErrorWhenAllFail verifies aggregated failure reporting when no proxy succeeds.
func Test_fetchByWebFetchProxyRaceReturnsErrorWhenAllFail(t *testing.T) {
	original := webFetchProxies
	webFetchProxies = []webFetchProxy{
		stubWebFetchProxy{name: "jina", err: errors.New("jina fail")},
		stubWebFetchProxy{name: "defuddle", err: errors.New("defuddle fail")},
	}
	t.Cleanup(func() {
		webFetchProxies = original
	})

	body, err := fetchByWebFetchProxyRace(context.Background(), "https://example.com")
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
			},
		},
	}
	t.Cleanup(func() {
		webFetchProxies = originalOverrides
		iconfig.Config = originalConfig
	})

	proxies, err := registeredWebFetchProxies()
	require.NoError(t, err)
	require.Len(t, proxies, 2)
	require.Equal(t, "jina", proxies[0].Name())
	require.Equal(t, "scrapeless", proxies[1].Name())
}

// Test_extractScrapelessContent verifies nested Scrapeless JSON payloads can produce content.
func Test_extractScrapelessContent(t *testing.T) {
	body, err := extractScrapelessContent([]byte(`{"data":{"content":"<html><body>ok</body></html>"}}`))
	require.NoError(t, err)
	require.Equal(t, "<html><body>ok</body></html>", body)
}
