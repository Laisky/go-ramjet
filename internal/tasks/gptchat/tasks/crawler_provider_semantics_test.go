package tasks

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/stretchr/testify/require"
)

// TestCrawlerProviderGFM preserves table, strikethrough and task-list meaning in
// both output modes while preserving the original Markdown and safe HTML rules.
func TestCrawlerProviderGFM(t *testing.T) {
	content := "# Provider article\n\n| Field | Value |\n| --- | --- |\n| status | ~~old~~ new |\n\n- [x] Complete\n- [ ] Pending\n\n<script>untrusted()</script>"
	for _, markdown := range []bool{true, false} {
		t.Run(fmt.Sprintf("markdown_%t", markdown), func(t *testing.T) {
			result, err := prepareCrawlerProviderContent(t.Context(), content, markdown)
			require.NoError(t, err)
			for _, fragment := range []string{"<table>", "<th>Field</th>", "<td>status</td>", "<del>old</del>", `type="checkbox"`, "checked", "disabled"} {
				require.Contains(t, string(result.body), fragment)
			}
			require.NotContains(t, string(result.body), "<script>")
			if markdown {
				require.Equal(t, content, result.markdown)
			} else {
				require.Empty(t, result.markdown)
			}
		})
	}
}

// TestCrawlerProviderHTMLMedia keeps meaningful HTML-only content when HTML was
// requested, while refusing to manufacture Markdown from an empty conversion.
func TestCrawlerProviderHTMLMedia(t *testing.T) {
	content := `<body><canvas width="64" height="64"></canvas></body>`
	result, err := prepareCrawlerProviderContent(t.Context(), content, false)
	require.NoError(t, err)
	require.Contains(t, string(result.body), "<canvas")
	require.Empty(t, result.markdown)

	result, err = prepareCrawlerProviderContent(t.Context(), content, true)
	require.Error(t, err)
	require.Nil(t, result)

	result, err = prepareCrawlerProviderContent(t.Context(), "<body> \n </body>", false)
	require.Error(t, err)
	require.Nil(t, result)
}

// providerSemanticsResponse encodes an owned content fixture using each real
// adapter's response envelope, returning a provider payload or a test failure.
func providerSemanticsResponse(t *testing.T, provider, content string) []byte {
	t.Helper()
	if provider == "prefix" {
		return []byte(content)
	}
	payload := map[string]any{"data": map[string]any{"content": content}}
	if provider == "firecrawl" {
		payload = map[string]any{"success": true, "data": map[string]any{
			"markdown": content, "metadata": map[string]any{"statusCode": http.StatusOK},
		}}
	}
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	return encoded
}

// TestCrawlerProviderAdapterChallengeExamples exercises actual HTTP adapters
// through acquisition so Markdown quotations survive and active challenges fail.
func TestCrawlerProviderAdapterChallengeExamples(t *testing.T) {
	for _, fixture := range providerHTTPFixtures() {
		for _, scenario := range []struct {
			name, content string
			rejected      bool
		}{
			{name: "ordinary_article", content: "# A useful article\n\nThis is useful content."},
			{name: "fenced_example", content: "# Challenge documentation\n\n```html\n<div id=\"cf-chl-widget\"></div>\n```\n\nUseful explanation."},
			{name: "inline_example", content: "# Challenge documentation\n\nAn inline `<div id=\"cf-chl-widget\"></div>` example."},
			{name: "active_html", content: `<body><div id="cf-chl-widget"></div></body>`, rejected: true},
			{name: "active_markdown", content: "# Just a moment\n\n<div id=\"cf-chl-widget\"></div>", rejected: true},
		} {
			for _, markdown := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/%s/markdown_%t", fixture.name, scenario.name, markdown), func(t *testing.T) {
					preserveVerifiedWorkerSeams(t)
					var requests, dials atomic.Int32
					payload := providerSemanticsResponse(t, fixture.name, scenario.content)
					server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						requests.Add(1)
						_, err := w.Write(payload)
						require.NoError(t, err)
					}))
					t.Cleanup(server.Close)
					deps := rendererTestDependencies(t, server, "93.184.216.34", &dials)
					deps.rootCAs = x509.NewCertPool()
					deps.rootCAs.AddCert(server.Certificate())
					ctx := withVerifiedWebFetchClient(t.Context(), deps)
					fetchVerifiedHTMLContent = func(_ context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
						return `<body><div id="cf-chl-widget"></div></body>`, verifiedTestReceipt(task), nil
					}
					webFetchProxies = []webFetchProxy{fixture.provider("https://example.com")}
					task := fallbackTestTask(t, true)
					task.OutputMarkdown = markdown
					result, err := acquireCrawlerContent(ctx, task)
					if scenario.rejected {
						require.Error(t, err)
						require.Nil(t, result)
					} else {
						require.NoError(t, err)
						require.Equal(t, crawleregress.AcquisitionProvider, result.acquisition.Method)
						require.Contains(t, string(result.body), "<h1>")
						if markdown {
							require.Equal(t, scenario.content, result.markdown)
						} else {
							require.Empty(t, result.markdown)
						}
					}
					require.EqualValues(t, 1, requests.Load())
					require.EqualValues(t, 1, dials.Load())
				})
			}
		}
	}
}

// TestLegacyProviderChallengeValidation retains direct adapter rejection of
// active challenges and its ordinary Markdown response behavior.
func TestLegacyProviderChallengeValidation(t *testing.T) {
	for _, fixture := range providerHTTPFixtures() {
		for _, challenge := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/challenge_%t", fixture.name, challenge), func(t *testing.T) {
				content := "# Legacy article\n\nUseful content."
				if challenge {
					content = `<body><div id="cf-chl-widget"></div></body>`
				}
				payload := providerSemanticsResponse(t, fixture.name, content)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, err := io.WriteString(w, string(payload))
					require.NoError(t, err)
				}))
				t.Cleanup(server.Close)
				body, err := fixture.provider(server.URL).Fetch(t.Context(), "https://example.org/document")
				if challenge {
					require.Error(t, err)
					require.Empty(t, body)
				} else {
					require.NoError(t, err)
					require.Equal(t, content, body)
				}
			})
		}
	}
}
