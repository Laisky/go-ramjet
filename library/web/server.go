// Package web implements web server.
package web

import (
	"net/http"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v5"
	glog "github.com/Laisky/go-utils/v4/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/log"
)

var (
	Server *gin.Engine
)

func init() {
	Server = gin.New()
	Server.RedirectTrailingSlash = false
	Server.Use(
		gin.Recovery(),
		gmw.NewLoggerMiddleware(
			gmw.WithLoggerMwColored(),
			gmw.WithLevel(glog.LevelInfo.String()),
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

	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      Server,
		ReadTimeout:  300 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  300 * time.Second,
	}

	log.Logger.Info("listening on http", zap.String("addr", addr))
	log.Logger.Panic("Server exit", zap.Error(httpSrv.ListenAndServe()))
}
