package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	// Fetch retrieves targetURL through the proxy and returns the response body.
	Fetch(ctx context.Context, targetURL string) (string, error)
}

// prefixedWebFetchProxy proxies requests by prefixing the target URL.
type prefixedWebFetchProxy struct {
	name   string
	prefix string
}

// scrapelessWebFetchProxy calls the Scrapeless universal scraping API.
type scrapelessWebFetchProxy struct {
	apiURL       string
	apiKey       string
	actor        string
	proxyCountry string
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

var (
	newCrawlerHTTPClient = gutils.NewHTTPClient
	webFetchProxies      []webFetchProxy
)

// Name returns the proxy name.
func (p prefixedWebFetchProxy) Name() string {
	return p.name
}

// Fetch retrieves targetURL through the prefixed proxy endpoint.
func (p prefixedWebFetchProxy) Fetch(ctx context.Context, targetURL string) (string, error) {
	return fetchByWebFetchProxyPrefix(ctx, p.name, p.prefix, targetURL)
}

// Name returns the proxy name.
func (p scrapelessWebFetchProxy) Name() string {
	return "scrapeless"
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

// webFetchProxyResult carries a single proxy fetch attempt result.
type webFetchProxyResult struct {
	name string
	body string
	err  error
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

// fetchByWebFetchProxyRace returns the first successful proxy response body.
func fetchByWebFetchProxyRace(ctx context.Context, targetURL string) (string, error) {
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

	logger := gmw.GetLogger(ctx).Named("fetch_by_web_fetch_proxy_race").With(
		zap.String("url", targetURL),
		zap.Int("n_proxy", len(proxies)),
	)
	resultCh := make(chan webFetchProxyResult, len(proxies))
	raceFns := make([]func(context.Context) error, 0, len(proxies)+1)
	result := webFetchProxyResult{}

	for _, proxy := range proxies {
		proxy := proxy
		raceFns = append(raceFns, func(ctx context.Context) error {
			body, err := proxy.Fetch(ctx, targetURL)
			select {
			case resultCh <- webFetchProxyResult{name: proxy.Name(), body: body, err: err}:
			case <-ctx.Done():
			}

			<-ctx.Done()
			return nil
		})
	}

	raceFns = append(raceFns, func(ctx context.Context) error {
		errMsgs := make([]string, 0, len(proxies))
		for range len(proxies) {
			select {
			case <-ctx.Done():
				return errors.WithStack(ctx.Err())
			case result = <-resultCh:
				if result.err == nil {
					return nil
				}

				errMsgs = append(errMsgs, fmt.Sprintf("%s: %v", result.name, result.err))
			}
		}

		return errors.Errorf("all web fetch proxies failed: %s", strings.Join(errMsgs, "; "))
	})

	if err = gutils.RaceErrWithCtx(ctx, raceFns...); err != nil {
		return "", errors.Wrap(err, "race web fetch proxies")
	}

	logger.Info("web fetch proxy succeeded",
		zap.String("proxy", result.name),
		zap.Int("len", len(result.body)))

	return result.body, nil
}

// buildConfiguredWebFetchProxies builds providers from GPTChat config.
func buildConfiguredWebFetchProxies() ([]webFetchProxy, error) {
	providers := make([]webFetchProxy, 0, 3)
	webFetchConfig := currentWebFetchConfig()

	if webFetchProviderEnabled(webFetchConfig.Jina.Enabled, true) {
		if webFetchConfig.Jina.Prefix == "" {
			return nil, errors.New("jina prefix is empty")
		}
		providers = append(providers, prefixedWebFetchProxy{name: "jina", prefix: webFetchConfig.Jina.Prefix})
	}

	if webFetchProviderEnabled(webFetchConfig.Defuddle.Enabled, true) {
		if webFetchConfig.Defuddle.Prefix == "" {
			return nil, errors.New("defuddle prefix is empty")
		}
		providers = append(providers, prefixedWebFetchProxy{name: "defuddle", prefix: webFetchConfig.Defuddle.Prefix})
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
		}
	}

	return iconfig.Config.WebFetch
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
