package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/log"
)

// embeddingRoundTripper routes test requests without contacting external services.
type embeddingRoundTripper func(*http.Request) (*http.Response, error)

// RoundTrip returns the response or error supplied by the test transport.
func (f embeddingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// embeddingCancelReader cancels the parent after returning a successful query body.
type embeddingCancelReader struct {
	io.Reader
	cancel context.CancelFunc
}

// Read cancels after a successful read while preserving the query response bytes.
func (r embeddingCancelReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.cancel()
	}
	return n, err
}

// Close releases the fake body without performing any work.
func (embeddingCancelReader) Close() error { return nil }

// embeddingQuotaHook records attempted quota commands without dialing Redis.
type embeddingQuotaHook struct{ calls atomic.Int32 }

// DialHook rejects any unexpected Redis connection attempt.
func (h *embeddingQuotaHook) DialHook(redis.DialHook) redis.DialHook {
	return func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("unexpected Redis dial")
	}
}

// ProcessHook counts quota commands and returns a local error without using the network.
func (h *embeddingQuotaHook) ProcessHook(redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, _ redis.Cmder) error {
		h.calls.Add(1)
		if err := ctx.Err(); err != nil {
			return errors.WithStack(err)
		}
		return errors.New("quota tripwire")
	}
}

// ProcessPipelineHook rejects unexpected pipeline commands without dialing Redis.
func (h *embeddingQuotaHook) ProcessPipelineHook(redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(context.Context, []redis.Cmder) error { return errors.New("unexpected Redis pipeline") }
}

// embeddingTestLogger returns a logger and observer for classification and privacy assertions.
func embeddingTestLogger(t *testing.T) (*glog.LoggerT, *observer.ObservedLogs) {
	t.Helper()
	core, entries := observer.New(zap.DebugLevel)
	logger, err := glog.NewWithName("embedding-test", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	return logger, entries
}

// embeddingTestSetup isolates mutable configuration and rejects default HTTP traffic.
func embeddingTestSetup(t *testing.T, server *httptest.Server) (*glog.LoggerT, *observer.ObservedLogs, *observer.ObservedLogs) {
	t.Helper()
	t.Setenv("DISABLE_LLM_CONSERVATION_AUDIT", "true")
	originalConfig, originalClient, originalLogger := config.Config, httpcli, log.Logger
	originalTransport := http.DefaultTransport
	originalQuotaManager := tokenQuotaMgr
	limiterInitialized := freeModelRateLimiter != nil || expensiveModelRateLimiter != nil
	t.Cleanup(func() {
		config.Config, httpcli, log.Logger = originalConfig, originalClient, originalLogger
		http.DefaultTransport = originalTransport
		tokenQuotaMgr = originalQuotaManager
		tokenQuotaOnce, onceLimiter = sync.Once{}, sync.Once{}
		if originalQuotaManager != nil {
			tokenQuotaOnce.Do(func() {})
		}
		if limiterInitialized {
			onceLimiter.Do(func() {})
		}
	})
	http.DefaultTransport = embeddingRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("external HTTP disabled in embedding regression")
	})
	onceLimiter = sync.Once{}
	onceLimiter.Do(func() {})
	config.Config = &config.OpenAI{RamjetURL: server.URL, API: server.URL, DefaultImageUrl: server.URL + "/images"}
	httpcli = server.Client()
	httpcli.Timeout = time.Second
	globalLogger, globalEntries := embeddingTestLogger(t)
	log.Logger = globalLogger
	requestLogger, requestEntries := embeddingTestLogger(t)
	return requestLogger.With(zap.String("trace_id", "embedding-test-trace")), requestEntries, globalEntries
}

// embeddingTestContext returns a real handler context with cached synthetic URL content.
func embeddingTestContext(t *testing.T, parent context.Context, logger glog.Logger, serverURL string, free bool, sources ...string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	urls := make([]string, len(sources))
	for i, source := range sources {
		url := "https://source.test/" + t.Name() + "/" + source + ".txt?signature=private-signature"
		urls[i] = url
		urlContentCache.Store(url, []byte(source))
		t.Cleanup(func() { urlContentCache.Delete(url) })
	}
	body := map[string]any{
		"model": "gpt-4.1", "stream": false, "max_tokens": 50,
		"messages":     []map[string]string{{"role": "user", "content": "private-prompt " + serverURL + " " + strings.Join(urls, " ")}},
		"laisky_extra": map[string]any{"chat_switch": map[string]bool{"disable_https_crawler": false}},
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", strings.NewReader(string(raw))).WithContext(parent)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(ctxKeyUser, &config.UserConfig{
		UserName: "embedding-test-user", Token: "synthetic-auth", OpenaiToken: "synthetic-auth",
		APIBase: serverURL, AllowedModels: []string{"*"}, IsFree: free, BYOK: true,
	})
	gmw.SetLogger(ctx, logger)
	return ctx, recorder
}

// TestURLEmbeddingParentEndsBeforeQuota proves real preparation stops and classifies parent termination once.
func TestURLEmbeddingParentEndsBeforeQuota(t *testing.T) {
	for _, name := range []string{"cancel", "deadline", "success_then_cancel", "already_cancelled"} {
		t.Run(name, func(t *testing.T) {
			deadline := name == "deadline"
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			var upstreamCalls, queryCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/gptchat/query/chunks" {
					queryCalls.Add(1)
					_, _ = io.Copy(io.Discard, r.Body)
					if !deadline {
						cancel()
					}
					select {
					case <-r.Context().Done():
					case <-time.After(time.Second):
					}
					return
				}
				upstreamCalls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(server.Close)
			logger, requestLogs, globalLogs := embeddingTestSetup(t, server)
			if deadline {
				cancel()
				parent, cancel = context.WithTimeout(context.Background(), 150*time.Millisecond)
				defer cancel()
			}
			if name == "success_then_cancel" {
				httpcli.Transport = embeddingRoundTripper(func(r *http.Request) (*http.Response, error) {
					queryCalls.Add(1)
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: r,
						Body: embeddingCancelReader{Reader: strings.NewReader(`{"results":"successful-child"}`), cancel: cancel}}, nil
				})
			}
			if name == "already_cancelled" {
				cancel()
			}
			quotaHook := &embeddingQuotaHook{}
			quotaClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
			quotaClient.AddHook(quotaHook)
			t.Cleanup(func() { require.NoError(t, quotaClient.Close()) })
			tokenQuotaOnce = sync.Once{}
			tokenQuotaOnce.Do(func() {})
			tokenQuotaMgr = &TokenQuotaManager{client: quotaClient, logger: logger, windowMinutes: 10}
			ctx, recorder := embeddingTestContext(t, parent, logger, server.URL, true, "cancel")
			err := sendChatWithResponsesToolLoop(ctx)
			wantErr, wantStatus, wantMessage := context.Canceled, 499, "request canceled"
			if deadline {
				wantErr, wantStatus, wantMessage = context.DeadlineExceeded, http.StatusGatewayTimeout, "request timeout"
			}
			t.Logf("quota commands=%d, global ERRORs=%d, request status=%d", quotaHook.calls.Load(), globalLogs.FilterLevelExact(zap.ErrorLevel).Len(), recorder.Code)
			if name != "already_cancelled" {
				require.Equal(t, int32(1), queryCalls.Load(), "must exercise termination during actual query work")
			} else {
				require.Zero(t, queryCalls.Load(), "already-ended parent must not start a query")
			}
			require.ErrorIs(t, err, wantErr)
			require.Zero(t, quotaHook.calls.Load(), "parent termination must stop before quota/DB preparation")
			require.Zero(t, upstreamCalls.Load())
			require.Equal(t, wantStatus, recorder.Code)
			require.Zero(t, globalLogs.FilterLevelExact(zap.ErrorLevel).Len())
			require.Zero(t, requestLogs.FilterLevelExact(zap.ErrorLevel).Len())
			classified := requestLogs.FilterMessage(wantMessage).All()
			require.Len(t, classified, 1)
			wantLevel := zap.DebugLevel
			if deadline {
				wantLevel = zap.WarnLevel
			}
			require.Equal(t, wantLevel, classified[0].Level)
			require.Equal(t, "embedding-test-trace", classified[0].ContextMap()["trace_id"])
		})
	}
}

// TestURLEmbeddingFailuresRemainBestEffort proves live-parent failures remain visible and preserve sibling results.
func TestURLEmbeddingFailuresRemainBestEffort(t *testing.T) {
	for _, mode := range []string{"child_deadline", "upstream_failure", "independent_cancel", "partial_success", "fetch_failure"} {
		t.Run(mode, func(t *testing.T) {
			var upstreamCalls, chunkCalls atomic.Int32
			var upstreamBody atomic.Value
			failed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/gptchat/query/chunks" {
					upstreamCalls.Add(1)
					body, _ := io.ReadAll(r.Body)
					upstreamBody.Store(string(body))
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"id":"embedding-regression","output_text":"ok","output":[]}`)
					return
				}
				chunkCalls.Add(1)
				if mode == "child_deadline" {
					_, _ = io.Copy(io.Discard, r.Body)
					select {
					case <-r.Context().Done():
					case <-time.After(time.Second):
					}
					return
				}
				var body struct {
					Content string `json:"content"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				content, _ := base64.StdEncoding.DecodeString(body.Content)
				if string(content) == "success" {
					select {
					case <-failed:
					case <-r.Context().Done():
						return
					case <-time.After(time.Second):
						w.WriteHeader(http.StatusGatewayTimeout)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"results":"retained-sibling-content"}`)
					return
				}
				close(failed)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			t.Cleanup(server.Close)
			logger, requestLogs, globalLogs := embeddingTestSetup(t, server)
			baseTransport := httpcli.Transport
			httpcli.Transport = embeddingRoundTripper(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "source.test" {
					return nil, errors.New("synthetic fetch failure")
				}
				if r.URL.Path == "/gptchat/query/chunks" && mode == "independent_cancel" {
					return nil, errors.WithStack(context.Canceled)
				}
				if r.URL.Path == "/gptchat/query/chunks" && mode == "child_deadline" {
					child, cancel := context.WithTimeout(r.Context(), 15*time.Millisecond)
					defer cancel()
					return baseTransport.RoundTrip(r.Clone(child))
				}
				return baseTransport.RoundTrip(r)
			})
			sources := []string{"failure"}
			if mode == "partial_success" {
				sources = append(sources, "success")
			}
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx, recorder := embeddingTestContext(t, parent, logger, server.URL, false, sources...)
			if mode == "fetch_failure" {
				urlContentCache.Delete("https://source.test/" + t.Name() + "/failure.txt?signature=private-signature")
			}
			require.NoError(t, sendChatWithResponsesToolLoop(ctx))
			require.NoError(t, parent.Err())
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, int32(1), upstreamCalls.Load())
			require.Contains(t, upstreamBody.Load().(string), "some url content is not available")
			if mode == "partial_success" {
				require.Equal(t, int32(2), chunkCalls.Load())
				require.Contains(t, upstreamBody.Load().(string), "retained-sibling-content")
			}
			t.Logf("chunk requests=%d, upstream requests=%d, global enrichment ERRORs=%d", chunkCalls.Load(), upstreamCalls.Load(), globalLogs.FilterMessage("query mentioned urls").Len())
			failures := requestLogs.FilterMessage("query mentioned urls").All()
			require.Len(t, failures, 1, "real enrichment failures must remain visible with request trace")
			require.Equal(t, zap.ErrorLevel, failures[0].Level)
			require.Equal(t, "embedding-test-trace", failures[0].ContextMap()["trace_id"])
			require.Zero(t, globalLogs.FilterMessage("query mentioned urls").Len())
			require.Zero(t, requestLogs.FilterMessage("request canceled").Len())
			fields := fmt.Sprint(failures[0].ContextMap())
			require.NotContains(t, fields, "private-prompt")
			require.NotContains(t, fields, "private-signature")
			require.NotContains(t, fields, "synthetic-auth")
		})
	}
}
