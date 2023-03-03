package web

import (
	"net/http"

	gmw "github.com/Laisky/gin-middlewares/v5"
	// gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/go-ramjet/library/log"
	glog "github.com/Laisky/go-utils/v4/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
)

var (
	Server *gin.Engine
)

func init() {
	Server = gin.New()
	Server.Use(
		gin.Recovery(),
		gmw.NewLoggerMiddleware(
			gmw.WithLoggerMwColored(),
			gmw.WithLevel(glog.LevelInfo),
			gmw.WithLogger(log.Logger),
		),
	)
}

func RunServer(addr string) {
	// if !gconfig.Shared.GetBool("debug") {
	// 	gin.SetMode(gin.ReleaseMode)
	// }

	// Server.Use(
	// 	gin.Recovery(),
	// 	gmw.NewLoggerMiddleware(
	// 		gmw.WithLoggerMwColored(),
	// 		gmw.WithLevel(glog.LevelInfo),
	// 		gmw.WithLogger(log.Logger),
	// 	),
	// )

	if err := gmw.EnableMetric(Server); err != nil {
		log.Logger.Panic("enable metrics", zap.Error(err))
	}

	Server.Any("/health", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, "hello, world")
	})

	log.Logger.Info("listening on http", zap.String("addr", addr))
	log.Logger.Panic("Server exit", zap.Error(Server.Run(addr)))
}
