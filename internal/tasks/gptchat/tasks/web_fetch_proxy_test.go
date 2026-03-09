package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
