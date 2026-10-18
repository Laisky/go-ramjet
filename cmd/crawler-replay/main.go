// Command crawler-replay reproduces local conversion from a validated caller/worker capture pair.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"unicode/utf8"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-ramjet/library/crawlercapture"
)

const (
	converterModule = "github.com/JohannesKaufmann/html-to-markdown/v2"
	converterName   = "html-to-markdown/v2"
	// TestReplayConverterSourcePin checks this fallback against the repository module pin.
	sourcePinnedConverterVersion = "v2.5.2"
)

// converterBuild identifies the executing dependency without inspecting application configuration.
type converterBuild struct {
	Version string
	Source  string
}

// replayReport preserves the original URL and summarizes exact conversion bytes without dumping HTML.
type replayReport struct {
	CallerSource                 crawlercapture.Source  `json:"caller_source"`
	WorkerSource                 crawlercapture.Source  `json:"worker_source"`
	TaskID                       string                 `json:"task_id"`
	RequestID                    string                 `json:"request_id,omitempty"`
	OriginalURL                  *string                `json:"original_url,omitempty"`
	OriginalURLUTF8Valid         bool                   `json:"original_url_utf8_valid"`
	OriginalURLBase64            string                 `json:"original_url_base64"`
	OriginalURLBytes             int64                  `json:"original_url_bytes"`
	OriginalURLSHA256            string                 `json:"original_url_sha256"`
	WorkerFailure                crawlercapture.Failure `json:"worker_failure"`
	Converter                    string                 `json:"converter"`
	CapturedConverterVersion     string                 `json:"captured_converter_version"`
	ReplayConverterVersion       string                 `json:"replay_converter_version"`
	ReplayConverterVersionSource string                 `json:"replay_converter_version_source"`
	VersionQualification         string                 `json:"version_qualification"`
	InputState                   string                 `json:"input_state"`
	InputBytes                   int64                  `json:"input_bytes"`
	InputSHA256                  string                 `json:"input_sha256,omitempty"`
	ReplayClassification         string                 `json:"replay_classification"`
	OutputBytes                  *int64                 `json:"output_bytes,omitempty"`
	OutputSHA256                 string                 `json:"output_sha256,omitempty"`
	TrimmedOutputBytes           *int64                 `json:"trimmed_output_bytes,omitempty"`
	TrimmedOutputSHA256          string                 `json:"trimmed_output_sha256,omitempty"`
	ReproductionStatus           string                 `json:"reproduction_status"`
	ConverterError               *string                `json:"converter_error,omitempty"`
	ConverterErrorBase64         *string                `json:"converter_error_base64,omitempty"`
	ConverterErrorUTF8Valid      *bool                  `json:"converter_error_utf8_valid,omitempty"`
}

// currentConverterBuild uses linked dependency metadata, with a source-pin fallback for test binaries.
func currentConverterBuild() converterBuild {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dependency := range info.Deps {
			if dependency.Path != converterModule {
				continue
			}
			if dependency.Replace != nil {
				return converterBuild{Version: dependency.Replace.Version, Source: "replacement_build_metadata"}
			}
			if dependency.Version != "" {
				return converterBuild{Version: dependency.Version, Source: "build_metadata"}
			}
		}
	}
	return converterBuild{Version: sourcePinnedConverterVersion, Source: "source_pin_fallback"}
}

// knownConverterVersion prevents absent build information from being described as an exact match.
func knownConverterVersion(version string) bool {
	switch version {
	case "", "unknown", "unavailable", "(devel)":
		return false
	default:
		return true
	}
}

// sameCapturedURL checks actual captured bytes; a missing observation cannot establish equality.
func sameCapturedURL(left, right crawlercapture.Blob) bool {
	return left.State == "captured" && right.State == "captured" && bytes.Equal(left.Bytes(), right.Bytes())
}

// replay validates provenance before invoking the same local converter on the captured argument.
func replay(caller, worker crawlercapture.Record, build converterBuild) (replayReport, error) {
	if err := caller.Validate(); err != nil {
		return replayReport{}, errors.New("caller capture validation failed")
	}
	if err := worker.Validate(); err != nil {
		return replayReport{}, errors.New("worker capture validation failed")
	}
	taskID := caller.Correlation.TaskID
	if taskID == "" || taskID != worker.Correlation.TaskID {
		return replayReport{}, errors.New("caller and worker task identities differ or are absent")
	}
	if worker.Source.Service != "go-ramjet" || caller.Source.Service == "" || caller.Source.Service == worker.Source.Service {
		return replayReport{}, errors.New("capture sources do not identify a distinct caller and go-ramjet worker")
	}
	if caller.Request.OriginalURL.State != "captured" {
		return replayReport{}, errors.New("original caller URL was not captured")
	}
	if caller.Request.SubmittedURL.State != "captured" || strings.TrimSpace(string(caller.Request.OriginalURL.Bytes())) != string(caller.Request.SubmittedURL.Bytes()) {
		return replayReport{}, errors.New("caller original URL does not trim to its submitted URL")
	}
	if !sameCapturedURL(caller.Request.SubmittedURL, worker.Request.OriginalURL) {
		return replayReport{}, errors.New("caller submitted URL differs from worker original URL")
	}
	if worker.Request.SubmittedURL.State == "captured" && !sameCapturedURL(caller.Request.SubmittedURL, worker.Request.SubmittedURL) {
		return replayReport{}, errors.New("caller submitted URL differs from worker submitted URL")
	}
	if caller.Request.OutputMarkdown != worker.Request.OutputMarkdown {
		return replayReport{}, errors.New("caller and worker output options differ")
	}
	if caller.Failure.WorkerStage != "" && caller.Failure.WorkerStage != worker.Failure.Stage {
		return replayReport{}, errors.New("caller worker-stage evidence differs from worker failure")
	}
	if caller.Failure.WorkerCode != "" && caller.Failure.WorkerCode != worker.Failure.Code {
		return replayReport{}, errors.New("caller worker-code evidence differs from worker failure")
	}
	if worker.Converter.Name != converterName {
		return replayReport{}, errors.New("capture does not identify the supported local converter")
	}
	qualification := "unqualified_version_unavailable"
	if knownConverterVersion(worker.Converter.Version) && knownConverterVersion(build.Version) {
		if worker.Converter.Version != build.Version {
			return replayReport{}, errors.New("captured converter version differs from replay dependency")
		}
		qualification = "matched"
	}
	original := caller.Request.OriginalURL.Bytes()
	report := replayReport{
		CallerSource: caller.Source, WorkerSource: worker.Source,
		TaskID: taskID, RequestID: caller.Correlation.RequestID,
		OriginalURLUTF8Valid: utf8.Valid(original), OriginalURLBase64: base64.StdEncoding.EncodeToString(original),
		OriginalURLBytes: int64(len(original)), OriginalURLSHA256: digest(original), WorkerFailure: worker.Failure,
		Converter: converterName, CapturedConverterVersion: worker.Converter.Version,
		ReplayConverterVersion: build.Version, ReplayConverterVersionSource: build.Source,
		VersionQualification: qualification, InputState: worker.Converter.Input.State,
		InputBytes: worker.Converter.Input.ByteLength, InputSHA256: worker.Converter.Input.SHA256,
		ReplayClassification: "not_reached",
	}
	if report.OriginalURLUTF8Valid {
		text := string(original)
		report.OriginalURL = &text
	}
	report.ReproductionStatus = "not_applicable"
	if worker.Converter.Input.State != "captured" {
		return report, nil
	}
	markdown, err := md.ConvertString(string(worker.Converter.Input.Bytes()))
	if err != nil {
		report.ReplayClassification = "conversion_failed"
		recordReplayError(&report, err)
		report.ReproductionStatus = reproductionStatus(worker.Failure, report.ReplayClassification, qualification)
		return report, nil
	}
	trimmed := strings.TrimSpace(markdown)
	rawBytes, trimmedBytes := int64(len(markdown)), int64(len(trimmed))
	report.OutputBytes, report.TrimmedOutputBytes = &rawBytes, &trimmedBytes
	report.OutputSHA256, report.TrimmedOutputSHA256 = digest([]byte(markdown)), digest([]byte(trimmed))
	report.ReplayClassification = "markdown"
	if trimmed == "" {
		report.ReplayClassification = "empty_markdown"
	}
	report.ReproductionStatus = reproductionStatus(worker.Failure, report.ReplayClassification, qualification)
	return report, nil
}

// recordReplayError preserves the actual converter error bytes without JSON string repair.
func recordReplayError(report *replayReport, err error) {
	text := err.Error()
	valid := utf8.ValidString(text)
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	report.ConverterErrorUTF8Valid, report.ConverterErrorBase64 = &valid, &encoded
	if valid {
		report.ConverterError = &text
	}
}

// reproductionStatus compares conversion classifications without attributing browser or publication causes.
func reproductionStatus(failure crawlercapture.Failure, classification, qualification string) string {
	if failure.Stage != "conversion" || (failure.Code != "empty_markdown" && failure.Code != "conversion_failed") || classification == "not_reached" {
		return "not_applicable"
	}
	if failure.Code != classification {
		return "mismatch"
	}
	if qualification != "matched" {
		return "unqualified"
	}
	return "match"
}

// digest reports byte identity without including input or output payloads.
func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// run reads only explicitly named private journal files and writes one JSON result.
func run(args []string, output, diagnostics io.Writer) error {
	flags := flag.NewFlagSet("crawler-replay", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	callerPath := flags.String("caller", "", "private caller capture .json path")
	workerPath := flags.String("worker", "", "private worker capture .json path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *callerPath == "" || *workerPath == "" || flags.NArg() != 0 {
		return errors.New("provide --caller and --worker capture paths without positional arguments")
	}
	caller, err := crawlercapture.Read(*callerPath)
	if err != nil {
		return errors.Wrap(err, "read caller capture")
	}
	worker, err := crawlercapture.Read(*workerPath)
	if err != nil {
		return errors.Wrap(err, "read worker capture")
	}
	report, err := replay(caller, worker, currentConverterBuild())
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(report)
}

// main reports offline replay failures without initializing application services.
func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "crawler-replay:", err)
		os.Exit(1)
	}
}
