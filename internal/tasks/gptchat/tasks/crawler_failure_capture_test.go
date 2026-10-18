package tasks

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
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

// crawlerFieldsThroughJSONCore encodes ordinary Zap fields using the production JSON core.
func crawlerFieldsThroughJSONCore(t *testing.T, fields []zap.Field) map[string]any {
	t.Helper()
	var output bytes.Buffer
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&output), zapcore.DebugLevel)
	zap.New(core).Error("synthetic failure", fields...)
	var record map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	return record
}

// crawlerWorkerLogBuffer installs a real ordinary log core without remote services or payload files.
func crawlerWorkerLogBuffer(t *testing.T) *bytes.Buffer {
	t.Helper()
	output := new(bytes.Buffer)
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(output), zapcore.DebugLevel)
	logger, err := glog.NewWithName("go-ramjet", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	prior := appLog.Logger
	appLog.Logger = logger
	t.Cleanup(func() { appLog.Logger = prior })
	return output
}

// crawlerWorkerFailureRecords selects actual worker failure entries from the ordinary core.
func crawlerWorkerFailureRecords(t *testing.T, output *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal(line, &record))
		if record["queued_target_url_observed"] == true {
			records = append(records, record)
		}
	}
	return records
}

// TestCrawlerFailureLoggingWorkerInputs uses the real converter and actual worker log boundary.
func TestCrawlerFailureLoggingWorkerInputs(t *testing.T) {
	preserveVerifiedWorkerSeams(t)
	output := crawlerWorkerLogBuffer(t)
	target := "https://1.1.1.1/Unicode-你好?literal=synthetic-input-canary#fragment"
	policy := crawleregress.Policy{Host: "1.1.1.1", Addresses: []string{"1.1.1.1"}, MaxRedirects: 5}
	task := rlibs.NewHTMLCrawlerTaskWithEgress(target, "SYNTHETIC_TASK_API_KEY_EXCLUDED", true, &policy)
	task.TaskID = "0199c118-4229-7000-8000-000000000002"
	page := "<html><head><title>fixture</title></head><body><script>const canary='synthetic-exact-script';</script></body></html>"
	expected := "<body><script>const canary='synthetic-exact-script';</script></body>"
	fetchVerifiedHTMLContent = func(_ context.Context, current *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
		receipt := crawleregress.NewReceipt(current.TaskID, current.Url, *current.Egress)
		receipt.Origins = []crawleregress.Origin{{URL: current.Url, Addresses: []string{"1.1.1.1"}, PeerAddress: "1.1.1.1", Kind: crawleregress.OriginDocument}}
		return page, receipt, nil
	}
	publishHTMLCrawlerTask = func(context.Context, *rlibs.HTMLCrawlerTask) error { return nil }
	persistVerifiedCrawlRecord = func(context.Context, *CrawlRecord) error {
		t.Fatal("failed conversion must not persist success")
		return nil
	}
	convertVerifiedMarkdown = convertVerifiedHTMLToMarkdown
	require.NoError(t, processHTMLCrawlerTask(context.Background(), task))
	require.Equal(t, rlibs.TaskStatusFailed, task.Status)
	require.Equal(t, "empty_markdown", *task.FailedReason)
	records := crawlerWorkerFailureRecords(t, output)
	require.Len(t, records, 1)
	record := records[0]
	require.Equal(t, target, record["queued_target_url"])
	require.Equal(t, page, record["rendered_html"])
	require.Equal(t, expected, record["conversion_input"])
	require.Equal(t, true, record["rendered_html_observed"])
	require.Equal(t, true, record["conversion_input_observed"])
	require.Equal(t, true, record["output_markdown"])
	require.Equal(t, "conversion", record["stage"])
	require.Equal(t, "empty_markdown", record["error_code"])
	require.Equal(t, task.TaskID, record["task_id"])
	require.Equal(t, "go-ramjet", record["source_service"])
	require.NotEmpty(t, record["source_hostname"])
	require.NotEmpty(t, record["source_process_run_id"])
	require.NotContains(t, output.String(), "SYNTHETIC_TASK_API_KEY_EXCLUDED")
	for _, key := range []string{"failure_capture_path", "failure_capture_id", "failure_capture_status", "failure_capture_sha256"} {
		require.NotContains(t, record, key)
	}
}

// TestCrawlerFailureLoggingRealConverterControl confirms empty Markdown is not a fake converter result.
func TestCrawlerFailureLoggingRealConverterControl(t *testing.T) {
	result, err := convertVerifiedHTMLToMarkdown("<body><script>const x=1;</script></body>")
	require.NoError(t, err)
	require.Empty(t, strings.TrimSpace(result))
	result, err = convertVerifiedHTMLToMarkdown("<body><p>visible positive control</p></body>")
	require.NoError(t, err)
	require.Contains(t, result, "visible positive control")
}

// TestCrawlerFailureLoggingConverterPin guards the public fallback version against dependency drift.
func TestCrawlerFailureLoggingConverterPin(t *testing.T) {
	data, err := os.ReadFile("../../../../go.mod")
	require.NoError(t, err)
	require.Contains(t, string(data), "github.com/JohannesKaufmann/html-to-markdown/v2 "+pinnedCrawlerConverterVersion)
	require.NotContains(t, string(data), "replace github.com/JohannesKaufmann/html-to-markdown/v2")
}

// TestCrawlerChosenInputLoggingStates preserves empty, absent, Unicode, binary, and large chosen values.
func TestCrawlerChosenInputLoggingStates(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		observed    bool
	}{
		{"not_reached", "", false}, {"empty", "", true}, {"unicode", "雪 😀 e\u0301\r\n\x00", true},
		{"invalid_utf8", string([]byte{0xff, 0, 0xc0, 0xaf}), true}, {"large", strings.Repeat("雪", 12000), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := crawlerFieldsThroughJSONCore(t, crawlerChosenInputFields("conversion_input", tc.value, tc.observed))
			require.Equal(t, tc.observed, record["conversion_input_observed"])
			if !tc.observed {
				require.NotContains(t, record, "conversion_input")
				return
			}
			require.EqualValues(t, len(tc.value), record["conversion_input_bytes"])
			if record["conversion_input_encoding"] == "base64" {
				decoded, err := base64.StdEncoding.DecodeString(record["conversion_input"].(string))
				require.NoError(t, err)
				require.Equal(t, []byte(tc.value), decoded)
			} else {
				require.Equal(t, tc.value, record["conversion_input"])
			}
		})
	}
}

// TestCrawlerFailureLoggingLifecycle preserves original processing/publication errors and success behavior.
func TestCrawlerFailureLoggingLifecycle(t *testing.T) {
	for _, mode := range []string{"running_publication", "render", "canceled_conversion", "terminal_publication", "audit_persistence", "success"} {
		t.Run(mode, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			output := crawlerWorkerLogBuffer(t)
			task := verifiedTestTask()
			originalURL := task.Url
			page := "<html><body><p>positive local conversion</p></body></html>"
			cause := errors.New("synthetic original failure")
			fetchVerifiedHTMLContent = func(_ context.Context, current *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				if mode == "render" {
					return "", nil, cause
				}
				return page, verifiedTestReceipt(current), nil
			}
			calls := 0
			publishHTMLCrawlerTask = func(context.Context, *rlibs.HTMLCrawlerTask) error {
				calls++
				if mode == "running_publication" || mode == "terminal_publication" && calls == 2 {
					return cause
				}
				return nil
			}
			persistVerifiedCrawlRecord = func(context.Context, *CrawlRecord) error {
				if mode == "audit_persistence" {
					return cause
				}
				return nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			convertVerifiedMarkdown = convertVerifiedHTMLToMarkdown
			if mode == "canceled_conversion" {
				convertVerifiedMarkdown = func(input string) (string, error) {
					result, err := convertVerifiedHTMLToMarkdown(input)
					cancel()
					return result, err
				}
			}
			processErr := processHTMLCrawlerTask(ctx, task)
			if mode == "running_publication" || mode == "terminal_publication" {
				require.ErrorIs(t, processErr, cause)
			} else if mode == "canceled_conversion" {
				require.ErrorIs(t, processErr, context.Canceled)
			} else {
				require.NoError(t, processErr)
			}
			records := crawlerWorkerFailureRecords(t, output)
			if mode == "success" {
				require.Empty(t, records)
				require.Equal(t, rlibs.TaskStatusSuccess, task.Status)
				return
			}
			require.NotEmpty(t, records)
			record := records[0]
			require.Equal(t, originalURL, record["queued_target_url"])
			if mode == "running_publication" || mode == "render" {
				require.Equal(t, false, record["rendered_html_observed"])
				require.Equal(t, false, record["conversion_input_observed"])
			} else {
				require.Equal(t, page, record["rendered_html"])
				require.Equal(t, "<body><p>positive local conversion</p></body>", record["conversion_input"])
			}
			if mode != "canceled_conversion" {
				require.Contains(t, output.String(), "synthetic original failure")
			}
		})
	}
}

// TestCrawlerFailureLoggingLargeWorkerInputs keeps actual DOM and serialized converter arguments intact.
func TestCrawlerFailureLoggingLargeWorkerInputs(t *testing.T) {
	for _, mode := range []string{"serialized_body_expansion", "oversize_browser_document"} {
		t.Run(mode, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			output := crawlerWorkerLogBuffer(t)
			count := rendererMaxResponseBytes/5 + 128
			page := "<html><body data-fixture=\"" + strings.Repeat("'", count) + "\"><p>visible positive control</p></body></html>"
			expected := "<body data-fixture=\"" + strings.Repeat("&#39;", count) + "\"><p>visible positive control</p></body>"
			if mode == "oversize_browser_document" {
				page = "<html><body>" + strings.Repeat("x", rendererMaxResponseBytes) + "</body></html>"
			}
			fetchVerifiedHTMLContent = func(_ context.Context, current *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				return page, verifiedTestReceipt(current), nil
			}
			publishHTMLCrawlerTask = func(context.Context, *rlibs.HTMLCrawlerTask) error { return nil }
			persistVerifiedCrawlRecord = func(context.Context, *CrawlRecord) error {
				t.Fatal("oversized result must not persist success")
				return nil
			}
			convertVerifiedMarkdown = convertVerifiedHTMLToMarkdown
			task := verifiedTestTask()
			require.NoError(t, processHTMLCrawlerTask(context.Background(), task))
			require.Equal(t, rlibs.TaskStatusFailed, task.Status)
			require.Equal(t, "result_too_large", *task.FailedReason)
			records := crawlerWorkerFailureRecords(t, output)
			require.Len(t, records, 1)
			require.Equal(t, page, records[0]["rendered_html"])
			if mode == "oversize_browser_document" {
				require.Equal(t, false, records[0]["conversion_input_observed"])
			} else {
				require.Greater(t, len(expected), rendererMaxResponseBytes)
				require.Equal(t, expected, records[0]["conversion_input"])
			}
		})
	}
}

// TestCrawlerPinnedAlertStillChangesChosenURL records the existing SDK fidelity limitation, not a guarantee.
func TestCrawlerPinnedAlertStillChangesChosenURL(t *testing.T) {
	_, message := crawlerAlertContextFixture(t, "<html><body><script>synthetic source input</script></body></html>", nil, true)
	require.Contains(t, message, "source_service")
	require.NotContains(t, message, "https://example.com/private?canary=query", "pinned SDK sanitizes chosen URLs; full remote-alert fidelity remains unqualified")
}

// TestCrawlerPinnedAlertRejectsLargeChosenInputs exposes the real SDK transport cap without clipping inputs.
func TestCrawlerPinnedAlertRejectsLargeChosenInputs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	alert, err := glog.NewAlert(ctx, "http://127.0.0.1:1", glog.WithAlertType("synthetic"), glog.WithAlertToken("synthetic-only"))
	require.NoError(t, err)
	defer alert.Close()
	fields := crawlerChosenInputFields("rendered_html", strings.Repeat("original chosen body ", 2000), true)
	err = alert.GetZapHook()(zapcore.Entry{LoggerName: "go-ramjet.run_dynamic_web_crawler.verified_crawler_task", Level: zapcore.ErrorLevel, Time: time.Now().UTC(), Message: "synthetic failure"}, fields)
	require.ErrorIs(t, err, glog.ErrAlertMessageTooLarge)
}
