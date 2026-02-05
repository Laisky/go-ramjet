// Package web implements web server.
package web

import (
	"net/http"
	"strings"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v6"
	glog "github.com/Laisky/go-utils/v5/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/log"
)

var (
	Server *gin.Engine
)

func init() {
	Server = gin.New()

	Server.Use(
		gin.Recovery(),
		gmw.NewLoggerMiddleware(
			gmw.WithLoggerMwColored(),
			gmw.WithLevel(glog.LevelDebug.String()),
			gmw.WithLogger(log.Logger),
		),
		gmw.LockableMw(),
		securityMiddleware,
	)
}

func securityMiddleware(c *gin.Context) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "SAMEORIGIN")
	c.Header("Content-Security-Policy", "frame-ancestors 'self'")
	c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
	c.Next()
}

type normalizeHandler struct {
	handler http.Handler
}

func (h *normalizeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Nginx's proxy_pass might merge the prefix with the next component
	// if trailing slashes are mismatched.
	// e.g., /gptchat/version -> /gptchatversion
	if strings.HasPrefix(r.URL.Path, "/gptchat") &&
		!strings.HasPrefix(r.URL.Path, "/gptchat/") {
		r.URL.Path = "/gptchat/" + strings.TrimPrefix(r.URL.Path, "/gptchat")
	}

	// Nginx's proxy_pass might also duplicate the location prefix.
	// e.g., /gptchat/favicon.ico -> /gptchat/gptchat/favicon.ico
	// Normalize /{task}/{task}/... -> /{task}/...
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] != "" && parts[0] == parts[1] {
		if len(parts) == 2 {
			r.URL.Path = "/" + parts[0] + "/"
		} else {
			r.URL.Path = "/" + parts[0] + "/" + strings.Join(parts[2:], "/")
		}
	}
	h.handler.ServeHTTP(w, r)
}

func RunServer(addr string) {
	if err := gmw.EnableMetric(Server); err != nil {
		log.Logger.Panic("enable metrics", zap.Error(err))
	}

	Server.Any("/health", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, "hello, world")
	})

	// Register SPA if built assets exist.
	// This does not affect existing task routes, because the fallback only triggers on NoRoute.
	_ = TryRegisterDefaultSPA(Server)

	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      &normalizeHandler{handler: Server},
		ReadTimeout:  30 * time.Minute,
		WriteTimeout: 30 * time.Minute,
		IdleTimeout:  300 * time.Second,
	}

	log.Logger.Info("listening on http", zap.String("addr", addr))
	log.Logger.Panic("Server exit", zap.Error(httpSrv.ListenAndServe()))
}
