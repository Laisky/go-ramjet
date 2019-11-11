package heartbeat

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"

	ramjet "github.com/Laisky/go-ramjet"
)

func bindHTTP() {
	ramjet.Server.GET("/heartbeat", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, "heartbeat with %v active goroutines", runtime.NumGoroutine())
	})
}
