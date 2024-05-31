package ario

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	"github.com/Laisky/go-ramjet/library/log"
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
	httpcli, err = gutils.NewHTTPClient()
	if err != nil {
		log.Logger.Panic("new http client", zap.Error(err))
	}
}

// GatewayHandler redirect request to multiple arweave gateways,
// and return the first response.
func GatewayHandler(ctx *gin.Context) {
	fileKey := strings.Trim(ctx.Param("fileKey"), "/")
	logger := gmw.GetLogger(ctx).With(
		zap.String("method", ctx.Request.Method),
		zap.String("fileKey", fileKey),
	)

	firstFinished := make(chan struct{}, 1)
	taskCtx, taskCancel := context.WithCancel(ctx.Request.Context())
	defer taskCancel()

	var pool errgroup.Group
	for _, gw := range ArweaveGateways {
		url := gw + fileKey
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
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				select {
				case <-taskCtx.Done():
				default:
					select {
					case firstFinished <- struct{}{}:
						logger.Info("got response", zap.String("url", url))
						ctx.Header("X-Ar-Io-Url", url)
						for k, v := range resp.Header {
							ctx.Header(k, v[0])
						}
						ctx.Status(resp.StatusCode)
						if _, err := io.Copy(ctx.Writer, resp.Body); err != nil {
							log.Logger.Error("copy response", zap.Error(err))
						}
						taskCancel()
					default:
					}
				}

				return nil
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return errors.Wrap(err, "read body")
			}
			return errors.Errorf("request %q, got [%d]%s", url, resp.StatusCode, string(body))
		})
	}

	go func() {
		if err := pool.Wait(); err != nil {
			logger.Error("failed to fetch file", zap.Error(err))
		}

		taskCancel()
	}()

	<-taskCtx.Done()
	select {
	case <-firstFinished:
	default:
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "all gateways are down",
		})
	}
}
