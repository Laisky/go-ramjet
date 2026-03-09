package tasks

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"

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

var (
	newCrawlerHTTPClient = gutils.NewHTTPClient
	webFetchProxies      = []webFetchProxy{
		prefixedWebFetchProxy{name: "jina", prefix: "https://r.jina.ai/"},
		prefixedWebFetchProxy{name: "defuddle", prefix: "https://defuddle.md/"},
	}
)

// Name returns the proxy name.
func (p prefixedWebFetchProxy) Name() string {
	return p.name
}

// Fetch retrieves targetURL through the prefixed proxy endpoint.
func (p prefixedWebFetchProxy) Fetch(ctx context.Context, targetURL string) (string, error) {
	return fetchByWebFetchProxyPrefix(ctx, p.name, p.prefix, targetURL)
}

// webFetchProxyResult carries a single proxy fetch attempt result.
type webFetchProxyResult struct {
	name string
	body string
	err  error
}

// registeredWebFetchProxies returns a snapshot of configured web fetch proxies.
func registeredWebFetchProxies() []webFetchProxy {
	proxies := make([]webFetchProxy, len(webFetchProxies))
	copy(proxies, webFetchProxies)
	return proxies
}

// fetchByWebFetchProxyRace returns the first successful proxy response body.
func fetchByWebFetchProxyRace(ctx context.Context, targetURL string) (string, error) {
	if strings.TrimSpace(targetURL) == "" {
		return "", errors.New("targetURL is empty")
	}

	proxies := registeredWebFetchProxies()
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

	if err := gutils.RaceErrWithCtx(ctx, raceFns...); err != nil {
		return "", errors.Wrap(err, "race web fetch proxies")
	}

	logger.Info("web fetch proxy succeeded",
		zap.String("proxy", result.name),
		zap.Int("len", len(result.body)))

	return result.body, nil
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
