// Package gptchat implements gptchat tasks.
package gptchat

import (
	"net/http"

	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v76"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
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
	grp.POST("/audit/conservation", ihttp.SaveLlmConservationHandler)
	grp.Any("/api", ihttp.ChatHandler)
	grp.POST("/images/generations", ihttp.DrawByDalleHandler)
	grp.POST("/images/generations/lcm", ihttp.DrawByLcmHandler)
	grp.POST("/images/generations/sdxl-turbo", ihttp.DrawBySdxlturboHandler)
	grp.GET("/user/me", ihttp.GetCurrentUser)
	grp.GET("/user/me/quota", ihttp.GetCurrentUserQuota)
	grp.Any("/ramjet/*any", ihttp.RamjetProxyHandler)
	grp.GET("/", ihttp.Chat)

	// payment
	stripe.Key = config.Config.PaymentStripeKey
	grp.POST("/create-payment-intent", ihttp.PaymentHandler)
	grp.GET("/payment/:ext", ihttp.PaymentStaticHandler)
}
