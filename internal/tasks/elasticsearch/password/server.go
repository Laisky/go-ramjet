package password

import (
	"net/http"

	gconfig "github.com/Laisky/go-config/v2"
	utils "github.com/Laisky/go-utils/v6"
	"github.com/gin-gonic/gin"

	web "github.com/Laisky/go-ramjet/library/web"
)

func bindHTTP() {
	web.Server.GET("/es/password", func(ctx *gin.Context) {
		ctx.String(http.StatusOK,
			GeneratePasswdByDate(
				utils.Clock.GetUTCNow(),
				gconfig.Shared.GetString("tasks.elasticsearch-v2.password.secret")))
	})
}
