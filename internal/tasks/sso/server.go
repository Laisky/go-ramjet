// Package gptchat implements gptchat tasks.
package sso

// import (
// 	"net/http"

// 	"github.com/Laisky/zap"
// 	"github.com/gin-gonic/gin"

// 	ihttp "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
// 	istatic "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates/static"
// 	"github.com/Laisky/go-ramjet/library/log"
// 	"github.com/Laisky/go-ramjet/library/web"
// )

// func bindHTTP() {
// 	// FIXME not implemented
// 	if err := ihttp.SetupHTTPCli(); err != nil {
// 		log.Logger.Panic("setup http client", zap.Error(err))
// 	}

// 	ihttp.RegisterStatic(web.Server.Group("/static"))
// 	web.Server.GET("/favicon.ico", func(ctx *gin.Context) {
// 		ctx.Header("Cache-Control", "max-age=86400")
// 		ctx.Data(http.StatusOK, "image/png", istatic.Favicon)
// 	})
// 	web.Server.Any("/api/", ihttp.APIHandler)
// 	web.Server.GET("/", ihttp.Chat)
// }
