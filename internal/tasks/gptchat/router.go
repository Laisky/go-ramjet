// Package gptchat implements gptchat tasks.
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

	grp := web.Server.Group("/gptchat")

	ihttp.RegisterStatic(grp.Group("/static"))
	grp.GET("/favicon.ico", func(ctx *gin.Context) {
		ctx.Header("Cache-Control", "max-age=86400")
		ctx.Data(http.StatusOK, "image/png", istatic.Favicon)
	})
	grp.Any("/api/v1/chat", ihttp.ChatHandler)
	grp.Any("/api/v1/images/generations", ihttp.ImageHandler)
	grp.GET("/user/me", ihttp.GetCurrentUser)
	grp.GET("/user/me/quota", ihttp.GetCurrentUserQuota)
	grp.Any("/ramjet/*any", ihttp.RamjetProxyHandler)
	grp.GET("/", ihttp.Chat)
}
