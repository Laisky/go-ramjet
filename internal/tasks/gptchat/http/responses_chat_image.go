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
	"golang.org/x/sync/errgroup"

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
// It returns the error that was also reported to the client, or nil on success.
func sendChatImageGeneration(ctx *gin.Context, frontendReq *FrontendReq, user *config.UserConfig) error {
	logger := gmw.GetLogger(ctx)
	var err error
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
		return err
	}

	if user.EnableExternalImageBilling {
		if err := checkUserExternalBilling(gmw.Ctx(ctx),
			user, GetImageModelPrice(frontendReq.Model), "txt2image"); web.AbortErr(ctx, err) {
			return err
		}
	}

	taskID := gutils.RandomStringWithLength(36)
	taskCtx, cancel := context.WithTimeout(gmw.Ctx(ctx), time.Minute*5)
	defer cancel()

	frontendReq.N = normalizeImageCount(frontendReq.N, user.IsFree)

	var imgContents [][]byte
	switch {
	case strings.Contains(user.ImageUrl, "openai.azure.com"):
		var pool errgroup.Group
		imgContents = make([][]byte, frontendReq.N)
		for i := range frontendReq.N {
			i := i
			pool.Go(func() (err error) {
				imgContents[i], err = fetchImageFromAzureDalle(taskCtx, user, prompt)
				return err
			})
		}
		if err := pool.Wait(); web.AbortErr(ctx, err) {
			return err
		}
	default:
		imgContents, err = fetchImageFromOpenaiDalle(taskCtx, user, frontendReq.Model, prompt, frontendReq.N, "")
		if web.AbortErr(ctx, err) {
			return err
		}
	}

	var pool errgroup.Group
	for i, imgContent := range imgContents {
		i, imgContent := i, imgContent
		pool.Go(func() error {
			return uploadImage2Minio(taskCtx,
				fmt.Sprintf("%s-%d", drawImageByTxtObjkeyPrefix(taskID), i),
				prompt,
				imgContent,
				".png",
			)
		})
	}

	if err := pool.Wait(); web.AbortErr(ctx, err) {
		return err
	}

	var markdownText string
	for i := range imgContents {
		url := fmt.Sprintf("https://%s/%s/%s-%d.%s",
			config.Config.S3.Endpoint,
			config.Config.S3.Bucket,
			drawImageByTxtObjkeyPrefix(taskID), i, "png",
		)
		markdownText += fmt.Sprintf("![Image](%s)\n\n", url)
	}

	return writeFinalToUI(ctx, frontendReq, nil, strings.TrimSpace(markdownText), "", nil)
}
