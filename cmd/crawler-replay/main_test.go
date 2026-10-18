package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-ramjet/library/crawlercapture"
	"github.com/stretchr/testify/require"
)

type replayPair struct {
	caller, worker         crawlercapture.Record
	callerPath, workerPath string
}

// privateReplayTempDir sets exact private permissions only on this test's own root.
func privateReplayTempDir(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.Chmod(directory, 0700))
	return directory
}

// writeReplayPair creates two real sealed private captures without task or application imports.
func writeReplayPair(t *testing.T, change func(*crawlercapture.Record, *crawlercapture.Record)) replayPair {
	t.Helper()
	const submitted = "https://example.invalid/submitted?q=雪&token=preserve-value#fragment"
	caller := crawlercapture.Record{
		Source:      crawlercapture.Source{Service: "synthetic-graphql", Hostname: "synthetic-host", ProcessRunID: "synthetic-caller-run"},
		Correlation: crawlercapture.Correlation{TaskID: "synthetic-task", RequestID: "synthetic-request"},
		Failure:     crawlercapture.Failure{Stage: "result", Code: "worker_failed", WorkerStage: "conversion", WorkerCode: "empty_markdown"},
		Request: crawlercapture.Request{
			OriginalURL:  crawlercapture.CapturedString(" " + submitted + " "),
			SubmittedURL: crawlercapture.CapturedString(submitted), OutputMarkdown: true,
		},
	}
	worker := crawlercapture.Record{
		Source:      crawlercapture.Source{Service: "go-ramjet", Hostname: "synthetic-worker-host", ProcessRunID: "synthetic-worker-run"},
		Correlation: crawlercapture.Correlation{TaskID: "synthetic-task"},
		Failure:     crawlercapture.Failure{Stage: "conversion", Code: "empty_markdown"},
		Request: crawlercapture.Request{
			OriginalURL:  crawlercapture.CapturedString(submitted),
			SubmittedURL: crawlercapture.CapturedString(submitted), OutputMarkdown: true,
		},
		Converter: crawlercapture.Converter{Name: converterName, Version: sourcePinnedConverterVersion,
			Input: crawlercapture.CapturedString("<body><script>synthetic-content-no-default-dump</script></body>")},
		RenderedHTML: crawlercapture.CapturedString("<html><body><script>synthetic-content-no-default-dump</script></body></html>"),
	}
	if change != nil {
		change(&caller, &worker)
	}
	directory := privateReplayTempDir(t)
	write := func(name string, record crawlercapture.Record) (string, crawlercapture.Record) {
		journal, err := crawlercapture.NewJournal(filepath.Join(directory, name), record.Source)
		require.NoError(t, err)
		reference, err := journal.Write(record)
		require.NoError(t, err)
		require.Equal(t, "complete", reference.Status)
		sealed, err := crawlercapture.Read(reference.Path)
		require.NoError(t, err)
		return reference.Path, sealed
	}
	callerPath, sealedCaller := write("caller", caller)
	workerPath, sealedWorker := write("worker", worker)
	return replayPair{caller: sealedCaller, worker: sealedWorker, callerPath: callerPath, workerPath: workerPath}
}

// TestReplayConverterSourcePin guards the fallback when dependency metadata is absent from test binaries.
func TestReplayConverterSourcePin(t *testing.T) {
	module, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	require.NoError(t, err)
	found, replacementBlock := false, false
	for _, line := range strings.Split(string(module), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "replace" {
			if len(fields) > 1 && fields[1] == "(" {
				replacementBlock = true
			} else if len(fields) > 1 {
				require.NotEqual(t, converterModule, fields[1], "a replacement invalidates the fixed fallback")
			}
			continue
		}
		if fields[0] == ")" {
			replacementBlock = false
			continue
		}
		if replacementBlock {
			require.NotEqual(t, converterModule, fields[0], "a replacement invalidates the fixed fallback")
			continue
		}
		if len(fields) >= 2 && fields[0] == converterModule {
			require.Equal(t, sourcePinnedConverterVersion, fields[1])
			found = true
		}
	}
	require.True(t, found, "the fallback must remain bound to the explicit converter dependency")
}

// TestReplayScriptOnlyInput invokes the actual converter and retains exact caller URL bytes.
func TestReplayScriptOnlyInput(t *testing.T) {
	pair := writeReplayPair(t, nil)
	var output, diagnostics bytes.Buffer
	require.NoError(t, run([]string{"--caller", pair.callerPath, "--worker", pair.workerPath}, &output, &diagnostics))
	var report replayReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	require.Equal(t, "synthetic-task", report.TaskID)
	require.Equal(t, pair.caller.Source, report.CallerSource)
	require.Equal(t, pair.worker.Source, report.WorkerSource)
	require.True(t, report.OriginalURLUTF8Valid)
	require.NotNil(t, report.OriginalURL)
	require.Equal(t, string(pair.caller.Request.OriginalURL.Bytes()), *report.OriginalURL)
	require.Equal(t, base64.StdEncoding.EncodeToString(pair.caller.Request.OriginalURL.Bytes()), report.OriginalURLBase64)
	require.Equal(t, "matched", report.VersionQualification)
	require.Equal(t, "match", report.ReproductionStatus)
	require.Equal(t, "empty_markdown", report.ReplayClassification)
	require.NotNil(t, report.TrimmedOutputBytes)
	require.Zero(t, *report.TrimmedOutputBytes)
	require.Equal(t, digest(nil), report.TrimmedOutputSHA256)
	require.Equal(t, pair.worker.Converter.Input.SHA256, report.InputSHA256)
	require.NotContains(t, output.String(), "synthetic-content-no-default-dump")
	require.Empty(t, diagnostics.String())
}

// TestReplayPairMismatches rejects ambiguous or contradictory pairing before conversion.
func TestReplayPairMismatches(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*crawlercapture.Record, *crawlercapture.Record)
	}{
		{name: "task identity", change: func(_, worker *crawlercapture.Record) { worker.Correlation.TaskID = "other-task" }},
		{name: "missing task identity", change: func(caller, worker *crawlercapture.Record) {
			caller.Correlation.TaskID = ""
			worker.Correlation.TaskID = ""
		}},
		{name: "original worker URL", change: func(_, worker *crawlercapture.Record) {
			worker.Request.OriginalURL = crawlercapture.CapturedString("https://other.invalid/")
		}},
		{name: "submitted worker URL", change: func(_, worker *crawlercapture.Record) {
			worker.Request.SubmittedURL = crawlercapture.CapturedString("https://other.invalid/")
		}},
		{name: "missing caller submitted URL", change: func(caller, _ *crawlercapture.Record) { caller.Request.SubmittedURL = crawlercapture.NotReached() }},
		{name: "contradictory original caller URL", change: func(caller, _ *crawlercapture.Record) {
			caller.Request.OriginalURL = crawlercapture.CapturedString("https://other.invalid/original")
		}},
		{name: "missing original caller URL", change: func(caller, _ *crawlercapture.Record) { caller.Request.OriginalURL = crawlercapture.NotReached() }},
		{name: "output option", change: func(_, worker *crawlercapture.Record) { worker.Request.OutputMarkdown = false }},
		{name: "worker stage", change: func(caller, _ *crawlercapture.Record) { caller.Failure.WorkerStage = "render" }},
		{name: "worker code", change: func(caller, _ *crawlercapture.Record) { caller.Failure.WorkerCode = "render_failed" }},
		{name: "worker source", change: func(_, worker *crawlercapture.Record) { worker.Source.Service = "other-worker" }},
		{name: "same source service", change: func(caller, _ *crawlercapture.Record) { caller.Source.Service = "go-ramjet" }},
		{name: "converter name", change: func(_, worker *crawlercapture.Record) { worker.Converter.Name = "another-converter" }},
		{name: "known converter version", change: func(_, worker *crawlercapture.Record) { worker.Converter.Version = "v9.9.9" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pair := writeReplayPair(t, tc.change)
			_, err := replay(pair.caller, pair.worker, converterBuild{Version: sourcePinnedConverterVersion, Source: "synthetic-build"})
			require.Error(t, err)
		})
	}
}

// TestReplayUnavailableVersion reports incomplete version evidence without claiming a match.
func TestReplayUnavailableVersion(t *testing.T) {
	for _, tc := range []struct{ name, captured, local string }{
		{name: "captured unavailable", captured: "unavailable", local: sourcePinnedConverterVersion},
		{name: "captured empty", local: sourcePinnedConverterVersion},
		{name: "replay unavailable", captured: sourcePinnedConverterVersion, local: "(devel)"},
		{name: "both unavailable", captured: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pair := writeReplayPair(t, func(_, worker *crawlercapture.Record) { worker.Converter.Version = tc.captured })
			report, err := replay(pair.caller, pair.worker, converterBuild{Version: tc.local, Source: "synthetic-build"})
			require.NoError(t, err)
			require.Equal(t, "unqualified_version_unavailable", report.VersionQualification)
			require.Equal(t, "unqualified", report.ReproductionStatus)
			require.Equal(t, "empty_markdown", report.ReplayClassification)
		})
	}
}

// TestReplayNotReached does not invent converter output when no invocation was captured.
func TestReplayNotReached(t *testing.T) {
	pair := writeReplayPair(t, func(caller, worker *crawlercapture.Record) {
		worker.Converter.Input = crawlercapture.NotReached()
		worker.Failure.Stage, worker.Failure.Code = "render", "render_failed"
		caller.Failure.WorkerStage, caller.Failure.WorkerCode = "render", "render_failed"
	})
	report, err := replay(pair.caller, pair.worker, converterBuild{Version: sourcePinnedConverterVersion})
	require.NoError(t, err)
	require.Equal(t, "not_reached", report.ReplayClassification)
	require.Equal(t, "not_applicable", report.ReproductionStatus)
	require.Nil(t, report.OutputBytes)
	require.Nil(t, report.TrimmedOutputBytes)
	require.Empty(t, report.OutputSHA256)
}

// TestReplayOriginalURLBase64 preserves exact bytes even when JSON strings require UTF-8 repair.
func TestReplayOriginalURLBase64(t *testing.T) {
	original := []byte{'h', 't', 't', 'p', 's', ':', '/', '/', 0xff, 0, '?', 'x', '=', 0xfe}
	pair := writeReplayPair(t, func(caller, worker *crawlercapture.Record) {
		caller.Request.OriginalURL = crawlercapture.Captured(original)
		submitted := bytes.TrimSpace(original)
		caller.Request.SubmittedURL = crawlercapture.Captured(submitted)
		worker.Request.OriginalURL = crawlercapture.Captured(submitted)
		worker.Request.SubmittedURL = crawlercapture.Captured(submitted)
	})
	var output, diagnostics bytes.Buffer
	require.NoError(t, run([]string{"--caller", pair.callerPath, "--worker", pair.workerPath}, &output, &diagnostics))
	var report replayReport
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	reconstructed, err := base64.StdEncoding.DecodeString(report.OriginalURLBase64)
	require.NoError(t, err)
	require.Equal(t, original, reconstructed)
	require.Equal(t, digest(original), report.OriginalURLSHA256)
	require.False(t, report.OriginalURLUTF8Valid)
	require.Nil(t, report.OriginalURL)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(output.Bytes(), &fields))
	_, humanURLPresent := fields["original_url"]
	require.False(t, humanURLPresent, "invalid UTF-8 must not be presented as a repaired original string")
}

// TestReplayCorruptCapture rejects payload corruption instead of converting untrusted changed bytes.
func TestReplayCorruptCapture(t *testing.T) {
	pair := writeReplayPair(t, nil)
	raw, err := os.ReadFile(pair.workerPath)
	require.NoError(t, err)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(raw, &envelope))
	converter := envelope["converter"].(map[string]any)
	input := converter["input"].(map[string]any)
	input["base64"] = base64.StdEncoding.EncodeToString([]byte("changed without resealing"))
	raw, err = json.Marshal(envelope)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(pair.workerPath, raw, 0600))
	var output, diagnostics bytes.Buffer
	require.Error(t, run([]string{"--caller", pair.callerPath, "--worker", pair.workerPath}, &output, &diagnostics))
	require.Empty(t, output.String())
}

// TestReplayArguments requires explicit capture paths and accepts no extra input sources.
func TestReplayArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"--caller", "unused"}, {"--worker", "unused"}, {"--caller", "a", "--worker", "b", "extra"}, {"--url", "https://example.invalid/"}} {
		var output, diagnostics bytes.Buffer
		require.Error(t, run(args, &output, &diagnostics))
		require.Empty(t, output.String())
	}
}

// TestReplayStandalone builds and executes the actual isolated command against real sealed captures.
func TestReplayStandalone(t *testing.T) {
	pair := writeReplayPair(t, nil)
	binary := filepath.Join(privateReplayTempDir(t), "crawler-replay")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	build := exec.CommandContext(t.Context(), goBinary, "build", "-o", binary, ".")
	built, err := build.CombinedOutput()
	require.NoError(t, err, "build standalone replay command: %s", built)
	command := exec.CommandContext(t.Context(), binary, "--caller", pair.callerPath, "--worker", pair.workerPath)
	raw, err := command.Output()
	require.NoError(t, err)
	var report replayReport
	require.NoError(t, json.Unmarshal(raw, &report))
	require.Equal(t, "empty_markdown", report.ReplayClassification)
	require.Equal(t, "matched", report.VersionQualification)
	require.Equal(t, "match", report.ReproductionStatus)
	require.Equal(t, sourcePinnedConverterVersion, report.ReplayConverterVersion)
	require.Equal(t, "build_metadata", report.ReplayConverterVersionSource)
	require.Equal(t, pair.caller.Correlation.TaskID, report.TaskID)
	require.Equal(t, base64.StdEncoding.EncodeToString(pair.caller.Request.OriginalURL.Bytes()), report.OriginalURLBase64)
	require.NotContains(t, string(raw), "synthetic-content-no-default-dump")
}

// TestReplayClassificationCorrespondence compares observed categories without asserting an upstream cause.
func TestReplayClassificationCorrespondence(t *testing.T) {
	for _, tc := range []struct{ name, stage, code, expected string }{
		{name: "different conversion classification", stage: "conversion", code: "conversion_failed", expected: "mismatch"},
		{name: "publication failure", stage: "persistence", code: "publication_failed", expected: "not_applicable"},
		{name: "render failure", stage: "render", code: "render_failed", expected: "not_applicable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pair := writeReplayPair(t, func(caller, worker *crawlercapture.Record) {
				worker.Failure.Stage, worker.Failure.Code = tc.stage, tc.code
				caller.Failure.WorkerStage, caller.Failure.WorkerCode = tc.stage, tc.code
			})
			report, err := replay(pair.caller, pair.worker, converterBuild{Version: sourcePinnedConverterVersion})
			require.NoError(t, err)
			require.Equal(t, "empty_markdown", report.ReplayClassification)
			require.Equal(t, tc.expected, report.ReproductionStatus)
		})
	}
}

// TestReplayErrorBytes preserves converter error text independently of JSON UTF-8 requirements.
func TestReplayErrorBytes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		text  string
		valid bool
	}{
		{name: "Unicode actual message", text: "synthetic converter error: 雪 token=unchanged", valid: true},
		{name: "invalid message bytes", text: string([]byte{'e', ':', 0xff, 0, 0xfe}), valid: false},
		{name: "empty actual message", valid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var report replayReport
			recordReplayError(&report, errors.New(tc.text))
			require.NotNil(t, report.ConverterErrorUTF8Valid)
			require.Equal(t, tc.valid, *report.ConverterErrorUTF8Valid)
			require.NotNil(t, report.ConverterErrorBase64)
			recovered, err := base64.StdEncoding.DecodeString(*report.ConverterErrorBase64)
			require.NoError(t, err)
			require.Equal(t, []byte(tc.text), recovered)
			raw, err := json.Marshal(report)
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &fields))
			if tc.valid {
				require.NotNil(t, report.ConverterError)
				require.Equal(t, tc.text, *report.ConverterError)
				_, present := fields["converter_error"]
				require.True(t, present)
			} else {
				require.Nil(t, report.ConverterError)
				_, present := fields["converter_error"]
				require.False(t, present, "an invalid message must not become repaired human-readable evidence")
			}
		})
	}
}
