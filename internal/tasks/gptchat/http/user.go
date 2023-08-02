package http

import (
	"github.com/Laisky/go-utils/v4/json"
	"github.com/gin-gonic/gin"
)

// GetCurrentUser get current user
func GetCurrentUser(ctx *gin.Context) {
	user, err := getUserFromToken(ctx)
	if AbortErr(ctx, err) {
		return
	}

	payload, err := json.Marshal(user)
	if AbortErr(ctx, err) {
		return
	}

	ctx.Data(200, "application/json", payload)
}
