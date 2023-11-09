package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/go-utils/v4/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/s3"
)

func ImageHandler(ctx *gin.Context) {
	taskID := gutils.RandomStringWithLength(36)
	logger := gmw.GetLogger(ctx).Named("image").With(
		zap.String("task_id", taskID),
	)
	gmw.SetLogger(ctx, logger)

	req := new(ImageHandlerRequest)
	if err := ctx.BindJSON(req); AbortErr(ctx, err) {
		return
	}

	user, err := getUserByAuthHeader(ctx)
	if AbortErr(ctx, err) {
		return
	}

	if err = user.IsModelAllowed(req.Model); AbortErr(ctx, err) {
		return
	}
	if err := checkUserExternalBilling(ctx.Request.Context(), user, db.PriceTxt2Image); AbortErr(ctx, err) {
		return
	}

	go func() {
		logger.Debug("start image drawing task")
		taskCtx, cancel := context.WithTimeout(ctx, time.Minute*3)
		defer cancel()

		switch user.ImageTokenType {
		case config.ImageTokenOpenai:
			err = drawImageByOpenaiDalle(taskCtx, user, req.Prompt, taskID)
		default:
			err = errors.Errorf("unknown image token type %s", user.ImageTokenType)
		}
		if err != nil {
			// upload error msg
			if _, err := s3.GetCli().PutObject(ctx,
				config.Config.S3.Bucket,
				imageObjkeyPrefix(taskID)+".err.txt",
				bytes.NewReader([]byte(err.Error())),
				int64(len(err.Error())),
				minio.PutObjectOptions{
					ContentType: "text/plain",
				}); err != nil {
				logger.Error("upload error msg", zap.Error(err))
			}

			logger.Error("failed to draw image", zap.Error(err))
			return
		}

		logger.Info("succeed draw image done")
	}()

	ctx.JSON(http.StatusOK, gin.H{
		"task_id": taskID,
		"image_url": fmt.Sprintf("https://%s/%s/%s",
			config.Config.S3.Endpoint,
			config.Config.S3.Bucket,
			imageObjkeyPrefix(taskID)+".png",
		),
	})
}

func drawImageByOpenaiDalle(ctx context.Context,
	user *config.UserConfig, prompt, taskID string) (err error) {
	logger := gmw.GetLogger(ctx).Named("openai")
	logger.Debug("draw image by openai dalle")

	reqBody, err := json.Marshal(NewOpenaiCreateImageRequest(prompt))
	if err != nil {
		return errors.Wrap(err, "marshal request body")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.openai.com/v1/images/generations", bytes.NewReader(reqBody))
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

	if err = uploadImage2Minio(ctx, taskID, prompt, imgContent); err != nil {
		return errors.Wrap(err, "upload image")
	}

	return nil
}

func imageObjkeyPrefix(taskid string) string {
	return fmt.Sprintf("create-images/%s/%s/%s", taskid[:2], taskid[2:4], taskid)
}

func uploadImage2Minio(ctx context.Context, taskid, prompt string, img_content []byte) (err error) {
	logger := gmw.GetLogger(ctx)
	s3cli := s3.GetCli()
	objkeyPrefix := imageObjkeyPrefix(taskid)

	var pool errgroup.Group

	// upload image
	pool.Go(func() error {
		_, err = s3cli.PutObject(ctx,
			config.Config.S3.Bucket,
			objkeyPrefix+".png",
			bytes.NewReader(img_content),
			int64(len(img_content)),
			minio.PutObjectOptions{
				ContentType: "image/png",
			},
		)
		return errors.Wrap(err, "upload image")
	})

	// upload prompt
	pool.Go(func() error {
		_, err = s3cli.PutObject(ctx,
			config.Config.S3.Bucket,
			objkeyPrefix+".txt",
			bytes.NewReader([]byte(prompt)),
			int64(len(prompt)),
			minio.PutObjectOptions{
				ContentType: "text/plain",
			})
		return errors.Wrap(err, "upload prompt")
	})

	if err := pool.Wait(); err != nil {
		return errors.Wrap(err, "upload image result to s3")
	}

	logger.Info("upload image to minio", zap.String("key", objkeyPrefix))
	return nil
}
