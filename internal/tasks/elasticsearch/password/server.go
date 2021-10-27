package password

import (
	"net/http"

	"github.com/gin-gonic/gin"

	utils "github.com/Laisky/go-utils"

	web "github.com/Laisky/go-ramjet/library/web"
)

func bindHTTP() {
	web.Server.GET("/es/password", func(ctx *gin.Context) {
		ctx.String(http.StatusOK,
			GeneratePasswdByDate(utils.Clock.GetUTCNow(), utils.Settings.GetString("tasks.elasticsearch-v2.password.secret")))
	})
}
