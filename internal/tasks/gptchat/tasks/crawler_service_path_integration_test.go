package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	iconfig "github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	appLog "github.com/Laisky/go-ramjet/library/log"
)

// serviceHeaderTimeout reproduces a transport header timeout independently of
// caller or worker cancellation; no historical request timing is asserted.
type serviceHeaderTimeout struct{}

// Error uses the standard transport's header-timeout diagnostic text.
func (serviceHeaderTimeout) Error() string { return "net/http: timeout awaiting response headers" }

// Timeout marks this deterministic fixture error as a typed transport timeout.
func (serviceHeaderTimeout) Timeout() bool { return true }

// Temporary satisfies net.Error for the synthetic transport failure.
func (serviceHeaderTimeout) Temporary() bool { return true }

// TestCrawlerServicePathWorker is a test-binary helper driven by the GraphQL
// socket integration test. Redis and all HTTP traffic are owned local fixtures.
// It runs the real worker processor, egress HTTP fetcher, local conversion,
// persistence and wire encoder. Chrome is replaced, and RemoteAddr is synthetic;
// this test is not published-image or real-public-peer attestation.
func TestCrawlerServicePathWorker(t *testing.T) {
	socket := os.Getenv("CRAWLER_SERVICE_TEST_REDIS_SOCKET")
	if socket == "" {
		t.Skip("run through TestMCPWebFetchServicePath with a compiled Ramjet test binary")
	}
	require.True(t, strings.HasSuffix(socket, "/redis.sock"), "helper requires an owned disposable Redis socket")
	preserveVerifiedWorkerSeams(t)
	// The caller service matrix intentionally passes an owned synthetic query
	// canary and verifies its exact receipt by the provider. This isolated
	// test-binary worker explicitly opts in; production remains denied by default.
	originalConfig := iconfig.Config
	iconfig.Config = &iconfig.OpenAI{WebFetch: iconfig.WebFetchConfig{AllowQueryForwarding: true}}
	t.Cleanup(func() { iconfig.Config = originalConfig })
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket, Protocol: 2, DisableIdentity: true, MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, client.Ping(ctx).Err())

	alert, err := glog.NewAlert(ctx, os.Getenv("CRAWLER_SERVICE_TEST_ALERT_URL"),
		glog.WithAlertType("synthetic-service-path"), glog.WithAlertToken("synthetic-alert-only"))
	require.NoError(t, err)
	t.Cleanup(func() { alert.Close(); <-alert.Done() })
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(os.Stdout), zap.DebugLevel)
	logger, err := glog.NewWithName("go-ramjet", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }),
		zap.HooksWithFields(appLog.CrawlerDiagnosticAlertHook(alert.GetZapHook())))
	require.NoError(t, err)
	originalLogger := appLog.Logger
	appLog.Logger = logger
	t.Cleanup(func() { appLog.Logger = originalLogger })

	var hits atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("Cookie"))
		require.Empty(t, r.Header.Get("X-Api-Key"))
		w.Header().Set("Content-Type", "text/html")
		body := "<html><body><h1>service-body-canary</h1><p>Local fixture.</p></body></html>"
		switch r.URL.Path {
		case "/challenge":
			body = "<html><body><div id=\"cf-chl-widget\">service-body-canary</div></body></html>"
		case "/provider-challenge", "/provider-error", "/forged-provider-result":
			w.WriteHeader(http.StatusForbidden)
			body = "<html><body><div id=\"cf-chl-widget\">service-body-canary</div></body></html>"
		case "/quoted-challenge":
			body = "<html><body><h1>service-body-canary</h1><pre>cf-chl-widget _cf_chl_opt challenge-platform challenge-error-text</pre></body></html>"
		}
		_, writeErr := fmt.Fprint(w, body)
		require.NoError(t, writeErr)
	}))
	t.Cleanup(fixture.Close)
	deps := rendererTestDependencies(t, fixture, "93.184.216.34", &hits)
	var providerHits atomic.Int32
	var providerMu sync.Mutex
	providerRequests := map[string]int{}
	providerFixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "reader.example", r.Host)
		for _, name := range []string{"Authorization", "Cookie", "X-Api-Key", "Referer"} {
			require.Empty(t, r.Header.Get(name))
		}
		target, parseErr := url.Parse(strings.TrimPrefix(r.URL.RequestURI(), "/"))
		require.NoError(t, parseErr)
		require.Equal(t, "http", target.Scheme)
		require.Equal(t, "93.184.216.34", target.Host)
		require.Equal(t, "service-query-canary", target.Query().Get("private"))
		providerMu.Lock()
		providerRequests[target.Path]++
		providerMu.Unlock()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if target.Path == "/provider-error" {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, writeErr := fmt.Fprint(w, "service-provider-error-canary")
			require.NoError(t, writeErr)
			return
		}
		_, writeErr := fmt.Fprint(w, "# service-provider-canary\n\nOwned provider content.")
		require.NoError(t, writeErr)
	}))
	t.Cleanup(providerFixture.Close)
	providerCtx := withVerifiedWebFetchClient(ctx, rendererTestDependencies(t, providerFixture, "1.1.1.1", &providerHits))
	fetchVerifiedHTMLContent = func(renderCtx context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
		require.Empty(t, task.APIKey)
		parsed, parseErr := url.Parse(task.Url)
		require.NoError(t, parseErr)
		if parsed.Path == "/render-failure" {
			return "", nil, errors.New("navigation failed: https://private.invalid/?key=service-secret-canary")
		}
		if parsed.Path == "/provider-timeout" {
			require.NoError(t, renderCtx.Err(), "transport timeout must not cancel the worker")
			timeout := &url.Error{Op: "Get", URL: task.Url, Err: serviceHeaderTimeout{}}
			var typed net.Error
			require.ErrorAs(t, timeout, &typed)
			require.True(t, typed.Timeout())
			require.NotErrorIs(t, timeout, context.DeadlineExceeded)
			return "", nil, timeout
		}
		fetcher, fetchErr := newRendererEgressFetcher(renderCtx, task.TaskID, task.Url, *task.Egress, deps)
		if fetchErr != nil {
			return "", nil, fetchErr
		}
		_, body, fetchErr := fetcher.fetch(renderCtx, "document", "", task.Url, crawleregress.OriginDocument, http.MethodGet, nil)
		if fetchErr != nil {
			return "", nil, fetchErr
		}
		receipt, sealErr := fetcher.seal(renderCtx)
		if sealErr != nil {
			return "", nil, sealErr
		}
		if parsed.Path == "/missing-worker-receipt" {
			receipt = nil
		}
		return string(body), receipt, nil
	}
	persistVerifiedCrawlRecord = func(recordCtx context.Context, record *CrawlRecord) error {
		raw, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			return errors.Wrap(marshalErr, "encode owned crawl record")
		}
		return saveCrawlRecordPayload(recordCtx, client, record, raw)
	}
	publishHTMLCrawlerTask = func(publishCtx context.Context, task *rlibs.HTMLCrawlerTask) error {
		published := *task
		parsed, parseErr := url.Parse(task.Url)
		require.NoError(t, parseErr)
		// Tamper only after the real worker succeeded, to exercise the consumer's
		// verification boundary independently from worker-side verification.
		if task.Status == rlibs.TaskStatusSuccess {
			switch parsed.Path {
			case "/missing-result-receipt":
				published.EgressReceipt = nil
			case "/forged-result-peer":
				receipt := *task.EgressReceipt
				receipt.Origins = append([]crawleregress.Origin(nil), receipt.Origins...)
				receipt.Origins[0].PeerAddress = "127.0.0.1"
				published.EgressReceipt = &receipt
			case "/forged-provider-result":
				require.NotNil(t, task.Acquisition)
				require.Equal(t, crawleregress.AcquisitionProvider, task.Acquisition.Method)
				require.Nil(t, task.EgressReceipt)
				require.Empty(t, task.RequestChain)
				published.RequestChain = []string{task.Url}
			}
		}
		raw, encodeErr := encodeHTMLCrawlerTaskResult(publishCtx, &published)
		if encodeErr != nil {
			return encodeErr
		}
		if setErr := client.Set(publishCtx, crawlerResultKey(task.TaskID), raw, crawlerDataTTL).Err(); setErr != nil {
			return errors.Wrap(setErr, "publish owned service result")
		}
		if task.Status != rlibs.TaskStatusRunning {
			if setErr := client.Set(publishCtx, "service-path/case"+parsed.Path, task.TaskID, time.Minute).Err(); setErr != nil {
				return errors.Wrap(setErr, "publish owned case index")
			}
		}
		return nil
	}
	require.NoError(t, client.Set(ctx, "service-path/worker-ready", "ready", time.Minute).Err())
	for range 12 {
		task, popErr := popHTMLCrawlerTaskUsingClient(ctx, client)
		require.NoError(t, popErr)
		require.Empty(t, task.APIKey)
		require.Equal(t, []string{"93.184.216.34"}, task.Egress.Addresses)
		require.True(t, task.AllowProviderFallback, "actual MCP producer advertises the additive capability")
		parsed, parseErr := url.Parse(task.Url)
		require.NoError(t, parseErr)
		webFetchProxies = []webFetchProxy{}
		taskCtx := ctx
		if strings.HasPrefix(parsed.Path, "/provider-") || parsed.Path == "/forged-provider-result" {
			webFetchProxies = []webFetchProxy{prefixedWebFetchProxy{name: "service-reader", prefix: "http://reader.example/"}}
			taskCtx = providerCtx
		}
		require.NoError(t, processHTMLCrawlerTask(taskCtx, task))
		require.NoError(t, taskCtx.Err(), "all fixture acquisitions use a live worker context")
		if task.Status == rlibs.TaskStatusSuccess {
			for _, key := range []string{crawlerRecordKey(task.TaskID), crawlerLatestKey(task.Url)} {
				raw, readErr := client.Get(ctx, key).Bytes()
				require.NoError(t, readErr)
				var record CrawlRecord
				require.NoError(t, json.Unmarshal(raw, &record))
				require.Equal(t, task.TaskID, record.TaskID)
				require.Equal(t, task.Url, record.URL)
				require.Equal(t, task.Acquisition, record.Acquisition, "both persisted record indexes retain truthful provenance")
				require.Equal(t, task.ResultHTML, record.RawBody)
			}
		}
	}
	require.EqualValues(t, 10, hits.Load(), "render-failure and typed timeout never contact even the local direct fixture")
	require.EqualValues(t, 4, providerHits.Load(), "only explicit provider cases may contact the pinned provider fixture")
	providerMu.Lock()
	require.Equal(t, map[string]int{"/provider-challenge": 1, "/provider-timeout": 1, "/provider-error": 1, "/forged-provider-result": 1}, providerRequests)
	providerMu.Unlock()
	require.NoError(t, client.Set(ctx, "service-path/worker-done", "done", time.Minute).Err())
}
