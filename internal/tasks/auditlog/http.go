package auditlog

import (
	"fmt"
	"net/http"

	glog "github.com/Laisky/go-utils/v4/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/web"
)

type router struct {
	logger glog.Logger
	svc    *Service
}

func newRouter(logger glog.Logger, svc *Service) *router {
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
}

func (r *router) abortErr(ctx *gin.Context, err error) bool {
	if err == nil {
		return false
	}

	r.logger.Error("http server abort", zap.Error(err))
	ctx.AbortWithStatusJSON(http.StatusBadRequest, fmt.Sprintf("%+v", err))
	return true
}

func (r *router) receiveLog(ctx *gin.Context) {
	var (
		err error
		log = new(Log)
	)
	err = ctx.BindJSON(log)
	if r.abortErr(ctx, err) {
		return
	}

	err = r.svc.SaveLog(ctx.Request.Context(), log)
	if r.abortErr(ctx, err) {
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"msg": "ok",
	})
}
