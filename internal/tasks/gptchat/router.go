// Package gptchat implements gptchat tasks.
package gptchat

import (
	"context"
	"net/http"
	"sync"

	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/stripe/stripe-go/v76"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	ihttp "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
	istatic "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates/static"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/web"
)

var (
	once              sync.Once
	globalRatelimiter *gutils.RateLimiter
)

func setupInit() {
	once.Do(func() {
		var err error
		if globalRatelimiter, err = gutils.NewRateLimiter(context.Background(),
			gutils.RateLimiterArgs{
				Max:     100,
				NPerSec: 10,
			}); err != nil {
			log.Logger.Panic("new ratelimiter", zap.Error(err))
		}
	})
}

func globalRatelimitMw(ctx *gin.Context) {
	setupInit()

	if !globalRatelimiter.Allow() {
		web.AbortErr(ctx, errors.New("global rate limit"))
	}

	ctx.Next()
}

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

	grp.GET("/", ihttp.Chat)
	apiWithRatelimiter := grp.Group("", globalRatelimitMw)
	apiWithRatelimiter.POST("/audit/conservation", ihttp.SaveLlmConservationHandler)
	apiWithRatelimiter.Any("/api", ihttp.ChatHandler)
	apiWithRatelimiter.POST("/images/generations", ihttp.DrawByDalleHandler)
	apiWithRatelimiter.POST("/images/generations/lcm", ihttp.DrawByLcmHandler)
	apiWithRatelimiter.POST("/images/generations/flux/:model", ihttp.DrawByFlux)
	apiWithRatelimiter.POST("/images/edit/flux/:model", ihttp.InpaitingByFlux)
	apiWithRatelimiter.POST("/images/generations/sdxl-turbo", ihttp.DrawBySdxlturboHandlerByNvidia)
	apiWithRatelimiter.POST("/chat/oneshot", ihttp.OneShotChatHandler)
	apiWithRatelimiter.POST("/files/chat", ihttp.UploadFiles)
	apiWithRatelimiter.GET("/audio/tts", ihttp.TTSHanler)
	grp.GET("/user/me", ihttp.GetCurrentUser)
	// grp.GET("/user/me/quota", ihttp.GetCurrentUserQuota)
	apiWithRatelimiter.POST("/user/config", ihttp.UploadUserConfig)
	grp.GET("/user/config", ihttp.DownloadUserConfig)
	apiWithRatelimiter.Any("/ramjet/*any", ihttp.RamjetProxyHandler)
	grp.Any("/oneapi/*any", ihttp.OneapiProxyHandler)
	grp.GET("/version", func(ctx *gin.Context) {
		ctx.Data(http.StatusOK, gutils.HTTPHeaderContentTypeValJSON, []byte(gutils.PrettyBuildInfo()))
	})

	// payment
	stripe.Key = config.Config.PaymentStripeKey
	grp.POST("/create-payment-intent", ihttp.PaymentHandler)
	grp.GET("/payment/:ext", ihttp.PaymentStaticHandler)
}
