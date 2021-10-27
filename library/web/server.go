package web

import (
	"net/http"

	"github.com/Laisky/go-ramjet/library/log"

	"github.com/Laisky/gin-middlewares/metrics"
	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
)

var (
	Server = gin.New()
)

func RunServer(addr string) {
	if !utils.Settings.GetBool("debug") {
		gin.SetMode(gin.ReleaseMode)
	}

	Server.Use(gin.Recovery())
	metrics.Enable(Server)
	Server.Any("/health", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, "hello, world")
	})

	log.Logger.Info("listening on http", zap.String("addr", addr))
	log.Logger.Panic("Server exit", zap.Error(Server.Run(addr)))
}
