package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/go-utils/v5/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/s3"
	"github.com/Laisky/go-ramjet/library/web"
)

func DrawByDalleHandler(ctx *gin.Context) {
	taskID := gutils.RandomStringWithLength(36)

	req := new(DrawImageByTextRequest)
	if err := ctx.BindJSON(req); web.AbortErr(ctx, err) {
		return
	}

	logger := gmw.GetLogger(ctx).Named("image").With(
		zap.String("task_id", taskID),
	)
	gmw.SetLogger(ctx, logger)

	user, err := getUserByAuthHeader(ctx)
	if web.AbortErr(ctx, err) {
		return
	}

	if err = IsModelAllowed(ctx, user,
		&FrontendReq{
			N:     1,
			Model: req.Model,
		}); web.AbortErr(ctx, err) {
		return
	}

	if user.EnableExternalImageBilling {
		if err := checkUserExternalBilling(gmw.Ctx(ctx),
			user, db.PriceTxt2Image, "txt2image"); web.AbortErr(ctx, err) {
			return
		}
	}

	go func() {
		logger.Debug("start image drawing task")
		taskCtx, cancel := context.WithTimeout(ctx, time.Minute*5)
		defer cancel()

		switch {
		case strings.Contains(user.ImageUrl, "openai.azure.com"):
			err = drawImageByAzureDalle(taskCtx, user, req.Prompt, taskID)
		default:
			err = drawImageByOpenaiDalle(taskCtx, user, req.Prompt, taskID)
		}

		if err != nil {
			// upload error msg
			msg := []byte(fmt.Sprintf("failed to draw image for %q, got %s", req.Prompt, err.Error()))
			objkey := drawImageByTxtObjkeyPrefix(taskID) + ".err.txt"
			s3cli, err := s3.GetCli()
			if err != nil {
				logger.Error("get s3 client", zap.Error(err))
			}

			if _, err := s3cli.PutObject(taskCtx,
				config.Config.S3.Bucket,
				objkey,
				bytes.NewReader(msg),
				int64(len(msg)),
				minio.PutObjectOptions{
					ContentType: "text/plain",
				}); err != nil {
				logger.Error("upload error msg", zap.Error(err))
			}

			logger.Error("failed to draw image", zap.Error(err), zap.String("objkey", objkey))
			return
		}

		logger.Info("succeed draw image done")
	}()

	ctx.JSON(http.StatusOK, gin.H{
		"task_id": taskID,
		"image_urls": []string{
			fmt.Sprintf("https://%s/%s/%s-0.%s",
				config.Config.S3.Endpoint,
				config.Config.S3.Bucket,
				drawImageByTxtObjkeyPrefix(taskID), "png",
			),
		},
	})
}

func drawImageByOpenaiDalle(ctx context.Context,
	user *config.UserConfig, prompt, taskID string) (err error) {
	logger := gmw.GetLogger(ctx).Named("openai")
	logger.Debug("draw image by openai dalle", zap.String("img_url", user.ImageUrl))

	reqBody, err := json.Marshal(NewOpenaiCreateImageRequest(prompt))
	if err != nil {
		return errors.Wrap(err, "marshal request body")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		user.ImageUrl, bytes.NewReader(reqBody))
	if err != nil {
		return errors.Wrap(err, "new request")
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+user.ImageToken)

	resp := new(OpenaiCreateImageResponse)
	if httpresp, err := httpcli.Do(req); err != nil { //nolint: bodyclose
		return errors.Wrap(err, "do request")
	} else {
		defer gutils.LogErr(httpresp.Body.Close, logger)
		if httpresp.StatusCode != http.StatusOK {
			payload, _ := io.ReadAll(req.Body)
			return errors.Errorf("bad status code [%d]%s", httpresp.StatusCode, string(payload))
		}

		if err = json.NewDecoder(httpresp.Body).Decode(resp); err != nil {
			return errors.Wrap(err, "decode response")
		}

		if len(resp.Data) == 0 {
			return errors.New("empty response")
		}
	}

	logger.Debug("succeed get image from openai")
	imgContent, err := base64.StdEncoding.DecodeString(resp.Data[0].B64Json)
	if err != nil {
		return errors.Wrap(err, "decode image")
	}

	if err = uploadImage2Minio(ctx,
		drawImageByTxtObjkeyPrefix(taskID)+"-0",
		prompt,
		imgContent,
		".png",
	); err != nil {
		return errors.Wrap(err, "upload image")
	}

	return nil
}

func drawImageByAzureDalle(ctx context.Context,
	user *config.UserConfig, prompt, taskID string) (err error) {
	logger := gmw.GetLogger(ctx).Named("azure")
	logger.Debug("draw image by azure dalle")

	reqBody, err := json.Marshal(OpenaiCreateImageRequest{
		Prompt: prompt,
		Size:   "1024x1024",
		N:      1,
	})
	if err != nil {
		return errors.Wrap(err, "marshal request body")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		user.ImageUrl, bytes.NewReader(reqBody))
	if err != nil {
		return errors.Wrap(err, "new request")
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Api-Key", user.ImageToken)

	resp := new(AzureCreateImageResponse)
	if httpresp, err := httpcli.Do(req); err != nil { //nolint: bodyclose
		return errors.Wrap(err, "do request")
	} else {
		defer gutils.LogErr(httpresp.Body.Close, logger)
		if httpresp.StatusCode != http.StatusOK {
			payload, _ := io.ReadAll(req.Body)
			return errors.Errorf("bad status code [%d]%s", httpresp.StatusCode, string(payload))
		}

		if err = json.NewDecoder(httpresp.Body).Decode(resp); err != nil {
			return errors.Wrap(err, "decode response")
		}

		if len(resp.Data) == 0 {
			return errors.New("empty response")
		}
	}

	// download image
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, resp.Data[0].Url, nil)
	if err != nil {
		return errors.Wrap(err, "new request")
	}
	imgResp, err := httpcli.Do(req) //nolint: bodyclose
	if err != nil {
		return errors.Wrap(err, "download azure image")
	}
	defer gutils.LogErr(imgResp.Body.Close, logger)

	if imgResp.StatusCode != http.StatusOK {
		return errors.Errorf("download azure image got bad status code %d", imgResp.StatusCode)
	}

	logger.Debug("succeed get image from azure")
	imgContent, err := io.ReadAll(imgResp.Body)
	if err != nil {
		return errors.Wrap(err, "read image")
	}

	if err = uploadImage2Minio(ctx,
		drawImageByTxtObjkeyPrefix(taskID)+"-0",
		prompt,
		imgContent,
		".png",
	); err != nil {
		return errors.Wrap(err, "upload image")
	}

	return nil
}
