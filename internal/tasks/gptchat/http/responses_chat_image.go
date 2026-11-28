package http

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/web"
)

// sendChatImageGeneration serves a chat request whose model is an image model:
// it generates images from the last user message, uploads them, and answers with
// Markdown image links through writeFinalToUI.
//
// Parameters:
//   - ctx: Gin context of the chat request.
//   - frontendReq: Parsed frontend request; Model and N select the images.
//   - user: Authenticated user of the request.
//
// A free user's token reservation is settled with the answer's tokens on success
// and always cleared from the request context.
//
// It returns the error that was also reported to the client, or nil on success.
func sendChatImageGeneration(ctx *gin.Context, frontendReq *FrontendReq, user *config.UserConfig) error {
	logger := gmw.GetLogger(ctx)
	// Like the text paths, the request-scoped reservation never outlives the
	// branch; a successful answer settles it below.
	defer clearTokenReservation(ctx)
	logger.Debug("routing image model request to image generation logic",
		zap.String("model", frontendReq.Model))

	// Extract prompt from last user message
	var prompt string
	for i := len(frontendReq.Messages) - 1; i >= 0; i-- {
		if frontendReq.Messages[i].Role == OpenaiMessageRoleUser {
			prompt = frontendReq.Messages[i].Content.String()
			break
		}
	}

	if prompt == "" {
		err := errors.New("prompt is empty for image generation")
		web.AbortErr(ctx, err)
		return errors.Wrap(err, "validate image prompt")
	}

	frontendReq.N = normalizeImageCount(frontendReq.N, user.IsFree)
	taskID := gutils.RandomStringWithLength(36)
	taskCtx, cancel := context.WithTimeout(gmw.Ctx(ctx), time.Minute*5)
	defer cancel()

	imgContents, err := generateImages(taskCtx, user, frontendReq.Model, prompt, frontendReq.N, "")
	if web.AbortErr(ctx, err) {
		return errors.Wrap(err, "generate images")
	}
	imgUrls, err := storeGeneratedImages(taskCtx, frontendReq.N, taskID, prompt, imgContents)
	if web.AbortErr(ctx, err) {
		return errors.Wrap(err, "store images")
	}

	var markdownText string
	for _, url := range imgUrls {
		markdownText += fmt.Sprintf("![Image](%s)\n\n", url)
	}

	finalText := strings.TrimSpace(markdownText)
	// Settle the free-tier token reservation with the actual output, as the text
	// paths do; otherwise the whole max_tokens estimate stays charged.
	if reservation := getTokenReservation(ctx); reservation != nil {
		if err := reservation.Finalize(gmw.Ctx(ctx), CountTextTokens(finalText)); err != nil {
			logger.Warn("finalize token reservation", zap.Error(err))
		}
	}

	return writeFinalToUI(ctx, frontendReq, nil, finalText, "", nil)
}
