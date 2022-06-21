package web

import (
	"net/http"

	"github.com/Laisky/go-ramjet/library/log"

	gmw "github.com/Laisky/gin-middlewares/v2"
	"github.com/Laisky/go-utils/v2"
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

	Server.Use(
		gin.Recovery(),
		gmw.GetLoggerMiddleware(log.Logger),
	)

	if err := gmw.EnableMetric(Server); err != nil {
		log.Logger.Panic("enable metrics", zap.Error(err))
	}

	Server.Any("/health", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, "hello, world")
	})

	log.Logger.Info("listening on http", zap.String("addr", addr))
	log.Logger.Panic("Server exit", zap.Error(Server.Run(addr)))
}
