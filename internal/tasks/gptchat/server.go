package gptchat

import (
	"net/http"

	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	ihttp "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
	istatic "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates/static"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/web"
)

func bindHTTP() {
	if err := ihttp.SetupHTTPCli(); err != nil {
		log.Logger.Panic("setup http client", zap.Error(err))
	}

	ihttp.RegisterStatic(web.Server.Group("/static"))
	web.Server.GET("/favicon.ico", func(ctx *gin.Context) {
		ctx.Data(http.StatusOK, "application/javascript", istatic.Favicon)
	})
	web.Server.Any("/api/", ihttp.APIHandler)
	web.Server.GET("/", ihttp.Chat)
}
