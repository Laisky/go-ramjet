package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/crawlercapture"
)

// TestCrawlerFailureJournalWorkerRecords exercises the actual pinned converter and worker lifecycle.
func TestCrawlerFailureJournalWorkerRecords(t *testing.T) {
	preserveVerifiedWorkerSeams(t)
	directory := t.TempDir()
	require.NoError(t, os.Chmod(directory, 0700))
	t.Setenv("CRAWLER_FAILURE_LOG_DIR", directory)
	journal, err := crawlercapture.NewJournal(directory, crawlercapture.Source{Service: "go-ramjet", Hostname: "synthetic-worker", ProcessRunID: "synthetic-worker-run", Revision: "synthetic-source"})
	require.NoError(t, err)
	original := newCrawlerFailureJournal
	newCrawlerFailureJournal = func() (*crawlercapture.Journal, error) { return journal, nil }
	t.Cleanup(func() { newCrawlerFailureJournal = original })
	url := "https://1.1.1.1/Unicode-你好?literal=synthetic-input-canary#fragment"
	policy := crawleregress.Policy{Host: "1.1.1.1", Addresses: []string{"1.1.1.1"}, MaxRedirects: 5}
	task := rlibs.NewHTMLCrawlerTaskWithEgress(url, "SYNTHETIC_TASK_API_KEY_EXCLUDED", true, &policy)
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
	require.NotNil(t, task.FailedReason)
	require.Equal(t, "empty_markdown", *task.FailedReason)
	paths, err := filepath.Glob(filepath.Join(directory, "*.json"))
	require.NoError(t, err)
	require.Len(t, paths, 1, "exact original queued URL and actual converter input must be durably journalled")
	capture, err := crawlercapture.Read(paths[0])
	require.NoError(t, err)
	require.Equal(t, url, string(capture.Request.OriginalURL.Bytes()))
	require.Equal(t, url, string(capture.Request.SubmittedURL.Bytes()))
	require.True(t, capture.Request.OutputMarkdown)
	require.Equal(t, expected, string(capture.Converter.Input.Bytes()))
	require.Equal(t, page, string(capture.RenderedHTML.Bytes()))
	require.Equal(t, "conversion", capture.Failure.Stage)
	require.Equal(t, "empty_markdown", capture.Failure.Code)
	require.Equal(t, task.TaskID, capture.Correlation.TaskID)
	require.Equal(t, "synthetic-worker", capture.Source.Hostname)
	raw, err := os.ReadFile(paths[0])
	require.NoError(t, err)
	require.NotContains(t, string(raw), "SYNTHETIC_TASK_API_KEY_EXCLUDED")
	for _, blob := range []crawlercapture.Blob{capture.Request.OriginalURL, capture.Request.SubmittedURL, capture.Request.OriginalOutputMarkdown, capture.RenderedHTML, capture.Converter.Input} {
		require.NotContains(t, string(blob.Bytes()), "SYNTHETIC_TASK_API_KEY_EXCLUDED")
	}
	require.Contains(t, string(capture.Converter.Input.Bytes()), "synthetic-exact-script")
	if export := os.Getenv("CRAWLER_CAPTURE_TEST_FIXTURE_DIR"); export != "" {
		require.NoError(t, os.MkdirAll(export, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(export, "worker-empty-markdown.json"), raw, 0600))
	}
}

// TestCrawlerFailureJournalRealConverterControl confirms the empty result is not a fake converter.
func TestCrawlerFailureJournalRealConverterControl(t *testing.T) {
	output, err := convertVerifiedHTMLToMarkdown("<body><script>const x=1;</script></body>")
	require.NoError(t, err)
	require.Empty(t, strings.TrimSpace(output))
	output, err = convertVerifiedHTMLToMarkdown("<body><p>visible positive control</p></body>")
	require.NoError(t, err)
	require.Contains(t, output, "visible positive control")
}

// TestCrawlerFailureJournalConverterPin guards the fallback against public module drift.
func TestCrawlerFailureJournalConverterPin(t *testing.T) {
	data, err := os.ReadFile("../../../../go.mod")
	require.NoError(t, err)
	require.Contains(t, string(data), "github.com/JohannesKaufmann/html-to-markdown/v2 "+pinnedCrawlerConverterVersion)
	require.NotContains(t, string(data), "replace github.com/JohannesKaufmann/html-to-markdown/v2")
}

// TestCrawlerFailureJournalLifecycle covers failures on either side of the converter and an ordinary success.
func TestCrawlerFailureJournalLifecycle(t *testing.T) {
	for _, mode := range []string{"running_publication", "render", "canceled_conversion", "terminal_publication", "audit_persistence", "success"} {
		t.Run(mode, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			directory := t.TempDir()
			require.NoError(t, os.Chmod(directory, 0700))
			journal, err := crawlercapture.NewJournal(directory, crawlercapture.Source{Service: "go-ramjet", Hostname: "synthetic-worker", ProcessRunID: "synthetic-lifecycle-run"})
			require.NoError(t, err)
			original := newCrawlerFailureJournal
			newCrawlerFailureJournal = func() (*crawlercapture.Journal, error) { return journal, nil }
			t.Cleanup(func() { newCrawlerFailureJournal = original })
			task := verifiedTestTask()
			page := "<html><body><p>positive local conversion</p></body></html>"
			fetchVerifiedHTMLContent = func(ctx context.Context, current *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				if mode == "render" {
					return "", nil, errors.New("synthetic network read failure")
				}
				return page, verifiedTestReceipt(current), nil
			}
			calls := 0
			publishHTMLCrawlerTask = func(context.Context, *rlibs.HTMLCrawlerTask) error {
				calls++
				if mode == "running_publication" || mode == "terminal_publication" && calls == 2 {
					return errors.New("synthetic publication failure")
				}
				return nil
			}
			persistVerifiedCrawlRecord = func(context.Context, *CrawlRecord) error {
				if mode == "audit_persistence" {
					return errors.New("synthetic audit store failure")
				}
				return nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			convertVerifiedMarkdown = convertVerifiedHTMLToMarkdown
			if mode == "canceled_conversion" {
				convertVerifiedMarkdown = func(input string) (string, error) {
					output, e := convertVerifiedHTMLToMarkdown(input)
					cancel()
					return output, e
				}
			}
			processErr := processHTMLCrawlerTask(ctx, task)
			if mode == "running_publication" || mode == "terminal_publication" || mode == "canceled_conversion" {
				require.Error(t, processErr)
			} else {
				require.NoError(t, processErr)
			}
			paths, e := filepath.Glob(filepath.Join(directory, "*.json"))
			require.NoError(t, e)
			if mode == "success" {
				require.Empty(t, paths)
				return
			}
			require.Len(t, paths, 1, "each failure gets one capture even if cleanup/publication logs again")
			record, e := crawlercapture.Read(paths[0])
			require.NoError(t, e)
			require.Equal(t, task.TaskID, record.Correlation.TaskID)
			if mode == "running_publication" || mode == "render" {
				require.Equal(t, "not_reached", record.Converter.Input.State)
				require.Equal(t, "not_reached", record.RenderedHTML.State)
			} else {
				require.Equal(t, page, string(record.RenderedHTML.Bytes()))
				require.Equal(t, "<body><p>positive local conversion</p></body>", string(record.Converter.Input.Bytes()))
			}
		})
	}
}

// TestCrawlerFailureJournalActualAlertReference survives the receiver's first-3000-rune boundary.
func TestCrawlerFailureJournalActualAlertReference(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.Chmod(directory, 0700))
	for range 29 {
		directory = filepath.Join(directory, strings.Repeat("synthetic", 13))
	}
	journal, err := crawlercapture.NewJournal(directory, crawlercapture.Source{Service: "go-ramjet", Hostname: "synthetic-alert-worker", ProcessRunID: "synthetic-alert-run"})
	require.NoError(t, err)
	original := newCrawlerFailureJournal
	newCrawlerFailureJournal = func() (*crawlercapture.Journal, error) { return journal, nil }
	t.Cleanup(func() { newCrawlerFailureJournal = original })
	page := "<body><script>" + strings.Repeat("synthetic-private-body;", 1024) + "</script></body>"
	_, message := crawlerAlertContextFixture(t, page, nil, true)
	paths, err := filepath.Glob(filepath.Join(directory, "*.json"))
	require.NoError(t, err)
	require.Len(t, paths, 1)
	record, err := crawlercapture.Read(paths[0])
	require.NoError(t, err)
	raw, err := os.ReadFile(paths[0])
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	prefix := []rune(message)
	if len(prefix) > 3000 {
		prefix = prefix[:3000]
	}
	for _, value := range []string{"go-ramjet", "synthetic-alert-worker", "synthetic-alert-run", "complete", record.CaptureID, hex.EncodeToString(sum[:])} {
		require.Contains(t, string(prefix), value, "compact identity must precede the potentially long filesystem path")
	}
	require.NotContains(t, message, "synthetic-private-body")
	require.Contains(t, string(record.Converter.Input.Bytes()), strings.Repeat("synthetic-private-body;", 1024))
	require.Greater(t, len(directory), 3000, "real long configured path must exercise receiver truncation")
}

// TestCrawlerFailureJournalLargeWorkerInputs preserves observed and serialized sizes without imposing a capture cap.
func TestCrawlerFailureJournalLargeWorkerInputs(t *testing.T) {
	for _, mode := range []string{"serialized_body_expansion", "oversize_browser_document"} {
		t.Run(mode, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			directory := t.TempDir()
			require.NoError(t, os.Chmod(directory, 0700))
			journal, err := crawlercapture.NewJournal(directory, crawlercapture.Source{Service: "go-ramjet", Hostname: "synthetic-large-worker", ProcessRunID: "synthetic-large-run"})
			require.NoError(t, err)
			original := newCrawlerFailureJournal
			newCrawlerFailureJournal = func() (*crawlercapture.Journal, error) { return journal, nil }
			t.Cleanup(func() { newCrawlerFailureJournal = original })
			count := rendererMaxResponseBytes/5 + 128
			// Literal apostrophes are valid inside a double-quoted DOM attribute.
			// Go's real BODY serializer expands each to &#39; without changing the visible text.
			page := "<html><body data-fixture=\"" + strings.Repeat("'", count) + "\"><p>visible positive control</p></body></html>"
			expectedInput := "<body data-fixture=\"" + strings.Repeat("&#39;", count) + "\"><p>visible positive control</p></body>"
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
			conversions := 0
			passedInput := ""
			convertedOutput := ""
			convertVerifiedMarkdown = func(input string) (string, error) {
				conversions++
				passedInput = input
				output, err := convertVerifiedHTMLToMarkdown(input)
				convertedOutput = output
				return output, err
			}
			task := verifiedTestTask()
			require.NoError(t, processHTMLCrawlerTask(context.Background(), task))
			require.Equal(t, rlibs.TaskStatusFailed, task.Status)
			require.NotNil(t, task.FailedReason)
			require.Equal(t, "result_too_large", *task.FailedReason)
			paths, err := filepath.Glob(filepath.Join(directory, "*.json"))
			require.NoError(t, err)
			require.Len(t, paths, 1)
			record, err := crawlercapture.Read(paths[0])
			require.NoError(t, err)
			require.Equal(t, page, string(record.RenderedHTML.Bytes()))
			require.EqualValues(t, len(page), record.RenderedHTML.ByteLength)
			require.Equal(t, "result_too_large", record.Failure.Code)
			if mode == "oversize_browser_document" {
				require.Greater(t, len(page), rendererMaxResponseBytes)
				require.Equal(t, 0, conversions)
				require.Equal(t, "not_reached", record.Converter.Input.State)
				require.Equal(t, "render", record.Failure.Stage)
				return
			}
			require.Less(t, len(page), rendererMaxResponseBytes, "browser document is accepted before serialization expands it")
			require.Greater(t, len(expectedInput), rendererMaxResponseBytes)
			require.Equal(t, 1, conversions, "the actual pinned converter executes once, without another fetch")
			require.Equal(t, expectedInput, passedInput)
			require.Contains(t, convertedOutput, "visible positive control", "real converter produces nonempty output before the existing BODY-size rejection")
			require.Equal(t, expectedInput, string(record.Converter.Input.Bytes()))
			require.EqualValues(t, len(expectedInput), record.Converter.Input.ByteLength)
			sum := sha256.Sum256([]byte(expectedInput))
			require.Equal(t, hex.EncodeToString(sum[:]), record.Converter.Input.SHA256)
			require.Equal(t, "conversion", record.Failure.Stage)
		})
	}
}
