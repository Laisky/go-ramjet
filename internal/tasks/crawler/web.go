package crawler

import (
	"net/http"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/web"
)

func registerWeb(svc *Service) {
	web.Server.GET("/crawler/search", func(ctx *gin.Context) {
		q := strings.TrimSpace(ctx.Query("q"))
		if q == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "empty query"})
			return
		}
		if len(q) > 500 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "query too long"})
			return
		}

		rets, err := svc.Search(gmw.Ctx(ctx), q)
		if err != nil {
			log.Logger.Error("crawler search failed", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "search failed"})
			return
		}

		ctx.JSON(http.StatusOK, rets)
	})
}
