package http

import (
	"net/http"

	"github.com/Laisky/go-utils/v4/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/log"
)

// GetCurrentUser get current user
func GetCurrentUser(ctx *gin.Context) {
	user, err := getUserByAuthHeader(ctx)
	if AbortErr(ctx, err) {
		return
	}

	payload, err := json.Marshal(user)
	if AbortErr(ctx, err) {
		return
	}

	ctx.Data(200, "application/json", payload)
}

func GetCurrentUserQuota(ctx *gin.Context) {
	usertoken := ctx.Query("apikey")
	user, err := getUserByToken(ctx.Request.Context(), usertoken)
	if AbortErr(ctx, err) {
		return
	}

	externalBill, err := GetUserExternalBillingQuota(ctx.Request.Context(), user)
	if err != nil {
		log.Logger.Error("get user external billing quota", zap.Error(err))
	}

	// internalBill, err := GetUserInternalBill(ctx.Request.Context(), user, db.BillTypeTxt2Image)
	// if err != nil {
	// 	log.Logger.Error("get user internal billing quota", zap.Error(err))
	// }

	ctx.JSON(http.StatusOK, map[string]any{
		"external": externalBill,
	})
}
