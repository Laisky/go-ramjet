package ario

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/web"
)

var ArweaveGateways = []string{
	"https://permagate.io/",
	"https://ar-io.dev/",
	"https://g8way.io/",
	"https://arweave.net/",
	// cache
	"https://s3.laisky.com/public/arweave/uploadcache/",
	// non official domains
	"https://vevivoofficial.xyz/",
	"https://arweave.developerdao.com/",
	"https://snafyr.xyz/",
	"https://exnihilio.dnshome.de/",
	"https://gateway.ao/",
	"https://rikimaru111.site/",
	"https://0xsav.xyz/",
	"https://ar.satoshispalace.casino/",
	"https://mssnodes.xyz/",
	"https://kt10vip.online/",
	"https://skindetect.online/",
	"https://sulapan.com/",
	"https://frostor.xyz/",
	"https://ariogateway.online/",
	"https://alicans.online/",
	"https://zionalc.online/",
	"https://nodechecker.xyz/",
	"https://ainodes.xyz/",
	"https://ar.ionode.online/",
	"https://flechemano.com/",
	"https://yusufaytn.xyz/",
	"https://mrciga.com/",
	"https://aralper.xyz/",
	"https://2save.xyz/",
	"https://ariospeedwagon.com/",
	"https://siyantest.xyz/",
	"https://mygeto.online/",
	"https://love4src.com/",
	"https://adaconna.top/",
	"https://sooneraydin.xyz/",
	"https://cmdexe1.xyz/",
	"https://kabaoglu.store/",
	"https://adora0x0.xyz/",
	"https://khacasablanca.top/",
	"https://fennari.xyz/",
	"https://yukovskinode.online/",
	"https://ioar.xyz/",
	"https://kyotoorbust.site/",
	"https://canduesed.me/",
}

var (
	httpcli *http.Client
)

func init() {
	var err error
	httpcli, err = gutils.NewHTTPClient(
		gutils.WithHTTPClientTimeout(gatewayTimeout),
	)
	if err != nil {
		log.Logger.Panic("new http client", zap.Error(err))
	}
}

// RegexpArweaveFileID matches arweave file id
var RegexpArweaveFileID = regexp.MustCompile(`^[a-zA-Z0-9_-]{40,100}$`)

// regexpArNSName matches one ArNS name or undername ("undername_name") used as
// a single DNS label in front of a gateway host. It admits no URL delimiter,
// so the caller selects a subdomain of a configured gateway and never the host.
var regexpArNSName = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9_-]{0,61}[a-zA-Z0-9])?$`)

// maxGatewayRequestBodyBytes bounds the caller body buffered once and replayed
// to every gateway. Gateway reads carry no body; this only caps misuse.
const maxGatewayRequestBodyBytes = 1 << 20

// forwardedGatewayHeaders lists the caller headers that shape the requested
// content. Everything else, notably cookies, credentials and client metadata,
// stays on this service instead of reaching third-party gateways.
var forwardedGatewayHeaders = []string{
	"Accept",
	"Accept-Encoding",
	"Accept-Language",
	"Cache-Control",
	"Content-Type",
	"If-Match",
	"If-Modified-Since",
	"If-None-Match",
	"If-Range",
	"If-Unmodified-Since",
	"Range",
}

// gatewayTarget returns the URL fetched from gateway. With an ArNS domain it is
// the gateway's root on the domain's subdomain; otherwise it is fileKey on the gateway.
// The domain must already match regexpArNSName.
func gatewayTarget(gateway, domain, fileKey string) (string, error) {
	if domain == "" {
		return gateway + fileKey, nil
	}
	parsed, err := url.Parse(gateway)
	if err != nil {
		return "", errors.Wrapf(err, "parse gateway %q", gateway)
	}
	parsed.Host = domain + "." + parsed.Host
	return parsed.String(), nil
}

// GatewayRequestHeader returns a new header holding only the forwardable
// content-negotiation headers of source. Every relay of Arweave content uses
// it, so caller cookies and credentials never reach a gateway.
func GatewayRequestHeader(source http.Header) http.Header {
	header := make(http.Header, len(forwardedGatewayHeaders))
	for _, name := range forwardedGatewayHeaders {
		if values := source.Values(name); len(values) > 0 {
			header[name] = append([]string(nil), values...)
		}
	}
	return header
}

// GatewayHandler redirects request to multiple arweave gateways,
// and returns the first response.
//
// Arweave content is third-party content served on this origin, so only
// content and caching headers are relayed and the response is marked untrusted:
// active types are sandboxed into an opaque, script-free origin, while PDFs stay
// viewable. Every hop away from the configured gateway hosts, including
// redirects, passes the public egress policy.
func GatewayHandler(ctx *gin.Context) {
	fileKey := strings.Trim(ctx.Param("fileKey"), "/")
	domain := ctx.Query("domain")

	if fileKey == "" {
		ctx.Redirect(http.StatusFound, "https://ario.laisky.com/alias/doc")
		return
	}

	if !RegexpArweaveFileID.MatchString(fileKey) {
		web.AbortErr(ctx, errors.Errorf("invalid file key %q", fileKey))
		return
	}

	if domain != "" && !regexpArNSName.MatchString(domain) {
		web.AbortErr(ctx, errors.New("invalid domain, expect a single ArNS name such as name or undername_name"))
		return
	}

	targets := make([]string, 0, len(ArweaveGateways))
	for _, gw := range ArweaveGateways {
		target, err := gatewayTarget(gw, domain, fileKey)
		if web.AbortErr(ctx, err) {
			return
		}
		targets = append(targets, target)
	}

	// Every gateway receives its own reader over one bounded copy of the body;
	// a shared request body would be split between concurrent requests.
	var body []byte
	if ctx.Request.Body != nil {
		var err error
		body, err = io.ReadAll(io.LimitReader(ctx.Request.Body, maxGatewayRequestBodyBytes+1))
		if web.AbortErr(ctx, errors.Wrap(err, "read request body")) {
			return
		}
	}
	if len(body) > maxGatewayRequestBodyBytes {
		ctx.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"err": "request body too large"})
		return
	}

	logger := gmw.GetLogger(ctx).With(
		zap.String("method", ctx.Request.Method),
		zap.String("fileKey", fileKey),
	)

	taskCtx, taskCancel := context.WithCancel(gmw.Ctx(ctx))
	defer taskCancel()

	// Gateway goroutines may outlive this handler once the first response wins,
	// so they must not read the recycled gin context or mutable globals.
	fanOut := gatewayFanOut{
		client: newGatewayClient(httpcli, ArweaveGateways, gatewayEgress),
		method: ctx.Request.Method,
		header: GatewayRequestHeader(ctx.Request.Header),
		body:   body,
		logger: logger,
	}
	resp, err := fanOut.first(taskCtx, targets)
	if web.AbortErr(ctx, err) {
		return
	}
	defer gutils.LogErr(resp.Body.Close, logger)
	logger.Info("got response", zap.String("upstream", resp.Request.URL.String()))

	// A failure after the status is committed must abort the transfer, not
	// append an error document to a partial body.
	if err := RelayGatewayResponse(ctx, resp); err != nil {
		web.AbortStreamErr(ctx, errors.Wrap(err, "relay gateway response"))
	}
}
