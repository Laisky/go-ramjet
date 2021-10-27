package heartbeat

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/web"
)

func bindHTTP() {
	web.Server.GET("/heartbeat", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, "heartbeat with %v active goroutines", runtime.NumGoroutine())
	})
}
