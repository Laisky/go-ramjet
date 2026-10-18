package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"

	appLog "github.com/Laisky/go-ramjet/library/log"
)

// TestCrawlerLifecycleActualSerialization distinguishes processing from publication through JSON and the actual SDK HTTP hook.
func TestCrawlerLifecycleActualSerialization(t *testing.T) {
	for _, scenario := range []string{"running_publish_failure", "canceled", "deadline", "terminal_publish_failure", "cleanup_publish_failure", "processing_failure_published", "document_status_failure", "audit_failure", "success"} {
		t.Run(scenario, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			messages := make(chan string, 16)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				var envelope struct {
					Variables struct {
						Message string `json:"msg"`
					} `json:"variables"`
				}
				if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&envelope); err != nil {
					http.Error(w, "invalid fixture", 400)
					return
				}
				messages <- envelope.Variables.Message
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"data":{"TelegramMonitorAlert":{"name":"accepted"}}}`)
			}))
			t.Cleanup(server.Close)
			alertCtx, stopAlert := context.WithCancel(context.Background())
			t.Cleanup(stopAlert)
			alert, err := glog.NewAlert(alertCtx, server.URL, glog.WithAlertType("synthetic"), glog.WithAlertToken("synthetic-only"))
			require.NoError(t, err)
			t.Cleanup(func() {
				alert.Close()
				select {
				case <-alert.Done():
				case <-time.After(3 * time.Second):
					t.Error("alert did not stop")
				}
			})
			var encoded bytes.Buffer
			core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&encoded), zap.DebugLevel)
			logger, err := glog.NewWithName("go-ramjet", glog.LevelDebug,
				zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }),
				zap.HooksWithFields(appLog.CrawlerDiagnosticAlertHook(alert.GetZapHook())))
			require.NoError(t, err)
			original := appLog.Logger
			appLog.Logger = logger
			t.Cleanup(func() { appLog.Logger = original })
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			if scenario == "deadline" {
				deadlineCtx, stop := context.WithTimeout(ctx, 5*time.Millisecond)
				t.Cleanup(stop)
				ctx = deadlineCtx
			}
			task := verifiedTestTask()
			task.TaskID = "37b25fb8-4974-4a11-a5f5-b2f6c296025c"
			publishCalls := 0
			publicationFailure := &crawlerDiagnosticCanaryCause{}
			publishHTMLCrawlerTask = func(_ context.Context, result *rlibs.HTMLCrawlerTask) error {
				publishCalls++
				if scenario == "running_publish_failure" && publishCalls == 1 || scenario == "terminal_publish_failure" && publishCalls == 2 || scenario == "cleanup_publish_failure" && publishCalls == 2 {
					return publicationFailure
				}
				return nil
			}
			fetchVerifiedHTMLContent = func(renderCtx context.Context, current *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				switch scenario {
				case "canceled", "cleanup_publish_failure":
					cancel()
					return "", nil, errors.WithStack(context.Canceled)
				case "deadline":
					<-renderCtx.Done()
					return "", nil, errors.WithStack(renderCtx.Err())
				case "processing_failure_published":
					return "", nil, &crawlerDiagnosticCanaryCause{}
				case "document_status_failure":
					fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Encoding", "gzip")
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = w.Write([]byte("private-upstream-body-canary"))
					}))
					defer fixture.Close()
					var hits atomic.Int32
					fetcher, initErr := newRendererEgressFetcher(renderCtx, current.TaskID, "http://example.com/document", rendererTestPolicy("example.com", "93.184.216.34"), rendererTestDependencies(t, fixture, "93.184.216.34", &hits))
					require.NoError(t, initErr)
					_, _, fetchErr := fetcher.fetch(renderCtx, "document", "", "http://example.com/document", crawleregress.OriginDocument, http.MethodGet, nil)
					require.Error(t, fetchErr)
					return "", nil, fetchErr
				}
				return "<html><body><p>body-canary-private</p></body></html>", verifiedTestReceipt(current), nil
			}
			persistVerifiedCrawlRecord = func(context.Context, *CrawlRecord) error {
				if scenario == "audit_failure" {
					return &crawlerDiagnosticCanaryCause{}
				}
				return nil
			}
			processErr := processHTMLCrawlerTask(ctx, task)
			switch scenario {
			case "running_publish_failure", "terminal_publish_failure":
				require.ErrorIs(t, processErr, publicationFailure)
			case "canceled", "cleanup_publish_failure":
				require.ErrorIs(t, processErr, context.Canceled)
			case "deadline":
				require.ErrorIs(t, processErr, context.DeadlineExceeded)
			default:
				require.NoError(t, processErr)
			}
			var rows []map[string]any
			for _, line := range strings.Split(strings.TrimSpace(encoded.String()), "\n") {
				if line == "" {
					continue
				}
				var row map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &row))
				rows = append(rows, row)
			}
			eventRows := func(event string) []map[string]any {
				var matching []map[string]any
				for _, row := range rows {
					if row["event"] == event {
						matching = append(matching, row)
					}
				}
				return matching
			}
			expectedStatus, expectedContext := "success", "active"
			if scenario == "canceled" || scenario == "cleanup_publish_failure" {
				expectedStatus, expectedContext = "failed", "canceled"
			}
			if scenario == "deadline" {
				expectedStatus, expectedContext = "failed", "deadline_exceeded"
			}
			if scenario == "processing_failure_published" || scenario == "document_status_failure" {
				expectedStatus = "failed"
			}
			if scenario == "running_publish_failure" {
				require.Empty(t, eventRows("processing_completed"))
			} else {
				completed := eventRows("processing_completed")
				require.Len(t, completed, 1, "processing completion is independently observable")
				require.Equal(t, expectedStatus, completed[0]["result_status"])
				require.Equal(t, expectedContext, completed[0]["worker_context_state"])
				if scenario == "document_status_failure" {
					require.Equal(t, float64(503), completed[0]["upstream_status"])
				} else {
					require.NotContains(t, completed[0], "upstream_status")
				}
			}
			publicationFailed := scenario == "running_publish_failure" || scenario == "terminal_publish_failure" || scenario == "cleanup_publish_failure"
			if publicationFailed {
				failed := eventRows("result_publication_failed")
				require.Len(t, failed, 1)
				require.Equal(t, "failed", failed[0]["publication_outcome"])
				require.Empty(t, eventRows("result_published"), "a failed publication must not claim terminal publication success")
			} else {
				published := eventRows("result_published")
				require.Len(t, published, 1)
				require.Equal(t, "success", published[0]["publication_outcome"])
				require.Equal(t, expectedStatus, published[0]["result_status"])
			}
			if scenario == "audit_failure" {
				require.Len(t, eventRows("audit_persistence_failed"), 1)
			}
			for _, row := range rows {
				if row["event"] == nil {
					continue
				}
				require.Equal(t, task.TaskID, row["task_id"])
				duration, ok := row["duration_ms"].(float64)
				require.True(t, ok)
				require.GreaterOrEqual(t, duration, float64(0))
				require.LessOrEqual(t, duration, float64(300000))
				require.NotContains(t, row, "caller_receipt")
			}
			if scenario != "audit_failure" && scenario != "success" {
				select {
				case message := <-messages:
					require.Contains(t, message, task.TaskID)
					require.Contains(t, message, "\"event\":")
					require.NotContains(t, message, "{}")
					if scenario == "document_status_failure" {
						require.Contains(t, message, "\"upstream_status\":503")
					}
					require.NotContains(t, message, "private-upstream-body-canary")
					for _, canary := range []string{"private.invalid", "private-key-canary", "private-auth-canary", "private-query-canary", "private-body-canary", "token-canary", "synthetic-only"} {
						require.NotContains(t, message, canary, "unselected error/config/transport canaries remain outside selected inputs")
					}
					if scenario == "terminal_publish_failure" {
						require.Contains(t, message, "body-canary-private", "selected DOM and converter input remain visible when the SDK accepts this small record")
					}
				case <-time.After(3 * time.Second):
					t.Fatal("no actual SDK lifecycle alert")
				}
			}
			payloadRows := 0
			for _, row := range rows {
				if row["queued_target_url_observed"] != true {
					continue
				}
				payloadRows++
				require.Equal(t, task.Url, row["queued_target_url"], "failed lifecycle boundaries retain the exact queued input")
				require.Equal(t, "go-ramjet", row["source_service"])
				require.NotEmpty(t, row["source_hostname"])
				if scenario == "terminal_publish_failure" || scenario == "audit_failure" {
					require.Equal(t, "<html><body><p>body-canary-private</p></body></html>", row["rendered_html"])
					require.Equal(t, "<body><p>body-canary-private</p></body>", row["conversion_input"])
					require.Equal(t, true, row["rendered_html_observed"])
					require.Equal(t, true, row["conversion_input_observed"])
				} else {
					require.Equal(t, false, row["rendered_html_observed"])
					require.Equal(t, false, row["conversion_input_observed"])
				}
			}
			expectedPayloadRows := 1
			switch scenario {
			case "success":
				expectedPayloadRows = 0
			case "cleanup_publish_failure":
				expectedPayloadRows = 2
			}
			require.Equal(t, expectedPayloadRows, payloadRows, "every failure boundary emits its chosen inputs; success emits no failure payload")
			for _, canary := range []string{"private.invalid", "private-key-canary", "private-auth-canary", "private-query-canary", "private-body-canary", "private-upstream-body-canary", "token-canary", "synthetic-only"} {
				require.NotContains(t, encoded.String(), canary, "no arbitrary error/config/credential object is serialized as selected input")
			}
		})
	}
}
