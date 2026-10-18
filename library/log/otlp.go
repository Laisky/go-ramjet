package log

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/errors/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

const otlpQueueSize = 512

var otlpState struct {
	sync.Mutex
	current  atomic.Pointer[otlpPipeline]
	retained *otlpPipeline
}

// otlpPipeline owns one private provider and its delivery counters.
type otlpPipeline struct {
	provider        *sdklog.LoggerProvider
	logger          otellog.Logger
	admission       sync.RWMutex
	stopped         bool
	redactionSecret string
	emitted         atomic.Uint64
	exported        atomic.Uint64
	failed          atomic.Uint64
}

// OTLPStats reports process-local attempts and outcomes, not durable acceptance.
// Unresolved includes queued, in-flight and SDK-overflow records; it is not a drop counter.
type OTLPStats struct {
	Enabled                               bool
	Emitted, Exported, Failed, Unresolved uint64
	QueueCapacity                         int
}

// LogOTLPStats returns bounded pipeline counters without emitting another log.
func LogOTLPStats() OTLPStats {
	otlpState.Lock()
	defer otlpState.Unlock()
	p := otlpState.retained
	s := OTLPStats{Enabled: otlpState.current.Load() != nil, QueueCapacity: otlpQueueSize}
	if p == nil {
		return s
	}
	s.Emitted, s.Exported, s.Failed = p.emitted.Load(), p.exported.Load(), p.failed.Load()
	if s.Emitted > s.Exported+s.Failed {
		s.Unresolved = s.Emitted - s.Exported - s.Failed
	}
	return s
}

// ConfigureOTLP enables the optional private provider once from APP_LOG_OTLP_*.
// Missing endpoint preserves stdout-only behavior without reading credential files.
func ConfigureOTLP(ctx context.Context) (func(context.Context) error, error) {
	otlpState.Lock()
	defer otlpState.Unlock()
	if otlpState.retained != nil {
		return ShutdownOTLP, nil
	}
	endpoint := os.Getenv("APP_LOG_OTLP_ENDPOINT")
	if endpoint == "" {
		return ShutdownOTLP, nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.New("invalid OTLP log endpoint")
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("OTLP log endpoint must be an HTTPS base URL without credentials, query or path")
	}
	token, err := readOTLPFile(os.Getenv("APP_LOG_OTLP_TOKEN_FILE"), 64*1024)
	if err != nil {
		return nil, errors.Wrap(err, "read OTLP bearer file")
	}
	bearer := strings.TrimSpace(string(token))
	if bearer == "" || strings.ContainsAny(bearer, " \r\n\t") {
		return nil, errors.New("invalid OTLP bearer file")
	}
	ca, err := readOTLPFile(os.Getenv("APP_LOG_OTLP_CA_FILE"), 1024*1024)
	if err != nil {
		return nil, errors.Wrap(err, "read OTLP CA file")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid OTLP CA file")
	}
	host, env := os.Getenv("APP_LOG_OTLP_HOST"), os.Getenv("APP_LOG_OTLP_ENV")
	valid := regexp.MustCompile(`^[a-zA-Z0-9._:-]{1,128}$`)
	for _, label := range []string{host, env} {
		if !valid.MatchString(label) {
			return nil, errors.New("invalid OTLP resource label")
		}
	}
	if err := validateOTLPResource(host, env); err != nil {
		return nil, err
	}
	exporter, err := otlploghttp.New(ctx,
		otlploghttp.WithEndpointURL(endpoint), otlploghttp.WithURLPath("/v1/logs"),
		otlploghttp.WithHeaders(map[string]string{"Authorization": "Bearer " + bearer}),
		otlploghttp.WithHTTPClient(newOTLPHTTPClient(roots)),
		otlploghttp.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, errors.New("create OTLP log exporter")
	}
	p := newOTLPPipeline(exporter, host, env, otlpQueueSize, 128, time.Second)
	p.redactionSecret = bearer
	otlpState.retained = p
	otlpState.current.Store(p)
	return ShutdownOTLP, nil
}

// readOTLPFile reads a bounded explicitly configured file without exposing its contents.
func readOTLPFile(path string, limit int64) ([]byte, error) {
	if path == "" {
		return nil, errors.New("OTLP file path is required")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("open OTLP file")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	closeErr := f.Close()
	if err != nil {
		return nil, errors.New("read OTLP file")
	}
	if closeErr != nil {
		return nil, errors.New("close OTLP file")
	}
	if int64(len(data)) > limit {
		return nil, errors.New("OTLP file exceeds size limit")
	}
	return data, nil
}

// newOTLPPipeline constructs an isolated SDK batch pipeline; test callers can inject an exporter.
func newOTLPPipeline(exporter sdklog.Exporter, host, env string, queue, batch int, interval time.Duration) *otlpPipeline {
	p := &otlpPipeline{}
	processor := sdklog.NewBatchProcessor(&countingOTLPExporter{Exporter: exporter, pipeline: p},
		sdklog.WithMaxQueueSize(queue), sdklog.WithExportMaxBatchSize(batch),
		sdklog.WithExportInterval(interval), sdklog.WithExportTimeout(5*time.Second))
	p.provider = sdklog.NewLoggerProvider(sdklog.WithResource(explicitOTLPResource(host, env)), sdklog.WithProcessor(processor),
		sdklog.WithAttributeCountLimit(24), sdklog.WithAttributeValueLengthLimit(2048))
	p.logger = p.provider.Logger("github.com/Laisky/go-ramjet/library/log")
	return p
}

// ShutdownOTLP stops new hook admission and drains the provider with a fresh bounded context.
// A hard process exit can still lose the process-local SDK buffer.
func ShutdownOTLP(ctx context.Context) error {
	otlpState.Lock()
	defer otlpState.Unlock()
	p := otlpState.current.Swap(nil)
	if p == nil {
		return nil
	}
	p.admission.Lock()
	p.stopped = true
	p.admission.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := p.provider.Shutdown(ctx); err != nil {
		return errors.New("OTLP log shutdown did not complete")
	}
	return nil
}

// countingOTLPExporter records terminal exporter outcomes without retaining payloads.
type countingOTLPExporter struct {
	sdklog.Exporter
	pipeline *otlpPipeline
}

// Export records final success or failure and strips unsafe transport error text.
func (e *countingOTLPExporter) Export(ctx context.Context, records []sdklog.Record) error {
	if err := e.Exporter.Export(ctx, records); err != nil {
		e.pipeline.failed.Add(uint64(len(records)))
		return errors.New("OTLP log export failed")
	}
	e.pipeline.exported.Add(uint64(len(records)))
	return nil
}

// newOTLPHTTPClient creates a private direct-only HTTPS transport with no proxy or redirects.
func newOTLPHTTPClient(roots *x509.CertPool) *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		MaxIdleConns:          2, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("OTLP redirects are disabled") },
	}
}

// explicitOTLPResource returns only application-owned immutable identity labels.
func explicitOTLPResource(host, env string) *resource.Resource {
	attrs := []attribute.KeyValue{attribute.String("service.name", "go-ramjet"), attribute.String("service.namespace", "laisky")}
	if host != "" {
		attrs = append(attrs, attribute.String("host.name", host))
	}
	if env != "" {
		attrs = append(attrs, attribute.String("deployment.environment.name", env))
	}
	return resource.NewSchemaless(attrs...)
}

// validateOTLPResource rejects unexpected SDK environment attributes before a provider can export them.
// Syntax is checked first because SDK detector diagnostics can contain malformed input text.
func validateOTLPResource(host, env string) error {
	raw := os.Getenv("OTEL_RESOURCE_ATTRIBUTES")
	if len(raw) > 16*1024 {
		return errors.New("unexpected OTLP resource environment configuration")
	}
	if raw != "" {
		for _, pair := range strings.Split(raw, ",") {
			key, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
			key = strings.TrimSpace(key)
			if !ok || !validOTLPKey(key) || !fixedOTLPResourceKey(key) {
				return errors.New("unexpected OTLP resource environment configuration")
			}
			if _, err := url.PathUnescape(strings.TrimSpace(value)); err != nil {
				return errors.New("unexpected OTLP resource environment configuration")
			}
		}
	}
	explicit := explicitOTLPResource(host, env)
	merged, err := resource.Merge(resource.Environment(), explicit)
	if err != nil || !merged.Equal(explicit) {
		return errors.New("unexpected OTLP resource environment configuration")
	}
	return nil
}

// fixedOTLPResourceKey identifies metadata keys that explicit application identity always overrides.
func fixedOTLPResourceKey(key string) bool {
	switch key {
	case "service.name", "service.namespace", "host.name", "deployment.environment.name":
		return true
	default:
		return false
	}
}
