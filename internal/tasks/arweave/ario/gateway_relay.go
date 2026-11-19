package ario

import (
	"bufio"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/egress"
	"github.com/Laisky/go-ramjet/library/web"
)

const (
	// gatewayTimeout bounds one whole gateway exchange, including the wait
	// for response headers while a gateway fetches an uncached transaction.
	gatewayTimeout = 5 * time.Minute
	// cacheGatewayHost is the operator's own upload cache. Its answers carry
	// no ar.io hop header and are trusted when they come from this host.
	cacheGatewayHost = "s3.laisky.com"
	// sniffLen is how many leading bytes http.DetectContentType considers.
	sniffLen = 512
)

// gatewayEgress carries every gateway hop whose host is not a configured
// gateway host: ArNS subdomains and redirect targets. Those hosts are chosen
// by a caller or by a third-party gateway, so each hop is admitted by the
// shared public-destination policy and pinned. Tests replace it.
var gatewayEgress http.RoundTripper = egress.PinnedTransport{ResponseHeaderTimeout: gatewayTimeout}

// relayedGatewayHeaders lists the gateway response headers that describe the
// relayed content or its caching. Every other header, such as Set-Cookie,
// NEL, Report-To, Service-Worker-Allowed or Clear-Site-Data, would let a
// third-party gateway act on this origin, which also serves GPTChat, so it is
// dropped. Content-Encoding is relayed separately, only when unmodified.
var relayedGatewayHeaders = []string{
	"Accept-Ranges",
	"Cache-Control",
	"Content-Disposition",
	"Content-Length",
	"Content-Range",
	"Content-Type",
	"Etag",
	"Expires",
	"Last-Modified",
}

// relayedGatewayHeaderPrefix marks ar.io headers, which the handler relies on
// to recognize gateway answers and which describe the relayed transaction.
const relayedGatewayHeaderPrefix = "X-Ar-Io-"

// gatewayTransport routes each request, including every redirect hop. A hop
// to a configured gateway host over https uses the server transport, so
// operator-chosen gateways keep working even when they resolve privately;
// any other hop goes through the egress transport.
type gatewayTransport struct {
	trusted http.RoundTripper
	egress  http.RoundTripper
	hosts   map[string]struct{}
}

// RoundTrip sends request through the transport matching its destination.
func (t gatewayTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme == "https" {
		if _, ok := t.hosts[strings.ToLower(request.URL.Host)]; ok {
			return t.trusted.RoundTrip(request)
		}
	}
	return t.egress.RoundTrip(request)
}

// newGatewayClient returns the client for one fan-out over gateways. It keeps
// base's timeout and redirect policy and routes every hop with gatewayTransport,
// trusting exactly the https hosts listed in gateways.
func newGatewayClient(base *http.Client, gateways []string, egressTransport http.RoundTripper) *http.Client {
	hosts := make(map[string]struct{}, len(gateways))
	for _, gateway := range gateways {
		parsed, err := url.Parse(gateway)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			continue
		}
		hosts[strings.ToLower(parsed.Host)] = struct{}{}
	}
	trusted := base.Transport
	if trusted == nil {
		trusted = http.DefaultTransport
	}
	client := *base
	client.Transport = gatewayTransport{trusted: trusted, egress: egressTransport, hosts: hosts}
	return &client
}

// finalGatewayURL returns the URL of the last hop that produced response,
// falling back to target when the response does not record its request.
func finalGatewayURL(response *http.Response, target string) *url.URL {
	if response.Request != nil && response.Request.URL != nil {
		return response.Request.URL
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return &url.URL{}
	}
	return parsed
}

// isOperatorCache reports whether u, the final hop of a gateway exchange, is
// the operator's upload cache or one of its ArNS subdomains.
func isOperatorCache(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	return host == cacheGatewayHost || strings.HasSuffix(host, "."+cacheGatewayHost)
}

// relayGatewayResponse writes response to ctx: the allowlisted headers, the
// untrusted-content policy for the relayed type, the status and the body. A
// missing Content-Type is detected from the first bytes, as net/http would.
func relayGatewayResponse(ctx *gin.Context, response *http.Response) error {
	for _, name := range relayedGatewayHeaders {
		if value := response.Header.Get(name); value != "" {
			ctx.Header(name, value)
		}
	}
	// net/http drops Content-Encoding after decompressing on its own; a value
	// that is still present matches the body as relayed.
	if value := response.Header.Get("Content-Encoding"); value != "" && !response.Uncompressed {
		ctx.Header("Content-Encoding", value)
	}
	for name, values := range response.Header {
		if strings.HasPrefix(http.CanonicalHeaderKey(name), relayedGatewayHeaderPrefix) && len(values) > 0 {
			ctx.Header(name, values[0])
		}
	}

	body := bufio.NewReaderSize(response.Body, sniffLen)
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		head, err := body.Peek(sniffLen)
		if err != nil && !errors.Is(err, io.EOF) {
			return errors.Wrap(err, "read gateway content")
		}
		contentType = http.DetectContentType(head)
		ctx.Header("Content-Type", contentType)
	}
	// Applied after the gateway headers so it replaces any policy they carried.
	web.MarkUntrustedContent(ctx, contentType)
	ctx.Status(response.StatusCode)
	if _, err := io.Copy(ctx.Writer, body); err != nil {
		return errors.Wrap(err, "copy gateway content")
	}
	return nil
}
