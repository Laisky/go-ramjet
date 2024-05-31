package web

import (
	"net/http"

	gmw "github.com/Laisky/gin-middlewares/v5"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
)

// AbortErr abort with error
func AbortErr(ctx *gin.Context, err error) bool {
	if err == nil {
		return false
	}

	gmw.GetLogger(ctx).Error("chat abort", zap.Error(err))
	ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
		"err": err.Error(),
	})
	return true
}
