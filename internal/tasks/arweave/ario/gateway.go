package ario

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/web"
)

var ArweaveGateways = []string{
	"https://akrd.net/",
	"https://permagate.io/",
	"https://ar-io.dev/",
	"https://vilenarios.com/",
	"https://arbr.pro/",
	"https://frostor.xyz/",
	"https://ariospeedwagon.com/",
	"https://vikanren.buzz/",
	"https://jaxtothehell.xyz/",
	"https://sulapan.com/",
	"https://arweave.fllstck.dev/",
	"https://yukovskibot.com/",
	// "https://rerererararags.store/",
	"https://karakartal.store/",
	"https://ruangnode.xyz/",
	// "https://logosnodos.site/",
	// "https://budavlebac.online/",
	// "https://testnetnodes.xyz/",  // domain expires
	// "https://aleko0o.store/",
}

var (
	httpcli *http.Client
)

func init() {
	var err error
	httpcli, err = gutils.NewHTTPClient(
		gutils.WithHTTPClientTimeout(5 * time.Minute),
	)
	if err != nil {
		log.Logger.Panic("new http client", zap.Error(err))
	}
}

// RegexpArweaveFileID matches arweave file id
var RegexpArweaveFileID = regexp.MustCompile(`^[a-zA-Z0-9_-]{40,100}$`)

// GatewayHandler redirects request to multiple arweave gateways,
// and returns the first response.
func GatewayHandler(ctx *gin.Context) {
	fileKey := strings.Trim(ctx.Param("fileKey"), "/")
	domain := ctx.Query("domain")

	if !RegexpArweaveFileID.MatchString(fileKey) {
		web.AbortErr(ctx, errors.Errorf("invalid file key %q", fileKey))
		return
	}

	logger := gmw.GetLogger(ctx).With(
		zap.String("method", ctx.Request.Method),
		zap.String("fileKey", fileKey),
	)

	firstFinished := make(chan *http.Response)
	taskCtx, taskCancel := context.WithCancel(gmw.Ctx(ctx))
	defer taskCancel()

	var pool errgroup.Group
	for _, gw := range ArweaveGateways {
		var url string
		if domain != "" {
			url = strings.Replace(gw, "https://", "https://"+domain+".", 1)
		} else {
			url = gw + fileKey
		}

		pool.Go(func() error {
			logger := logger.With(zap.String("target_url", url))
			logger.Debug("fetching file")

			req, err := http.NewRequestWithContext(taskCtx,
				ctx.Request.Method, url, ctx.Request.Body)
			if err != nil {
				return errors.Wrapf(err, "new request %q", url)
			}

			req.Header = ctx.Request.Header
			resp, err := httpcli.Do(req)
			if err != nil {
				return errors.Wrapf(err, "do request %q", url)
			}

			if resp.StatusCode == http.StatusOK {
				select {
				case firstFinished <- resp: // close body later
				case <-taskCtx.Done():
					_ = resp.Body.Close()
				}

				return nil
			}

			_ = resp.Body.Close()
			return errors.Errorf("request %q, got %d", url, resp.StatusCode)
		})
	}

	taskErrCh := make(chan error)
	go func() {
		if err := pool.Wait(); err != nil {
			select {
			case taskErrCh <- errors.WithStack(err):
			case <-taskCtx.Done():
				return
			}
		}
	}()

	select {
	case resp := <-firstFinished:
		func() {
			defer resp.Body.Close()
			reqUrl := resp.Request.URL.String()
			logger := logger.With(zap.String("upstream", reqUrl))
			logger.Info("got response")

			ctx.Header("X-Ar-Io-Url", reqUrl)
			for k, v := range resp.Header {
				ctx.Header(k, v[0])
			}
			ctx.Status(resp.StatusCode)

			_, err := io.Copy(ctx.Writer, resp.Body)
			if web.AbortErr(ctx, errors.Wrap(err, "copy response")) {
				return
			}
		}()

		return
	case err := <-taskErrCh:
		if err == nil {
			err = errors.Errorf("this is an internal error, no error occurred in upstreams, please contact the admin")
		}

		web.AbortErr(ctx, errors.WithStack(err))
		return
	}
}
