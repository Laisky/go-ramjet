package crawler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/web"
)

func registerWeb(svc *Service) {
	web.Server.GET("/crawler/search", func(ctx *gin.Context) {
		q := strings.TrimSpace(ctx.Query("q"))
		if q == "" {
			ctx.JSON(http.StatusBadRequest, nil)
			return
		}

		rets, err := svc.Search(ctx.Request.Context(), q)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, err)
			return
		}

		ctx.JSON(http.StatusOK, rets)
	})
}
