package gitlab

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	gmw "github.com/Laisky/gin-middlewares/v6"

	"github.com/Laisky/go-ramjet/library/web"
)

func registerWeb() {
	web.Server.GET("/gitlab/file", func(ctx *gin.Context) {
		file := strings.TrimSpace(ctx.Query("file"))
		if file == "" {
			ctx.JSON(http.StatusBadRequest, nil)
			return
		}

		rets, err := svc.GetFile(gmw.Ctx(ctx), file)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, fmt.Sprintf("%+v", err))
			return
		}

		ctx.JSON(http.StatusOK, rets)
	})
}
