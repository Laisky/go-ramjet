package ario

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/web"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

var ArweaveGateways = []string{
	"https://permagate.io/",
	"https://ar-io.dev/",
	"https://vilenarios.com/",
	"https://arbr.pro/",
	"https://frostor.xyz/",
	"https://logosnodos.site/",
	"https://ariospeedwagon.com/",
	"https://vikanren.buzz/",
	"https://jaxtothehell.xyz/",
	"https://sulapan.com/",
	"https://arweave.fllstck.dev/",
	"https://yukovskibot.com/",
	"https://rerererararags.store/",
	"https://testnetnodes.xyz/",
	"https://budavlebac.online/",
	"https://karakartal.store/",
	"https://aleko0o.store/",
	"https://ruangnode.xyz/",
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

// GatewayHandler redirects request to multiple arweave gateways,
// and returns the first response.
func GatewayHandler(ctx *gin.Context) {
	fileKey := strings.Trim(ctx.Param("fileKey"), "/")
	domain := ctx.Query("domain")

	logger := gmw.GetLogger(ctx).With(
		zap.String("method", ctx.Request.Method),
		zap.String("fileKey", fileKey),
	)

	firstFinished := make(chan *http.Response)
	taskCtx, taskCancel := context.WithCancel(ctx.Request.Context())
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
			logger.Debug("fetching file", zap.String("target_url", url))
			req, err := http.NewRequestWithContext(taskCtx, ctx.Request.Method, url, ctx.Request.Body)
			if err != nil {
				return errors.Wrap(err, "new request")
			}

			req.Header = ctx.Request.Header
			resp, err := httpcli.Do(req)
			if err != nil {
				return errors.Wrap(err, "do request")
			}

			if resp.StatusCode == http.StatusOK {
				select {
				case firstFinished <- resp:
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
			taskErrCh <- err
		}
	}()

	select {
	case resp := <-firstFinished:
		func() {
			defer resp.Body.Close()
			reqUrl := resp.Request.URL.RequestURI()
			logger := logger.With(zap.String("upstream", reqUrl))
			logger.Info("got response")

			ctx.Header("X-Ar-Io-Url", reqUrl)
			for k, v := range resp.Header {
				ctx.Header(k, v[0])
			}
			ctx.Status(resp.StatusCode)

			buf := make([]byte, 4*1024*1024) // 4MB buffer
			for {
				n, err := resp.Body.Read(buf)
				if err != nil {
					if err == io.EOF {
						break
					}

					logger.Debug("read chunk", zap.Error(err))
					web.AbortErr(ctx, errors.Wrap(err, "read chunk"))
					return
				}

				if n > 0 {
					if _, writeErr := ctx.Writer.Write(buf[:n]); writeErr != nil {
						web.AbortErr(ctx, errors.Wrap(writeErr, "write chunk"))
						return
					}
				}
			}

			taskCancel()
		}()
	case err := <-taskErrCh:
		web.AbortErr(ctx, err)
		return
	}
}
