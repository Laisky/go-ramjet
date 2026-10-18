package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"

	appLog "github.com/Laisky/go-ramjet/library/log"
)

// crawlerAlertContextFixture runs the actual worker and SDK HTTP alert hook with
// local render/conversion seams, returning its final wire payload and alert text.
func crawlerAlertContextFixture(t *testing.T, content string, convert func(string) (string, error), markdown bool, renderer ...func(context.Context, *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error)) (string, string) {
	t.Helper()
	preserveVerifiedWorkerSeams(t)
	messages := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body struct {
			Variables struct {
				Message string `json:"msg"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
			http.Error(w, "invalid synthetic alert", http.StatusBadRequest)
			return
		}
		messages <- body.Variables.Message
		w.Header().Set("Content-Type", "application/json")
		_, err := fmt.Fprint(w, `{"data":{"TelegramMonitorAlert":{"name":"accepted"}}}`)
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	alert, err := glog.NewAlert(ctx, server.URL, glog.WithAlertType("synthetic"), glog.WithAlertToken("synthetic-only"))
	require.NoError(t, err)
	t.Cleanup(func() {
		alert.Close()
		select {
		case <-alert.Done():
		case <-time.After(3 * time.Second):
			t.Error("alert sender did not stop")
		}
	})
	var local bytes.Buffer
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&local), zapcore.DebugLevel)
	base, err := glog.NewWithName("go-ramjet", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }),
		zap.HooksWithFields(appLog.CrawlerDiagnosticAlertHook(alert.GetZapHook())))
	require.NoError(t, err)
	prior := appLog.Logger
	appLog.Logger = base
	t.Cleanup(func() { appLog.Logger = prior })
	fetchVerifiedHTMLContent = func(_ context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
		return content, verifiedTestReceipt(task), nil
	}
	if len(renderer) == 1 {
		fetchVerifiedHTMLContent = renderer[0]
	}
	if convert != nil {
		convertVerifiedMarkdown = convert
	}
	persistVerifiedCrawlRecord = func(context.Context, *CrawlRecord) error { return nil }
	task := verifiedTestTask()
	task.TaskID = "37b25fb8-4974-4a11-a5f5-b2f6c296025c"
	task.OutputMarkdown = markdown
	task.Egress.AllowSubresources = true
	var terminal string
	publications := 0
	publishHTMLCrawlerTask = func(ctx context.Context, current *rlibs.HTMLCrawlerTask) error {
		publications++
		if current.Status != rlibs.TaskStatusRunning {
			var err error
			terminal, err = encodeHTMLCrawlerTaskResult(ctx, current)
			require.NoError(t, err)
		}
		return nil
	}
	require.NoError(t, processHTMLCrawlerTask(ctx, task))
	require.Equal(t, 2, publications)
	decoded, err := rlibs.NewHTMLCrawlerTaskFromString(terminal)
	require.NoError(t, err)
	require.Equal(t, task.TaskID, decoded.TaskID)
	require.Equal(t, task.Url, decoded.Url, "existing caller binding is preserved")
	if !markdown {
		require.Equal(t, rlibs.TaskStatusSuccess, decoded.Status)
		select {
		case <-messages:
			t.Fatal("success produced an error alert")
		case <-time.After(30 * time.Millisecond):
		}
		return terminal, ""
	}
	require.Equal(t, rlibs.TaskStatusFailed, decoded.Status)
	require.NotNil(t, decoded.FailedReason)
	require.Equal(t, "empty_markdown", *decoded.FailedReason)
	var message string
	select {
	case message = <-messages:
	case <-time.After(3 * time.Second):
		t.Fatal("actual worker alert missing")
	}
	select {
	case <-messages:
		t.Fatal("one failure produced duplicate alerts")
	case <-time.After(30 * time.Millisecond):
	}
	require.NotContains(t, message, "private?canary=query")
	require.NotContains(t, message, "token-canary")
	require.NotContains(t, message, "synthetic-private-body")
	return terminal, message
}

// TestCrawlerAlertContextEmptyConversion distinguishes zero observed body text,
// whitespace-only output and a converter returning empty output for visible text.
func TestCrawlerAlertContextEmptyConversion(t *testing.T) {
	for _, tc := range []struct {
		name, html, output   string
		textRunes, textNodes int64
		actualConverter      bool
	}{
		{name: "empty body", html: "<html><body></body></html>", actualConverter: true},
		{name: "script only body", html: "<html><body><script>synthetic-private-body</script><style>p{color:red}</style></body></html>", actualConverter: true},
		{name: "visible text empty output", html: "<html><body><p>visible</p></body></html>", textRunes: 7, textNodes: 1},
		{name: "whitespace output", html: "<html><body><p>visible</p></body></html>", output: " \n\t", textRunes: 7, textNodes: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var convert func(string) (string, error)
			if !tc.actualConverter {
				convert = func(string) (string, error) { return tc.output, nil }
			}
			wire, message := crawlerAlertContextFixture(t, tc.html, convert, true)
			if tc.name == "visible text empty output" && os.Getenv("RAMJET_SYNTHETIC_CONTEXT_OUTPUT") != "" {
				output := os.Getenv("RAMJET_SYNTHETIC_CONTEXT_OUTPUT")
				require.NoError(t, os.MkdirAll(output, 0700))
				require.NoError(t, os.WriteFile(filepath.Join(output, "empty-markdown-terminal.json"), []byte(wire), 0600))
			}
			var envelope struct {
				FailureContext *struct {
					TargetScheme      string `json:"target_scheme"`
					TargetHostname    string `json:"target_hostname"`
					FinalScheme       string `json:"final_scheme"`
					UpstreamStatus    *int64 `json:"upstream_status"`
					RenderedHTMLBytes *int64 `json:"rendered_html_bytes"`
					InputBytes        *int64 `json:"conversion_input_bytes"`
					OutputBytes       *int64 `json:"conversion_output_bytes"`
					TrimmedBytes      *int64 `json:"conversion_output_trimmed_bytes"`
					TextRunes         *int64 `json:"body_text_runes"`
					TextNodes         *int64 `json:"body_text_nodes"`
					Converter         string `json:"converter"`
				} `json:"failure_context"`
			}
			require.NoError(t, json.Unmarshal([]byte(wire), &envelope))
			require.NotNil(t, envelope.FailureContext, "actual terminal envelope lacks observed failure context")
			evidence := envelope.FailureContext
			require.Equal(t, "https", evidence.TargetScheme)
			require.Equal(t, "example.com", evidence.TargetHostname)
			require.Empty(t, evidence.FinalScheme, "a stub receipt does not prove final response metadata")
			require.Nil(t, evidence.UpstreamStatus, "do not invent HTTP200")
			require.NotNil(t, evidence.RenderedHTMLBytes)
			require.EqualValues(t, len(tc.html), *evidence.RenderedHTMLBytes)
			require.NotNil(t, evidence.InputBytes)
			require.Greater(t, *evidence.InputBytes, int64(0))
			require.NotNil(t, evidence.OutputBytes)
			require.EqualValues(t, len(tc.output), *evidence.OutputBytes)
			require.NotNil(t, evidence.TrimmedBytes)
			require.Zero(t, *evidence.TrimmedBytes)
			require.NotNil(t, evidence.TextRunes)
			require.Equal(t, tc.textRunes, *evidence.TextRunes)
			require.NotNil(t, evidence.TextNodes)
			require.Equal(t, tc.textNodes, *evidence.TextNodes)
			require.Equal(t, "html-to-markdown/v2", evidence.Converter)
			require.Contains(t, message, "requested Markdown result contains no non-whitespace text")
			require.Contains(t, message, `"conversion_output_trimmed_bytes":0`)
			require.Contains(t, message, `"body_text_runes":`+fmt.Sprint(tc.textRunes))
			require.Contains(t, message, `"target_hostname":"example.com"`)
		})
	}
}

// TestCrawlerAlertContextHTMLOnlyPreservesBehavior never invokes the converter
// or emits failure context when callers explicitly request only rendered HTML.
func TestCrawlerAlertContextHTMLOnlyPreservesBehavior(t *testing.T) {
	calls := 0
	wire, _ := crawlerAlertContextFixture(t, "<html><body>unchanged</body></html>", func(string) (string, error) { calls++; return "", nil }, false)
	require.Zero(t, calls)
	require.NotContains(t, wire, `"failure_context"`)
	require.True(t, strings.Contains(wire, "result_html"))
}
