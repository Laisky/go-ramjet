package ramjet

import (
	"net/http"

	ginMiddlewares "github.com/Laisky/go-utils/gin-middlewares"

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
	ginMiddlewares.EnableMetric(Server)
	Server.Any("/health", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, "hello, world")
	})

	utils.Logger.Info("listening on http", zap.String("addr", addr))
	utils.Logger.Panic("Server exit", zap.Error(Server.Run(addr)))
}
