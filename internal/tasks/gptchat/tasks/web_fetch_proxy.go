package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"

	iconfig "github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/log"
)

// webFetchProxy fetches page content through a proxy service.
type webFetchProxy interface {
	// Name returns the proxy identifier used in logs and errors.
	Name() string
	// Priority returns the selection priority; higher runs first, ties are random.
	Priority() int
	// Fetch retrieves targetURL through the proxy and returns the response body.
	Fetch(ctx context.Context, targetURL string) (string, error)
}

// prefixedWebFetchProxy proxies requests by prefixing the target URL.
type prefixedWebFetchProxy struct {
	name     string
	prefix   string
	priority int
}

// scrapelessWebFetchProxy calls the Scrapeless universal scraping API.
type scrapelessWebFetchProxy struct {
	apiURL       string
	apiKey       string
	actor        string
	proxyCountry string
	priority     int
}

// scrapelessRequestPayload is the request body sent to Scrapeless.
type scrapelessRequestPayload struct {
	Actor string                 `json:"actor"`
	Input scrapelessRequestInput `json:"input"`
	Proxy scrapelessRequestProxy `json:"proxy"`
}

// scrapelessRequestInput contains the target request details.
type scrapelessRequestInput struct {
	URL      string         `json:"url"`
	Method   string         `json:"method"`
	Redirect bool           `json:"redirect"`
	Headers  map[string]any `json:"headers,omitempty"`
}

// scrapelessRequestProxy contains proxy routing options.
type scrapelessRequestProxy struct {
	Country string `json:"country"`
}

// firecrawlWebFetchProxy calls the Firecrawl scrape API.
type firecrawlWebFetchProxy struct {
	apiURL   string
	apiKey   string
	priority int
}

// firecrawlRequestPayload is the request body sent to Firecrawl.
type firecrawlRequestPayload struct {
	URL     string   `json:"url"`
	Formats []string `json:"formats"`
}

// firecrawlResponse is the response body returned by the Firecrawl scrape API.
type firecrawlResponse struct {
	Success bool                  `json:"success"`
	Error   string                `json:"error"`
	Data    firecrawlResponseData `json:"data"`
}

// firecrawlResponseData carries the scraped content and target metadata.
type firecrawlResponseData struct {
	Markdown string            `json:"markdown"`
	Metadata firecrawlMetadata `json:"metadata"`
}

// firecrawlMetadata carries metadata about the scraped target.
type firecrawlMetadata struct {
	// StatusCode is the HTTP status the target site returned. It is distinct from
	// the Firecrawl API's own status, which stays 200 even when the target failed.
	StatusCode int `json:"statusCode"`
}

var (
	newCrawlerHTTPClient = gutils.NewHTTPClient
	webFetchProxies      []webFetchProxy
)

// Name returns the proxy name.
func (p prefixedWebFetchProxy) Name() string {
	return p.name
}

// Priority returns the proxy selection priority.
func (p prefixedWebFetchProxy) Priority() int {
	return p.priority
}

// Fetch retrieves targetURL through the prefixed proxy endpoint.
func (p prefixedWebFetchProxy) Fetch(ctx context.Context, targetURL string) (string, error) {
	return fetchByWebFetchProxyPrefix(ctx, p.name, p.prefix, targetURL)
}

// Name returns the proxy name.
func (p scrapelessWebFetchProxy) Name() string {
	return "scrapeless"
}

// Priority returns the proxy selection priority.
func (p scrapelessWebFetchProxy) Priority() int {
	return p.priority
}

// Fetch retrieves targetURL through the Scrapeless API.
func (p scrapelessWebFetchProxy) Fetch(ctx context.Context, targetURL string) (string, error) {
	if strings.TrimSpace(targetURL) == "" {
		return "", errors.New("targetURL is empty")
	}
	if strings.TrimSpace(p.apiKey) == "" {
		return "", errors.New("scrapeless api key is empty")
	}

	payload := scrapelessRequestPayload{
		Actor: p.actor,
		Input: scrapelessRequestInput{
			URL:      targetURL,
			Method:   http.MethodGet,
			Redirect: false,
		},
		Proxy: scrapelessRequestProxy{Country: p.proxyCountry},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", errors.Wrap(err, "marshal scrapeless payload")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", errors.Wrap(err, "new scrapeless request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-token", p.apiKey)

	cli, err := newCrawlerHTTPClient()
	if err != nil {
		return "", errors.Wrap(err, "new scrapeless http client")
	}

	resp, err := cli.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "scrapeless request")
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "read scrapeless body")
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", errors.Errorf("scrapeless request failed: %d body=%s", resp.StatusCode, truncateForLog(string(body), 256))
	}

	content, err := extractScrapelessContent(body)
	if err != nil {
		return "", errors.Wrap(err, "extract scrapeless content")
	}

	return content, nil
}

// Name returns the proxy name.
func (p firecrawlWebFetchProxy) Name() string {
	return "firecrawl"
}

// Priority returns the proxy selection priority.
func (p firecrawlWebFetchProxy) Priority() int {
	return p.priority
}

// Fetch retrieves targetURL through the Firecrawl scrape API.
func (p firecrawlWebFetchProxy) Fetch(ctx context.Context, targetURL string) (string, error) {
	if strings.TrimSpace(targetURL) == "" {
		return "", errors.New("targetURL is empty")
	}
	if strings.TrimSpace(p.apiKey) == "" {
		return "", errors.New("firecrawl api key is empty")
	}

	payload := firecrawlRequestPayload{
		URL:     targetURL,
		Formats: []string{"markdown"},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", errors.Wrap(err, "marshal firecrawl payload")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", errors.Wrap(err, "new firecrawl request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	cli, err := newCrawlerHTTPClient()
	if err != nil {
		return "", errors.Wrap(err, "new firecrawl http client")
	}

	resp, err := cli.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "firecrawl request")
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "read firecrawl body")
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", errors.Errorf("firecrawl request failed: %d body=%s", resp.StatusCode, truncateForLog(string(body), 256))
	}

	content, err := extractFirecrawlContent(body)
	if err != nil {
		return "", errors.Wrap(err, "extract firecrawl content")
	}

	return content, nil
}

// registeredWebFetchProxies returns a snapshot of configured web fetch proxies.
func registeredWebFetchProxies() ([]webFetchProxy, error) {
	if webFetchProxies != nil {
		proxies := make([]webFetchProxy, len(webFetchProxies))
		copy(proxies, webFetchProxies)
		return proxies, nil
	}

	providers, err := buildConfiguredWebFetchProxies()
	if err != nil {
		return nil, errors.Wrap(err, "build configured web fetch proxies")
	}

	proxies := make([]webFetchProxy, len(providers))
	copy(proxies, providers)
	return proxies, nil
}

// fetchByWebFetchProxyPriority returns the first successful proxy response body.
//
// Providers are tried sequentially in descending priority order; providers that
// share a priority are tried in random order so load spreads across them, while
// lower-priority providers act only as fallbacks. The first success wins, and a
// provider is only attempted once the higher-priority ones have failed.
func fetchByWebFetchProxyPriority(ctx context.Context, targetURL string) (string, error) {
	if strings.TrimSpace(targetURL) == "" {
		return "", errors.New("targetURL is empty")
	}

	proxies, err := registeredWebFetchProxies()
	if err != nil {
		return "", errors.Wrap(err, "register web fetch proxies")
	}
	if len(proxies) == 0 {
		return "", errors.New("no web fetch proxies configured")
	}

	orderWebFetchProxiesByPriority(proxies)

	logger := gmw.GetLogger(ctx).Named("fetch_by_web_fetch_proxy_priority").With(
		zap.String("url", targetURL),
		zap.Int("n_proxy", len(proxies)),
	)

	errMsgs := make([]string, 0, len(proxies))
	for _, proxy := range proxies {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", errors.Wrap(ctxErr, "web fetch proxy context done")
		}

		logger.Debug("start web fetch proxy request",
			zap.String("proxy", proxy.Name()),
			zap.Int("priority", proxy.Priority()))
		body, ferr := proxy.Fetch(ctx, targetURL)
		if ferr != nil {
			logger.Debug("web fetch proxy request failed",
				zap.String("proxy", proxy.Name()),
				zap.Error(ferr))
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %v", proxy.Name(), ferr))
			continue
		}

		logger.Info("web fetch proxy succeeded",
			zap.String("proxy", proxy.Name()),
			zap.Int("priority", proxy.Priority()),
			zap.Int("len", len(body)))
		return body, nil
	}

	return "", errors.Errorf("all web fetch proxies failed: %s", strings.Join(errMsgs, "; "))
}

// orderWebFetchProxiesByPriority sorts proxies in place by descending priority,
// shuffling first so providers sharing a priority are ordered randomly.
func orderWebFetchProxiesByPriority(proxies []webFetchProxy) {
	rand.Shuffle(len(proxies), func(i, j int) {
		proxies[i], proxies[j] = proxies[j], proxies[i]
	})
	sort.SliceStable(proxies, func(i, j int) bool {
		return proxies[i].Priority() > proxies[j].Priority()
	})
}

// buildConfiguredWebFetchProxies builds providers from GPTChat config.
func buildConfiguredWebFetchProxies() ([]webFetchProxy, error) {
	providers := make([]webFetchProxy, 0, 4)
	webFetchConfig := currentWebFetchConfig()

	if webFetchProviderEnabled(webFetchConfig.Jina.Enabled, true) {
		if webFetchConfig.Jina.Prefix == "" {
			return nil, errors.New("jina prefix is empty")
		}
		providers = append(providers, prefixedWebFetchProxy{
			name:     "jina",
			prefix:   webFetchConfig.Jina.Prefix,
			priority: webFetchProviderPriority(webFetchConfig.Jina.Priority, 100),
		})
	}

	if webFetchProviderEnabled(webFetchConfig.Defuddle.Enabled, true) {
		if webFetchConfig.Defuddle.Prefix == "" {
			return nil, errors.New("defuddle prefix is empty")
		}
		providers = append(providers, prefixedWebFetchProxy{
			name:     "defuddle",
			prefix:   webFetchConfig.Defuddle.Prefix,
			priority: webFetchProviderPriority(webFetchConfig.Defuddle.Priority, 100),
		})
	}

	if webFetchProviderEnabled(webFetchConfig.Scrapeless.Enabled, false) {
		if webFetchConfig.Scrapeless.API == "" {
			return nil, errors.New("scrapeless api is empty")
		}
		if strings.TrimSpace(webFetchConfig.Scrapeless.APIKey) == "" {
			return nil, errors.New("scrapeless api key is empty")
		}
		providers = append(providers, scrapelessWebFetchProxy{
			apiURL:       webFetchConfig.Scrapeless.API,
			apiKey:       webFetchConfig.Scrapeless.APIKey,
			actor:        webFetchConfig.Scrapeless.Actor,
			proxyCountry: webFetchConfig.Scrapeless.ProxyCountry,
			priority:     webFetchProviderPriority(webFetchConfig.Scrapeless.Priority, 50),
		})
	}

	if webFetchProviderEnabled(webFetchConfig.Firecrawl.Enabled, false) {
		if webFetchConfig.Firecrawl.API == "" {
			return nil, errors.New("firecrawl api is empty")
		}
		if strings.TrimSpace(webFetchConfig.Firecrawl.APIKey) == "" {
			return nil, errors.New("firecrawl api key is empty")
		}
		providers = append(providers, firecrawlWebFetchProxy{
			apiURL:   webFetchConfig.Firecrawl.API,
			apiKey:   webFetchConfig.Firecrawl.APIKey,
			priority: webFetchProviderPriority(webFetchConfig.Firecrawl.Priority, 50),
		})
	}

	return providers, nil
}

// webFetchProviderEnabled returns the configured enabled state or the provided default.
func webFetchProviderEnabled(enabled *bool, defaultValue bool) bool {
	if enabled == nil {
		return defaultValue
	}

	return *enabled
}

// webFetchProviderPriority returns the configured priority or the provided default
// when unset (nil). An explicit value, including 0 or negative, is honored as-is.
func webFetchProviderPriority(priority *int, defaultValue int) int {
	if priority == nil {
		return defaultValue
	}

	return *priority
}

// currentWebFetchConfig returns the configured web fetch providers or zero values when config is unavailable.
func currentWebFetchConfig() iconfig.WebFetchConfig {
	if iconfig.Config == nil {
		return iconfig.WebFetchConfig{
			Jina:     iconfig.PrefixWebFetchProxyConfig{Prefix: "https://r.jina.ai/"},
			Defuddle: iconfig.PrefixWebFetchProxyConfig{Prefix: "https://defuddle.md/"},
			Scrapeless: iconfig.ScrapelessWebFetchProxyConfig{
				API:          "https://api.scrapeless.com/api/v2/unlocker/request",
				Actor:        "unlocker.webunlocker",
				ProxyCountry: "ANY",
			},
			Firecrawl: iconfig.FirecrawlWebFetchProxyConfig{
				API: "https://api.firecrawl.dev/v2/scrape",
			},
		}
	}

	return iconfig.Config.WebFetch
}

// extractFirecrawlContent extracts the markdown payload from a Firecrawl scrape response.
func extractFirecrawlContent(body []byte) (string, error) {
	var resp firecrawlResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", errors.Wrap(err, "unmarshal firecrawl body")
	}
	if !resp.Success {
		if resp.Error != "" {
			return "", errors.Errorf("firecrawl returned error: %s", truncateForLog(resp.Error, 256))
		}

		return "", errors.New("firecrawl returned unsuccessful response")
	}

	// Firecrawl keeps its own HTTP status at 200 / success:true even when the
	// target page itself failed; the target status lives in data.metadata.statusCode.
	// Reject non-2xx targets so the race falls through to other proxies instead of
	// surfacing a soft-failure (error or challenge page) as a successful fetch.
	if code := resp.Data.Metadata.StatusCode; code != 0 &&
		(code < http.StatusOK || code >= http.StatusMultipleChoices) {
		return "", errors.Errorf("firecrawl target returned status %d", code)
	}

	content := strings.TrimSpace(resp.Data.Markdown)
	if content == "" {
		return "", errors.New("firecrawl returned empty content")
	}
	if isCloudflareChallenge(content) {
		return "", errors.New("firecrawl returned cloudflare challenge")
	}

	return content, nil
}

// extractScrapelessContent extracts the first useful string payload from a Scrapeless response.
func extractScrapelessContent(body []byte) (string, error) {
	content := strings.TrimSpace(string(body))
	if content == "" {
		return "", errors.New("scrapeless returned empty body")
	}
	if content[0] != '{' && content[0] != '[' {
		if isCloudflareChallenge(content) {
			return "", errors.New("scrapeless returned cloudflare challenge")
		}

		return content, nil
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", errors.Wrap(err, "unmarshal scrapeless body")
	}

	if extracted := findFirstStringValue(payload, "content", "html", "body", "markdown", "text", "data"); extracted != "" {
		if isCloudflareChallenge(extracted) {
			return "", errors.New("scrapeless returned cloudflare challenge")
		}

		return extracted, nil
	}

	return "", errors.Errorf("scrapeless content not found in response: %s", truncateForLog(content, 256))
}

// findFirstStringValue searches nested JSON-like data for the first non-empty string under preferred keys.
func findFirstStringValue(value any, preferredKeys ...string) string {
	for _, key := range preferredKeys {
		if extracted := findStringValueForKey(value, key); extracted != "" {
			return strings.TrimSpace(extracted)
		}
	}

	return ""
}

// findStringValueForKey recursively searches for a non-empty string under the provided key.
func findStringValueForKey(value any, key string) string {
	switch typed := value.(type) {
	case map[string]any:
		if direct, ok := typed[key]; ok {
			if text := directStringValue(direct); text != "" {
				return text
			}
		}

		for _, nested := range typed {
			if text := findStringValueForKey(nested, key); text != "" {
				return text
			}
		}
	case []any:
		for _, nested := range typed {
			if text := findStringValueForKey(nested, key); text != "" {
				return text
			}
		}
	}

	return ""
}

// directStringValue unwraps nested values until it finds a string payload.
func directStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, nested := range typed {
			if text := directStringValue(nested); text != "" {
				return text
			}
		}
	case []any:
		for _, nested := range typed {
			if text := directStringValue(nested); text != "" {
				return text
			}
		}
	}

	return ""
}

// truncateForLog shortens log payloads while keeping the prefix readable.
func truncateForLog(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 || len(s) <= limit {
		return s
	}

	return s[:limit] + "..."
}

// fetchByWebFetchProxyPrefix fetches targetURL through a prefixed web proxy endpoint.
func fetchByWebFetchProxyPrefix(ctx context.Context, proxyName, prefix, targetURL string) (string, error) {
	if strings.TrimSpace(targetURL) == "" {
		return "", errors.New("targetURL is empty")
	}
	if strings.TrimSpace(prefix) == "" {
		return "", errors.Errorf("proxy %q prefix is empty", proxyName)
	}

	requestURL := prefix + targetURL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", errors.Wrapf(err, "new %s request", proxyName)
	}

	cli, err := newCrawlerHTTPClient()
	if err != nil {
		return "", errors.Wrapf(err, "new %s http client", proxyName)
	}

	resp, err := cli.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "%s request", proxyName)
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", errors.Errorf("%s request failed: %d", proxyName, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "read %s body", proxyName)
	}

	content := string(body)
	if strings.TrimSpace(content) == "" {
		return "", errors.Errorf("%s returned empty body", proxyName)
	}
	if isCloudflareChallenge(content) {
		return "", errors.Errorf("%s returned cloudflare challenge", proxyName)
	}

	return content, nil
}
