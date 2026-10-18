package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

// captureChallengeSignalWorker runs the actual worker processor with an owned
// renderer fixture and returns its local JSON, real SDK HTTP alert and queue wire.
func captureChallengeSignalWorker(t *testing.T, content string, renderErr error, failed bool) (string, string, string) {
	t.Helper()
	preserveVerifiedWorkerSeams(t)
	messages := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Variables struct {
				Message string `json:"msg"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
			http.Error(w, "invalid synthetic alert", http.StatusBadRequest)
			return
		}
		messages <- payload.Variables.Message
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `{"data":{"TelegramMonitorAlert":{"name":"accepted"}}}`); err != nil {
			t.Errorf("write owned alert response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	alert, err := glog.NewAlert(ctx, server.URL, glog.WithAlertType("synthetic"), glog.WithAlertToken("synthetic-only"))
	require.NoError(t, err)
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			alert.Close()
			select {
			case <-alert.Done():
			case <-time.After(3 * time.Second):
				t.Error("owned alert sender did not stop")
			}
		})
	}
	t.Cleanup(stop)
	var local bytes.Buffer
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&local), zap.DebugLevel)
	logger, err := glog.NewWithName("go-ramjet", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }),
		zap.HooksWithFields(appLog.CrawlerDiagnosticAlertHook(alert.GetZapHook())))
	require.NoError(t, err)
	original := appLog.Logger
	appLog.Logger = logger
	t.Cleanup(func() { appLog.Logger = original })
	fetchVerifiedHTMLContent = func(_ context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
		return content, verifiedTestReceipt(task), renderErr
	}
	persistVerifiedCrawlRecord = func(context.Context, *CrawlRecord) error { return nil }
	var wire string
	publishHTMLCrawlerTask = func(ctx context.Context, task *rlibs.HTMLCrawlerTask) error {
		encoded, err := encodeHTMLCrawlerTaskResult(ctx, task)
		if err != nil {
			return errors.Wrap(err, "encode owned worker result")
		}
		wire = encoded
		return nil
	}
	task := verifiedTestTask()
	task.TaskID = "37b25fb8-4974-4a11-a5f5-b2f6c296025c"
	task.OutputMarkdown = false
	require.NoError(t, processHTMLCrawlerTask(ctx, task))
	var message string
	if failed {
		require.Equal(t, rlibs.TaskStatusFailed, task.Status)
		select {
		case message = <-messages:
		case <-time.After(3 * time.Second):
			t.Fatal("missing actual worker SDK HTTP alert")
		}
	} else {
		require.Equal(t, rlibs.TaskStatusSuccess, task.Status)
		stop()
		require.Empty(t, messages, "successful worker must not emit a failure alert")
	}
	return local.String(), message, wire
}

// TestCrawlerChallengeSignalWorkerSerialization reproduces missing matched signals
// through actual worker JSON, SDK HTTP serialization and the legacy queue DTO.
func TestCrawlerChallengeSignalWorkerSerialization(t *testing.T) {
	for _, tc := range []struct{ name, content, signal string }{
		{"widget_id", `<div id="cf-chl-widget">private-body-canary</div>`, "widget_element"},
		{"widget_class", `<div class="other cf-chl-widget">private-body-canary</div>`, "widget_element"},
		{"error_id", `<div id="challenge-error-text">private-body-canary</div>`, "error_element"},
		{"error_class", `<div class="challenge-error-text">private-body-canary</div>`, "error_element"},
		{"platform_script", `<script src="https://private-resource.invalid/cdn-cgi/challenge-platform/h/g?api_key=private-key-canary"></script>`, "challenge_platform_script"},
		{"options_assignment", `<script>window._cf_chl_opt = {private: "private-body-canary"};</script>`, "challenge_options_assignment"},
		{"first_dom_match", `<div id="challenge-error-text"></div><div id="cf-chl-widget"></div>`, "error_element"},
		{"first_attribute_match", `<div id="cf-chl-widget" class="challenge-error-text"></div>`, "widget_element"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, isCloudflareChallenge(tc.content), "the retained classifier still rejects this fixture")
			local, remote, wire := captureChallengeSignalWorker(t, tc.content, nil, true)
			decoder := json.NewDecoder(strings.NewReader(local))
			var entries []map[string]any
			for {
				var entry map[string]any
				decodeErr := decoder.Decode(&entry)
				if errors.Is(decodeErr, io.EOF) {
					break
				}
				require.NoError(t, decodeErr, "each emitted worker event must be valid JSON")
				entries = append(entries, entry)
			}
			require.Len(t, entries, 2, "one processing failure precedes one successful terminal publication")
			const taskID = "37b25fb8-4974-4a11-a5f5-b2f6c296025c"
			for _, entry := range entries {
				require.Equal(t, taskID, entry["task_id"])
				require.Equal(t, "failed", entry["result_status"])
				require.Equal(t, "active", entry["worker_context_state"])
				duration, ok := entry["duration_ms"].(float64)
				require.True(t, ok, "each event must retain bounded elapsed duration")
				require.GreaterOrEqual(t, duration, float64(0))
				require.LessOrEqual(t, duration, float64(300000))
				require.NotContains(t, entry, "caller_receipt")
			}
			processing, publication := entries[0], entries[1]
			require.Equal(t, "processing_completed", processing["event"])
			require.Equal(t, "error", processing["level"])
			require.Equal(t, "render", processing["stage"])
			require.Equal(t, "challenge", processing["error_code"])
			require.Equal(t, "challenge", processing["category"])
			require.Equal(t, tc.signal, processing["challenge_signal"], "the actual worker must retain its finite matched signal")
			require.NotContains(t, processing, "publication_outcome", "processing completion does not establish publication")
			require.Equal(t, "result_published", publication["event"])
			require.Equal(t, "info", publication["level"])
			require.Equal(t, "success", publication["publication_outcome"])
			require.NotContains(t, publication, "challenge_signal", "publication is distinct from processing failure")
			require.Contains(t, remote, `"event":"processing_completed"`)
			require.Contains(t, remote, `"task_id":"`+taskID+`"`)
			require.Contains(t, remote, `"result_status":"failed"`)
			require.Contains(t, remote, `"challenge_signal":"`+tc.signal+`"`)
			require.NotContains(t, remote, `"event":"result_published"`, "the failure alert cannot claim caller receipt or publication")
			var envelope map[string]any
			require.NoError(t, json.Unmarshal([]byte(wire), &envelope))
			require.Equal(t, tc.signal, envelope["failure_signal"])
			require.Equal(t, "https://example.com/private?canary=query", processing["queued_target_url"])
			require.Equal(t, tc.content, processing["rendered_html"], "the selected challenge DOM is ordinary failure-log input, including credential-shaped fixture text")
			require.Equal(t, true, processing["rendered_html_observed"])
			require.Equal(t, false, processing["conversion_input_observed"], "challenge rejection precedes conversion")
			require.Equal(t, "go-ramjet", processing["source_service"])
			require.NotEmpty(t, processing["source_hostname"])
			for _, output := range []string{local, remote, wire} {
				require.NotContains(t, output, "token-canary", "task credentials are not selected diagnostic inputs")
				require.NotContains(t, output, "synthetic-only", "alert transport credentials never enter diagnostic fields")
			}
			// Queue compatibility remains scalar-only; adding log context must not copy page data into the DTO.
			for _, content := range []string{"private-body-canary", "private-resource.invalid", "private-key-canary", "_cf_chl_opt"} {
				require.NotContains(t, wire, content)
			}
			legacy, err := rlibs.NewHTMLCrawlerTaskFromString(wire)
			require.NoError(t, err)
			require.Equal(t, rlibs.TaskStatusFailed, legacy.Status)
			require.Equal(t, "challenge", *legacy.FailedReason)
			oldWire, err := legacy.ToString()
			require.NoError(t, err)
			require.NotContains(t, oldWire, "failure_signal", "legacy DTO must ignore the optional extension")
		})
	}
}

// TestCrawlerChallengeSignalMetadataBounds accepts only the finite signal on a
// failed render/challenge pair and excludes canary/unknown/nonchallenge metadata.
func TestCrawlerChallengeSignalMetadataBounds(t *testing.T) {
	for _, tc := range []struct {
		name, stage, category, signal, status string
		keep                                  bool
	}{
		{"valid", "render", "challenge", "widget_element", rlibs.TaskStatusFailed, true},
		{"unknown", "render", "challenge", "unknown", rlibs.TaskStatusFailed, false},
		{"canary", "render", "challenge", "api_key=private-signal-canary", rlibs.TaskStatusFailed, false},
		{"wrong_category", "render", "render_failed", "widget_element", rlibs.TaskStatusFailed, false},
		{"wrong_stage", "conversion", "conversion_failed", "widget_element", rlibs.TaskStatusFailed, false},
		{"success", "render", "challenge", "widget_element", rlibs.TaskStatusSuccess, false},
		{"running", "render", "challenge", "widget_element", rlibs.TaskStatusRunning, false},
		{"legacy_missing", "render", "challenge", "", rlibs.TaskStatusFailed, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// JSON decoding allows this regression to compile before the optional
			// Signal member is added to the production metadata type.
			raw, err := json.Marshal(map[string]string{"Stage": tc.stage, "Category": tc.category, "Signal": tc.signal})
			require.NoError(t, err)
			var metadata crawlerFailureMetadata
			require.NoError(t, json.Unmarshal(raw, &metadata))
			ctx := context.WithValue(t.Context(), crawlerFailureKey{}, metadata)
			task := verifiedTestTask()
			task.APIKey = ""
			task.Status = tc.status
			wire, err := encodeHTMLCrawlerTaskResult(ctx, task)
			require.NoError(t, err)
			var envelope map[string]any
			require.NoError(t, json.Unmarshal([]byte(wire), &envelope))
			if tc.keep {
				require.Equal(t, tc.signal, envelope["failure_signal"])
			} else {
				require.NotContains(t, envelope, "failure_signal")
			}
			require.NotContains(t, wire, "private-signal-canary")
		})
	}
}

// TestCrawlerChallengeSignalWorkerOmission excludes matched-signal fields from
// actual successful/nonchallenge processing while retaining ordinary cause text.
func TestCrawlerChallengeSignalWorkerOmission(t *testing.T) {
	local, remote, wire := captureChallengeSignalWorker(t, "", errors.New("navigation failed: connection refused"), true)
	for _, output := range []string{local, remote, wire} {
		require.NotContains(t, output, `"challenge_signal"`)
		require.NotContains(t, output, `"failure_signal"`)
	}
	require.Contains(t, remote, "connection refused")
	local, remote, wire = captureChallengeSignalWorker(t, `<html><body><p>Ordinary content.</p></body></html>`, nil, false)
	for _, output := range []string{local, remote, wire} {
		require.NotContains(t, output, `"challenge_signal"`)
		require.NotContains(t, output, `"failure_signal"`)
	}
}

// TestCrawlerChallengeSignalTypedBounds rejects invalid or wrongly scoped typed
// signals before error formatting, worker field extraction and wire publication.
func TestCrawlerChallengeSignalTypedBounds(t *testing.T) {
	for _, tc := range []struct {
		name, stage, code string
		signal            crawlerChallengeSignal
		keep              bool
	}{
		{"approved", "render", "challenge", crawlerChallengeWidgetElement, true},
		{"unknown", "render", "challenge", crawlerChallengeSignal("unknown"), false},
		{"canary", "render", "challenge", crawlerChallengeSignal("private-signal-canary https://private.invalid"), false},
		{"nonchallenge", "render", "render_failed", crawlerChallengeWidgetElement, false},
		{"wrong_stage", "conversion", "conversion_failed", crawlerChallengeWidgetElement, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := errors.WithStack(&crawlerTaskError{code: tc.code, stage: tc.stage, signal: tc.signal})
			if tc.keep {
				require.Equal(t, tc.signal, crawlerDiagnosticSignal(failure))
			} else {
				require.Empty(t, crawlerDiagnosticSignal(failure))
			}
			task := verifiedTestTask()
			task.APIKey = ""
			task.Status = rlibs.TaskStatusFailed
			wire, err := encodeHTMLCrawlerTaskResult(withCrawlerFailure(t.Context(), failure), task)
			require.NoError(t, err)
			var envelope map[string]any
			require.NoError(t, json.Unmarshal([]byte(wire), &envelope))
			if tc.keep {
				require.Equal(t, string(tc.signal), envelope["failure_signal"])
			} else {
				require.NotContains(t, envelope, "failure_signal")
			}
			for _, output := range []string{wire, failure.Error(), fmt.Sprintf("%+v", failure)} {
				require.NotContains(t, output, "private-signal-canary")
				require.NotContains(t, output, "private.invalid")
			}
		})
	}
}
