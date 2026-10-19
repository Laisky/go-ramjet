package tasks

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/require"

	appLog "github.com/Laisky/go-ramjet/library/log"
)

// crawlerPanickingDiagnosticCause simulates a formatter panic containing synthetic credential data.
type crawlerPanickingDiagnosticCause struct{}

// Error panics to verify that diagnostic projection never renders a panic value.
func (*crawlerPanickingDiagnosticCause) Error() string {
	panic("api_key=panic-private-key-canary body=panic-private-body-canary")
}

// captureCrawlerDiagnosticProjection emits an error through actual local JSON and SDK HTTP serialization.
func captureCrawlerDiagnosticProjection(t *testing.T, failure error) (string, string) {
	t.Helper()
	messages := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		var request struct {
			Variables struct {
				Message string `json:"msg"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20)).Decode(&request); err != nil {
			http.Error(w, "invalid synthetic request", http.StatusBadRequest)
			return
		}
		messages <- request.Variables.Message
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":{"TelegramMonitorAlert":{"name":"accepted"}}}`)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	alert, err := glog.NewAlert(ctx, server.URL, glog.WithAlertType("synthetic"), glog.WithAlertToken("synthetic-only"), glog.WithAlertMaxMessageBytes(appLog.HTTPAlertMaxMessageBytes))
	require.NoError(t, err)
	t.Cleanup(func() {
		alert.Close()
		select {
		case <-alert.Done():
		case <-time.After(3 * time.Second):
			t.Error("SDK sender failed to stop")
		}
	})
	var local bytes.Buffer
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&local), zap.DebugLevel)
	logger, err := glog.NewWithName("go-ramjet.projection-test", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }), zap.HooksWithFields(alert.GetZapHook()))
	require.NoError(t, err)
	require.NotPanics(t, func() { logger.Error("synthetic crawler failure", zap.Error(failure)) })
	select {
	case message := <-messages:
		return local.String(), message
	case <-time.After(3 * time.Second):
		t.Fatal("no diagnostic HTTP alert")
		return "", ""
	}
}

// TestCrawlerDiagnosticTextBounds retains useful context and limits complete error text including its marker.
func TestCrawlerDiagnosticTextBounds(t *testing.T) {
	ordinary := crawlerFailure("render_failed", "render", errors.New("navigation failed: connection refused"))
	require.Contains(t, ordinary.Error(), "navigation failed: connection refused")
	for _, suffix := range []string{strings.Repeat("a", 10000), strings.Repeat("界", 10000)} {
		failure := crawlerFailure("render_failed", "render", errors.New("navigation failed: "+suffix))
		require.LessOrEqual(t, len(failure.Error()), 2048, "the entire projection includes its truncation marker")
		require.Contains(t, failure.Error(), "navigation failed")
		require.Contains(t, failure.Error(), "[truncated]")
		require.True(t, utf8.ValidString(failure.Error()))
	}
}

// TestCrawlerDiagnosticProjectionHTTPAndJSONBounds checks emitted errors rather than only helper output.
func TestCrawlerDiagnosticProjectionHTTPAndJSONBounds(t *testing.T) {
	failure := crawlerFailure("render_failed", "render", errors.New("navigation failed: "+strings.Repeat("bounded-message", 2000)))
	local, remote := captureCrawlerDiagnosticProjection(t, failure)
	var fields map[string]any
	require.NoError(t, json.Unmarshal([]byte(local), &fields))
	require.LessOrEqual(t, len(fields["error"].(string)), 2048)
	require.Contains(t, fields["error"], "[truncated]")
	require.Contains(t, remote, "navigation failed")
	require.Contains(t, remote, "[truncated]")
	// The SDK now retains ordinary Zap rich-error fields, including verbose stack
	// text. Their complete alert need not fit twice the application error-text bound.
	offset := strings.Index(remote, "\n{")
	require.GreaterOrEqual(t, offset, 0, "actual alert must retain its structured error fields")
	var remoteFields map[string]any
	require.NoError(t, json.Unmarshal([]byte(remote[offset+1:]), &remoteFields))
	require.Equal(t, fields["error"], remoteFields["error"], "transport preserves the exact application-owned projection")
	remoteError := remoteFields["error"].(string)
	require.LessOrEqual(t, len(remoteError), crawlerDiagnosticTextLimit)
	require.True(t, utf8.ValidString(remoteError))
	if verbose, present := fields["errorVerbose"]; present {
		require.Equal(t, verbose, remoteFields["errorVerbose"], "faithful transport preserves the ordinary Zap verbose representation")
	} else {
		require.NotContains(t, remoteFields, "errorVerbose")
	}
	require.LessOrEqual(t, len(remote), appLog.HTTPAlertMaxMessageBytes, "the complete formatted alert respects its explicit SDK admission budget")
}

// TestCrawlerDiagnosticPanickingCauseNeverLeaks keeps a formatter panic out of direct and emitted diagnostics.
func TestCrawlerDiagnosticPanickingCauseNeverLeaks(t *testing.T) {
	cause := &crawlerPanickingDiagnosticCause{}
	failure := crawlerFailure("render_failed", "render", cause)
	require.ErrorIs(t, failure, cause)
	var typed *crawlerPanickingDiagnosticCause
	require.ErrorAs(t, failure, &typed)
	require.NotPanics(t, func() { _ = failure.Error() })
	require.NotPanics(t, func() { _ = fmt.Sprintf("%+v", failure) })
	local, remote := captureCrawlerDiagnosticProjection(t, failure)
	for _, output := range []string{failure.Error(), fmt.Sprintf("%+v", failure), local, remote} {
		require.Contains(t, output, "render_failed")
		require.NotContains(t, output, "panic-private")
		require.NotContains(t, output, "PANIC")
	}
}

// TestCrawlerDiagnosticBrowserPayloadOmitted recognizes typed page exceptions without parsing their user text.
func TestCrawlerDiagnosticBrowserPayloadOmitted(t *testing.T) {
	cause := &chromedp.ExceptionError{ExceptionDetails: &cdpruntime.ExceptionDetails{Text: "unlabeled-private-page-text", LineNumber: 7, ColumnNumber: 12,
		Exception: &cdpruntime.RemoteObject{ClassName: "TypeError", Description: "<article>unlabeled-private-html-content</article>"}}}
	failure := crawlerFailure("render_failed", "render", errors.WithStack(cause))
	var typed *chromedp.ExceptionError
	require.ErrorAs(t, failure, &typed)
	require.Same(t, cause, typed)
	local, remote := captureCrawlerDiagnosticProjection(t, failure)
	for _, output := range []string{failure.Error(), fmt.Sprintf("%+v", failure), local, remote} {
		require.Contains(t, output, "render_failed")
		require.NotContains(t, output, "unlabeled-private")
		require.NotContains(t, output, "<article>")
		require.Contains(t, output, "TypeError")
		require.Contains(t, output, "7:12")
	}
}

// TestCrawlerDiagnosticTypedTransportPrivacy retains routine operational context while excluding URLs, credentials and certificate payloads.
func TestCrawlerDiagnosticTypedTransportPrivacy(t *testing.T) {
	certificate := &x509.Certificate{DNSNames: []string{"private-certificate-name.invalid"}}
	hostname := x509.HostnameError{Certificate: certificate, Host: "tls.example.test"}
	for _, tc := range []struct {
		name    string
		cause   error
		useful  []string
		private []string
	}{
		{name: "dns", cause: &net.DNSError{Err: "no such host", Name: "resolver.example.test", Server: "192.0.2.73:53"},
			useful: []string{"DNS lookup failed", "resolver.example.test", "no such host"}},
		{name: "dns_credential", cause: &net.DNSError{Err: "no such host", Name: "https://synthetic-user:synthetic-password@resolver.example.test/path?api_key=dns-private-key", Server: "192.0.2.73:53"},
			useful: []string{"DNS lookup failed"}, private: []string{"https://", "synthetic-user", "synthetic-password", "dns-private-key", "/path"}},
		{name: "tls_hostname", cause: hostname, useful: []string{"TLS certificate verification failed", "tls.example.test"},
			private: []string{"private-certificate-name"}},
		{name: "tls_verification", cause: &tls.CertificateVerificationError{UnverifiedCertificates: []*x509.Certificate{certificate}, Err: hostname},
			useful: []string{"TLS certificate verification failed", "tls.example.test"}, private: []string{"private-certificate-name"}},
		{name: "tls_credential", cause: x509.HostnameError{Certificate: certificate, Host: "api_key=tls-private-key"},
			useful: []string{"TLS certificate verification failed"}, private: []string{"tls-private-key", "private-certificate-name"}},
		{name: "connection", cause: &net.OpError{Op: "dial", Net: "tcp", Source: &net.TCPAddr{IP: net.ParseIP("192.0.2.61"), Port: 4321},
			Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.62"), Port: 443}, Err: syscall.ECONNREFUSED},
			useful: []string{"connection refused", "dial tcp"}, private: []string{"192.0.2.61", "192.0.2.62"}},
		{name: "connection_credential", cause: &net.OpError{Op: "dial token=operation-private-token", Net: "tcp", Err: syscall.ECONNREFUSED},
			useful: []string{"connection refused", "dial"}, private: []string{"operation-private-token"}},
		{name: "url", cause: &url.Error{Op: "Get", URL: "https://synthetic-user:synthetic-password@request.example.test/private-path?api_key=request-private-key", Err: syscall.ECONNREFUSED},
			useful: []string{"connection refused", "Get"}, private: []string{"https://", "synthetic-user", "synthetic-password", "private-path", "request-private-key"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := crawlerFailure("render_failed", "render", errors.WithStack(tc.cause))
			require.ErrorIs(t, failure, tc.cause)
			code, stage := crawlerDiagnostic(failure)
			require.Equal(t, "render_failed", code)
			require.Equal(t, "render", stage)
			local, remote := captureCrawlerDiagnosticProjection(t, failure)
			for _, output := range []string{failure.Error(), fmt.Sprintf("%+v", failure), local, remote} {
				for _, useful := range tc.useful {
					require.Contains(t, output, useful)
				}
				for _, private := range tc.private {
					require.NotContains(t, output, private)
				}
			}
		})
	}
}
