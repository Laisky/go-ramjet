package log

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	collectorlog "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/proto"
)

// configureSyntheticTLS installs production HTTPS settings using only temporary fixture credentials.
func configureSyntheticTLS(t *testing.T, server *httptest.Server) func(context.Context) error {
	t.Helper()
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	caFile := filepath.Join(dir, "ca.pem")
	require.NoError(t, os.WriteFile(tokenFile, []byte("synthetic-token"), 0600))
	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600))
	for key, value := range map[string]string{"APP_LOG_OTLP_ENDPOINT": server.URL, "APP_LOG_OTLP_TOKEN_FILE": tokenFile, "APP_LOG_OTLP_CA_FILE": caFile, "APP_LOG_OTLP_HOST": "b1", "APP_LOG_OTLP_ENV": "test"} {
		t.Setenv(key, value)
	}
	otlpState.Lock()
	previous := otlpState.retained
	otlpState.retained = nil
	otlpState.Unlock()
	t.Cleanup(func() {
		require.NoError(t, ShutdownOTLP(context.Background()))
		otlpState.Lock()
		otlpState.retained = previous
		otlpState.Unlock()
	})
	shutdown, err := ConfigureOTLP(context.Background())
	require.NoError(t, err)
	previousLogger := Logger
	Logger = Logger.WithOptions(privateOTLPOption())
	t.Cleanup(func() { Logger = previousLogger })
	return shutdown
}

// TestOTLPRedirectDoesNotForwardAuthorization exercises a same-origin redirect that ordinary clients follow.
func TestOTLPRedirectDoesNotForwardAuthorization(t *testing.T) {
	var original, redirected atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/logs" {
			original.Add(1)
			require.Equal(t, "Bearer synthetic-token", r.Header.Get("Authorization"))
			http.Redirect(w, r, "/redirected", http.StatusTemporaryRedirect)
			return
		}
		redirected.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	shutdown := configureSyntheticTLS(t, server)
	Logger.Info("redirect-fixture")
	require.Error(t, shutdown(context.Background()))
	require.EqualValues(t, 1, original.Load())
	require.Zero(t, redirected.Load())
	require.EqualValues(t, 1, LogOTLPStats().Failed)
}

// TestOTLPTransportIgnoresProxyEnvironment proves the private transport never consults environment proxies.
func TestOTLPTransportIgnoresProxyEnvironment(t *testing.T) {
	var proxyRequests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxyRequests.Add(1); w.WriteHeader(502) }))
	defer proxy.Close()
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("ALL_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	roots := x509.NewCertPool()
	client := newOTLPHTTPClient(roots)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.Nil(t, transport.Proxy)
	require.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	require.Equal(t, 5*time.Second, client.Timeout)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(200)
	}))
	defer server.Close()
	shutdown := configureSyntheticTLS(t, server)
	Logger.Info("proxy-fixture")
	require.NoError(t, shutdown(context.Background()))
	require.Zero(t, proxyRequests.Load())
	require.EqualValues(t, 1, LogOTLPStats().Exported)
}

// TestOTLPRequiresExplicitResourceLabels rejects missing deployment labels before enabling export.
func TestOTLPRequiresExplicitResourceLabels(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer server.Close()
	shutdown := configureSyntheticTLS(t, server)
	require.NoError(t, shutdown(context.Background()))
	otlpState.Lock()
	previous := otlpState.retained
	otlpState.retained = nil
	otlpState.Unlock()
	t.Cleanup(func() { otlpState.Lock(); otlpState.retained = previous; otlpState.Unlock() })
	for _, key := range []string{"APP_LOG_OTLP_HOST", "APP_LOG_OTLP_ENV"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "")
			_, err := ConfigureOTLP(context.Background())
			require.Error(t, err)
			require.Nil(t, otlpState.current.Load())
		})
	}
}

// TestOTLPDuplicateAttributesPreserved retains every application-supplied duplicate on the OTLP record.
func TestOTLPDuplicateAttributesPreserved(t *testing.T) {
	fields := make([]zapcore.Field, 1000)
	for index := range fields {
		fields[index] = zap.Int("attempt", index)
	}
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "", "", 2, 1, time.Hour))
	require.NoError(t, otlpHook(zapcore.Entry{Time: time.Now()}, fields))
	require.NoError(t, ShutdownOTLP(context.Background()))
	records := sink.snapshot()
	require.Len(t, records, 1)
	require.Equal(t, 1, records[0].AttributesLen())
	records[0].WalkAttributes(func(field attribute.KeyValue) bool {
		values := field.Value.AsSlice()
		require.Len(t, values, len(fields))
		for index, value := range values {
			require.EqualValues(t, index, value.AsInt64())
		}
		return true
	})
}

// TestOTLPPartialSuccessIsNotStorageAcceptance records sanitized terminal rejection without claiming storage acceptance.
func TestOTLPPartialSuccessIsNotStorageAcceptance(t *testing.T) {
	reports := make(chan string, 4)
	previous := otel.GetErrorHandler()
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { reports <- err.Error() }))
	t.Cleanup(func() { otel.SetErrorHandler(previous) })
	response, err := proto.Marshal(&collectorlog.ExportLogsServiceResponse{PartialSuccess: &collectorlog.ExportLogsPartialSuccess{RejectedLogRecords: 1, ErrorMessage: "receiver-diagnostic-secret-canary"}})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(response)
	}))
	defer server.Close()
	exporter, err := otlploghttp.New(context.Background(), otlploghttp.WithEndpointURL(server.URL), otlploghttp.WithInsecure(), otlploghttp.WithHeaders(map[string]string{}))
	require.NoError(t, err)
	isolateOTLP(t, newOTLPPipeline(exporter, "", "", 2, 1, time.Hour))
	Logger.Info("partial-success-fixture")
	if err := ShutdownOTLP(context.Background()); err != nil {
		require.Equal(t, "OTLP log shutdown did not complete", err.Error())
	}
	require.Zero(t, LogOTLPStats().Exported)
	require.EqualValues(t, 1, LogOTLPStats().Failed)
	select {
	case report := <-reports:
		require.Equal(t, "OTLP log export failed", report)
	default:
	}
}

// TestOTLPTransportUntrustedCertificateFails keeps TLS verification active.
func TestOTLPTransportUntrustedCertificateFails(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "unexpected") }))
	defer server.Close()
	client := newOTLPHTTPClient(x509.NewCertPool())
	_, err := client.Get(server.URL)
	require.Error(t, err)
}

// TestOTLPResourceEnvironmentPreserved validates malformed syntax and preserves operator-chosen metadata.
func TestOTLPResourceEnvironmentPreserved(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer server.Close()
	shutdown := configureSyntheticTLS(t, server)
	require.NoError(t, shutdown(context.Background()))
	otlpState.Lock()
	previous := otlpState.retained
	otlpState.retained = nil
	otlpState.Unlock()
	t.Cleanup(func() { otlpState.Lock(); otlpState.retained = previous; otlpState.Unlock() })
	reports := make(chan string, 4)
	handler := otel.GetErrorHandler()
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { reports <- err.Error() }))
	t.Cleanup(func() { otel.SetErrorHandler(handler) })
	for _, raw := range []string{"broken-synthetic-env-canary", "service.name=%zz-synthetic-env-canary"} {
		t.Run(raw[:min(len(raw), 10)], func(t *testing.T) {
			t.Setenv("OTEL_RESOURCE_ATTRIBUTES", raw)
			_, err := ConfigureOTLP(context.Background())
			require.Error(t, err)
			require.NotContains(t, err.Error(), "canary")
			require.Nil(t, otlpState.current.Load())
			require.Empty(t, reports)
		})
	}
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.name=ignored,host.name=ignored,deployment.environment.name=ignored,private_payload=synthetic-resource-canary")
	require.NoError(t, validateOTLPResource("b1", "test"))
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "b1", "test", 512, 128, time.Hour))
	Logger.Info("resource")
	require.NoError(t, ShutdownOTLP(context.Background()))
	records := sink.snapshot()
	require.Len(t, records, 1)
	attrs := records[0].Resource().Set()
	custom, ok := attrs.Value(attribute.Key("private_payload"))
	require.True(t, ok)
	require.Equal(t, "synthetic-resource-canary", custom.AsString())
	host, ok := attrs.Value(attribute.Key("host.name"))
	require.True(t, ok)
	require.Equal(t, "b1", host.AsString())
}

// TestOTLPResourceLabelsPreserveOperatorText accepts valid labels without bridge naming policy.
func TestOTLPResourceLabelsPreserveOperatorText(t *testing.T) {
	requests := make(chan []byte, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		requests <- body
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer server.Close()
	shutdown := configureSyntheticTLS(t, server)
	require.NoError(t, shutdown(context.Background()))
	otlpState.Lock()
	otlpState.retained = nil
	otlpState.Unlock()
	host := "synthetic host Unicode é " + strings.Repeat("h", 180)
	environment := "synthetic environment é " + strings.Repeat("e", 180)
	t.Setenv("APP_LOG_OTLP_HOST", host)
	t.Setenv("APP_LOG_OTLP_ENV", environment)
	shutdown, err := ConfigureOTLP(context.Background())
	require.NoError(t, err)
	Logger.Info("resource text")
	require.NoError(t, shutdown(context.Background()))
	var request collectorlog.ExportLogsServiceRequest
	require.NoError(t, proto.Unmarshal(<-requests, &request))
	fields := map[string]string{}
	for _, field := range request.ResourceLogs[0].Resource.Attributes {
		fields[field.Key] = field.Value.GetStringValue()
	}
	require.Equal(t, host, fields["host.name"])
	require.Equal(t, environment, fields["deployment.environment.name"])
}
