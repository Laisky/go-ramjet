package http

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/egress"
)

// privateDNSHost is a public-looking name that the test resolver answers with
// a private address, standing in for an internal service behind DNS.
const privateDNSHost = "intranet.example"

// forgeryTarget is one internal destination a caller can name, directly or
// through a public hop that redirects to it.
type forgeryTarget struct {
	name string
	base string
	// hop is the public service base resolves to when the target is reached
	// by redirect; nil when base names the internal destination itself.
	hop *internalService
}

// newRedirectService starts a public-looking service that counts requests and
// answers every one with a 307 to location, preserving the method and body.
func newRedirectService(t *testing.T, location string) *internalService {
	t.Helper()
	service := &internalService{}
	service.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service.hits.Add(1)
		http.Redirect(w, r, location, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(service.Close)
	return service
}

// installForgeryTargets returns the internal destinations used by the sink
// tests: a loopback service, the link-local metadata address, a name that
// resolves to a private address, and a public provider that redirects to the
// metadata address. Caller-selected egress resolves privateDNSHost to 10.0.0.7
// and egressProviderHost to a public address served by the redirector. A plain
// client, which uses http.DefaultTransport, reaches the redirector for
// egressProviderHost and standIn for every internal destination, so a sink that
// skips admission on any hop is caught by the stand-in counting the request
// instead of contacting a real metadata service.
func installForgeryTargets(t *testing.T, standIn *internalService) []forgeryTarget {
	t.Helper()
	redirector := newRedirectService(t, "http://169.254.169.254/latest/meta-data/")
	egressTestSetup(t, redirector.Server)
	resolve := tenantEgressDeps.Resolve
	tenantEgressDeps.Resolve = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host == privateDNSHost {
			return []net.IPAddr{{IP: net.ParseIP("10.0.0.7")}}, nil
		}
		return resolve(ctx, host)
	}
	standInAddress := standIn.Listener.Addr().String()
	http.DefaultTransport = &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			destination := standInAddress
			switch address {
			case "169.254.169.254:80", privateDNSHost + ":80", standInAddress:
			case egressProviderHost + ":80":
				destination = redirector.Listener.Addr().String()
			default:
				return nil, errors.Errorf("unexpected dial to %s", address)
			}
			var dialer net.Dialer
			conn, err := dialer.DialContext(ctx, network, destination)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return conn, nil
		},
	}
	return []forgeryTarget{
		{name: "loopback", base: standIn.URL},
		{name: "link-local metadata", base: "http://169.254.169.254"},
		{name: "private DNS", base: "http://" + privateDNSHost},
		{name: "redirect to metadata", base: "http://" + egressProviderHost, hop: redirector},
	}
}

// requireAdmissionRefusal asserts that err is a typed refusal of a non-public destination.
func requireAdmissionRefusal(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	reason, ok := egress.ReasonOf(err)
	require.True(t, ok, "expected an admission refusal, got: %v", err)
	require.Equal(t, egress.ReasonDestinationDenied, reason)
}

// TestRequestForgerySinksRefuseInternalTargets calls every fetch function that
// sends a caller-chosen URL (the go/request-forgery sinks in image_dalle.go and
// one-shot chat) with loopback, link-local metadata and private-DNS targets, and
// with a public provider that redirects to the metadata address. Each must fail
// with a typed admission refusal before contacting the internal target; the
// redirect case must reach the public provider first, so the refusal is the
// redirect hop's. Replacing the egress client at any of these sinks with a
// plain http.Client makes the stand-in count a request and the test fail.
func TestRequestForgerySinksRefuseInternalTargets(t *testing.T) {
	sinks := map[string]func(t *testing.T, base string) error{
		// image_dalle.go:199, fetchImageFromOpenaiDalle -> cli.Do (alerts #121, #94).
		"openai generation": func(t *testing.T, base string) error {
			_, err := fetchImageFromOpenaiDalle(t.Context(), tenantSelectedUser(base), "dall-e-3", "synthetic", 1, "")
			return err
		},
		// image_dalle.go:322, downloadImage of a caller image URL (alerts #123, #83).
		"caller image download": func(t *testing.T, base string) error {
			_, err := getDataFromUrlOrBase64(t.Context(), base+"/latest/meta-data")
			return err
		},
		// image_dalle.go:522, fetchImageEditFromOpenai -> cli.Do (alerts #124, #96).
		"openai edit": func(t *testing.T, base string) error {
			png, err := base64.StdEncoding.DecodeString(testPNGBase64(t))
			require.NoError(t, err)
			_, err = fetchImageEditFromOpenai(t.Context(), tenantSelectedUser(base), "gpt-image-1", "synthetic", png, nil)
			return err
		},
		// library/openai/oneshot.go cli.Do through userOneshotChat (alert #97).
		"one-shot chat": func(t *testing.T, base string) error {
			_, err := userOneshotChat(t.Context(), tenantSelectedUser(base), "", "", "synthetic")
			return err
		},
	}
	for name, sink := range sinks {
		t.Run(name, func(t *testing.T) {
			standIn := newInternalService(t, http.StatusOK, `{"data":[{"b64_json":"aW50ZXJuYWw="}]}`)
			for _, target := range installForgeryTargets(t, standIn) {
				t.Run(target.name, func(t *testing.T) {
					standIn.hits.Store(0)
					if target.hop != nil {
						target.hop.hits.Store(0)
					}
					err := sink(t, target.base)
					require.Zero(t, standIn.hits.Load(), "the internal target must not be contacted")
					requireAdmissionRefusal(t, err)
					if target.hop != nil {
						require.NotZero(t, target.hop.hits.Load(), "the public hop must be admitted and contacted")
					}
				})
			}
		})
	}
}
