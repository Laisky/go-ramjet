package password

import (
	"net/http"

	"github.com/gin-gonic/gin"

	web "github.com/Laisky/go-ramjet/web"
	utils "github.com/Laisky/go-utils"
)

func bindHTTP() {
	web.Server.GET("/es/password", func(ctx *gin.Context) {
		ctx.String(http.StatusOK,
			GeneratePasswdByDate(utils.Clock.GetUTCNow(), utils.Settings.GetString("tasks.elasticsearch-v2.password.secret")))
	})
}
