package auditlog

import (
	"context"
	"net/http"
	"time"

	glog "github.com/Laisky/go-utils/v4/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/web"
)

type router struct {
	logger glog.Logger
	svc    *service
}

func newRouter(logger glog.Logger, svc *service) *router {
	r := &router{
		logger: logger,
		svc:    svc,
	}
	r.bindHTTP()
	return r
}

func (r *router) bindHTTP() {
	grp := web.Server.Group("/auditlog")
	grp.POST("/log", r.receiveLog)
	grp.GET("/log", r.listLogs)
	grp.POST("/normal-log", r.receiveNormalLog)
	grp.GET("/normal-log", r.listNormalLogs)
}

func (r *router) abortErr(ctx *gin.Context, err error) bool {
	if err == nil {
		return false
	}

	r.logger.Error("http server abort", zap.Error(err))
	ctx.AbortWithStatusJSON(http.StatusBadRequest, map[string]any{
		"error": err.Error(),
	})
	return true
}

func (r *router) receiveLog(ctx *gin.Context) {
	log := new(Log)
	if err := ctx.BindJSON(log); r.abortErr(ctx, err) {
		return
	}

	// notice: use longlived background context,
	// 	   so that the request will not be aborted to avoid data loss
	// 	   when the client disconnects.
	ctxSave, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	if err := r.svc.SaveLog(ctxSave, log); r.abortErr(ctx, err) {
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"msg": "ok",
	})
}

func (r *router) listLogs(ctx *gin.Context) {
	logs, err := r.svc.ListLogs(ctx.Request.Context())
	if r.abortErr(ctx, err) {
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"msg":  "ok",
		"logs": logs,
	})
}

func (r *router) receiveNormalLog(ctx *gin.Context) {
	log := map[string]any{}
	if err := ctx.BindJSON(&log); r.abortErr(ctx, err) {
		return
	}

	delete(log, "_id")
	if err := r.svc.SaveNormalLog(ctx.Request.Context(), log); r.abortErr(ctx, err) {
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"msg": "ok",
	})
}

func (r *router) listNormalLogs(ctx *gin.Context) {
	logs, err := r.svc.ListNormalLogs(ctx.Request.Context())
	if r.abortErr(ctx, err) {
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"msg":  "ok",
		"logs": logs,
	})
}
