package password

import (
	"time"

	ramjet "github.com/Laisky/go-ramjet"
	utils "github.com/Laisky/go-utils"
	"github.com/kataras/iris"
)

func bindHTTP() {
	ramjet.Server.Get("/es/password", func(ctx iris.Context) {
		ctx.WriteString(GeneratePasswdByDate(time.Now(),
			utils.Settings.GetString("tasks.elasticsearch-v2.password.secret")))
	})
}
