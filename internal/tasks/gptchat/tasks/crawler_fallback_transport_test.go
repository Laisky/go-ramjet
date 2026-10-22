package tasks

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/stretchr/testify/require"
)

// TestCrawlerFallbackPreservesTLSIdentity runs actual TLS handshakes through the
// direct transport and verifies that identity failures never invoke providers.
func TestCrawlerFallbackPreservesTLSIdentity(t *testing.T) {
	for _, mode := range []string{"trusted", "unknown_ca", "wrong_hostname", "dial_timeout"} {
		t.Run(mode, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			task := fallbackTestTask(t, true)
			task.Url = "https://example.com/document"
			if mode == "wrong_hostname" {
				task.Url = "https://wrong.example/document"
				task.Egress.Host = "wrong.example"
			}
			var dials, requests, providerCalls atomic.Int32
			fixture := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				_, err := w.Write([]byte("<body>trusted direct content</body>"))
				require.NoError(t, err)
			}))
			t.Cleanup(fixture.Close)
			deps := rendererTestDependencies(t, fixture, "8.8.8.8", &dials)
			deps.rootCAs = x509.NewCertPool()
			if mode != "unknown_ca" {
				deps.rootCAs.AddCert(fixture.Certificate())
			}
			if mode == "dial_timeout" {
				deps.dial = func(context.Context, string, string) (net.Conn, error) {
					dials.Add(1)
					return nil, &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded}
				}
			}
			fetchVerifiedHTMLContent = func(ctx context.Context, direct *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				fetcher, err := newRendererEgressFetcher(ctx, direct.TaskID, direct.Url, *direct.Egress, deps)
				if err != nil {
					return "", nil, err
				}
				_, body, err := fetcher.fetch(ctx, "document", "", direct.Url, crawleregress.OriginDocument, http.MethodGet, nil)
				if err != nil {
					if mode == "unknown_ca" || mode == "wrong_hostname" {
						var verification *tls.CertificateVerificationError
						require.ErrorAs(t, err, &verification, "fixture must exercise an actual certificate rejection")
					}
					return "", nil, err
				}
				receipt, err := fetcher.seal(ctx)
				return string(body), receipt, err
			}
			webFetchProxies = []webFetchProxy{countingVerifiedProxy{calls: &providerCalls}}
			states := captureFallbackPublication(t)
			require.NoError(t, processHTMLCrawlerTask(t.Context(), task))
			require.Len(t, *states, 2)
			result := (*states)[1]
			require.EqualValues(t, 1, dials.Load())
			switch mode {
			case "trusted":
				require.Equal(t, rlibs.TaskStatusSuccess, result.Status)
				require.EqualValues(t, 1, requests.Load())
				require.Zero(t, providerCalls.Load())
				require.Equal(t, crawleregress.AcquisitionDirect, result.Acquisition.Method)
			case "dial_timeout":
				require.Equal(t, rlibs.TaskStatusSuccess, result.Status)
				require.Zero(t, requests.Load())
				require.EqualValues(t, 1, providerCalls.Load())
				require.Equal(t, crawleregress.AcquisitionProvider, result.Acquisition.Method)
			default:
				require.Equal(t, rlibs.TaskStatusFailed, result.Status)
				require.Zero(t, requests.Load())
				require.Zero(t, providerCalls.Load(), "a rejected TLS identity is not permission to disclose the target")
				require.Nil(t, result.Acquisition)
			}
		})
	}
}

// TestCrawlerFallbackDistinguishesDNSFailure exercises redirect-host admission
// with real initial HTTP and controlled resolver answers, preserving policy denials.
func TestCrawlerFallbackDistinguishesDNSFailure(t *testing.T) {
	for _, mode := range []string{
		"temporary", "child_timeout", "temporary_second_lookup", "child_timeout_second_lookup",
		"public", "private", "mixed", "empty", "rebinding", "parent_canceled",
	} {
		t.Run(mode, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			var dials, providerCalls, resolutions atomic.Int32
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Host == "example.com" {
					w.Header().Set("Location", "http://redirect.example/document")
					w.WriteHeader(http.StatusFound)
					return
				}
				_, err := w.Write([]byte("<body>redirected direct content</body>"))
				require.NoError(t, err)
			}))
			t.Cleanup(fixture.Close)
			deps := rendererTestDependencies(t, fixture, "8.8.8.8", &dials)
			deps.resolve = func(_ context.Context, host string) ([]net.IPAddr, error) {
				lookup := resolutions.Add(1)
				require.Equal(t, "redirect.example", host, "original host must use submitted pins")
				if lookup == 1 && (strings.HasSuffix(mode, "_second_lookup") || mode == "rebinding") {
					return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
				}
				switch strings.TrimSuffix(mode, "_second_lookup") {
				case "temporary":
					return nil, &net.DNSError{Name: host, Err: "temporary fixture failure", IsTemporary: true}
				case "child_timeout":
					return nil, context.DeadlineExceeded
				case "private", "rebinding":
					return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
				case "mixed":
					return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("127.0.0.1")}}, nil
				case "empty":
					return nil, nil
				case "parent_canceled":
					cancel()
					return nil, context.Canceled
				default:
					return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
				}
			}
			fetchVerifiedHTMLContent = func(renderCtx context.Context, direct *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				fetcher, err := newRendererEgressFetcher(renderCtx, direct.TaskID, direct.Url, *direct.Egress, deps)
				if err != nil {
					return "", nil, err
				}
				response, _, err := fetcher.fetch(renderCtx, "document", "", direct.Url, crawleregress.OriginDocument, http.MethodGet, nil)
				if err != nil {
					return "", nil, err
				}
				_, body, err := fetcher.fetch(renderCtx, "redirect", "document", response.Header.Get("Location"), crawleregress.OriginDocument, http.MethodGet, nil)
				if err != nil {
					return "", nil, err
				}
				receipt, err := fetcher.seal(renderCtx)
				return string(body), receipt, err
			}
			webFetchProxies = []webFetchProxy{countingVerifiedProxy{calls: &providerCalls}}
			result, err := acquireCrawlerContent(ctx, fallbackTestTask(t, true))
			require.Positive(t, resolutions.Load())
			switch mode {
			case "temporary", "child_timeout", "temporary_second_lookup", "child_timeout_second_lookup":
				require.NoError(t, err)
				require.EqualValues(t, 1, dials.Load(), "failed resolution never dials the redirect")
				require.EqualValues(t, 1, providerCalls.Load())
				require.Equal(t, crawleregress.AcquisitionProvider, result.acquisition.Method)
			case "public":
				require.NoError(t, err)
				require.EqualValues(t, 2, dials.Load())
				require.Zero(t, providerCalls.Load())
				require.Equal(t, crawleregress.AcquisitionDirect, result.acquisition.Method)
			default:
				require.Error(t, err)
				require.Nil(t, result)
				require.EqualValues(t, 1, dials.Load())
				require.Zero(t, providerCalls.Load())
				if mode == "parent_canceled" {
					require.True(t, errors.Is(err, context.Canceled))
				}
			}
		})
	}
}
