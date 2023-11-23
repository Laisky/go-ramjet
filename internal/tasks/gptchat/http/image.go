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

func DrawByLcmHandler(ctx *gin.Context) {
	taskID := gutils.RandomStringWithLength(36)
	logger := gmw.GetLogger(ctx).Named("image").With(
		zap.String("task_id", taskID),
	)
	gmw.SetLogger(ctx, logger)

	req := new(DrawImageByImageRequest)
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

	// free
	// if err := checkUserExternalBilling(ctx.Request.Context(), user, db.PriceTxt2Image, "txt2image"); AbortErr(ctx, err) {
	// 	return
	// }

	upstreamReqBody, err := json.Marshal(DrawImageByLcmRequest{
		Data: [6]any{
			strings.TrimSpace(req.Prompt),
			"data:image/png;base64," + req.ImageBase64,
			4,
			1,
			0.9,
			1337,
		},
		FnIndex: 1,
	})
	if AbortErr(ctx, errors.Wrap(err, "marshal request body")) {
		return
	}

	go func() {
		logger.Debug("start image drawing task")
		taskCtx, cancel := context.WithTimeout(ctx, time.Minute*3)
		defer cancel()

		if err := func() (err error) {
			upstreamReq, err := http.NewRequestWithContext(taskCtx, http.MethodPost,
				"https://draw2.laisky.com/run/predict", bytes.NewReader(upstreamReqBody))
			if AbortErr(ctx, errors.Wrap(err, "new request")) {
				return
			}

			upstreamReq.Header.Add("Content-Type", "application/json")
			if config.Config.LcmBasicAuthUsername != "" {
				upstreamReq.SetBasicAuth(
					config.Config.LcmBasicAuthUsername,
					config.Config.LcmBasicAuthPassword,
				)
			}

			resp, err := httpcli.Do(upstreamReq)
			if err != nil {
				return errors.Wrap(err, "do request")
			}
			defer gutils.LogErr(resp.Body.Close, logger)

			if resp.StatusCode != http.StatusOK {
				payload, _ := io.ReadAll(resp.Body)
				return errors.Errorf("bad status code [%d]%s", resp.StatusCode, string(payload))
			}

			respBody := new(DrawImageByLcmResponse)
			if err = json.NewDecoder(resp.Body).Decode(respBody); err != nil {
				return errors.Wrap(err, "decode response")
			}

			if len(respBody.Data) == 0 {
				return errors.New("empty response")
			}

			img, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(respBody.Data[0], "data:image/png;base64,"))
			if err != nil {
				return errors.Wrap(err, "decode image")
			}

			return uploadImage2Minio(taskCtx, drawImageByTxtObjkeyPrefix(taskID), req.Prompt, img)
		}(); err != nil {
			// upload error msg
			if _, err := s3.GetCli().PutObject(taskCtx,
				config.Config.S3.Bucket,
				drawImageByTxtObjkeyPrefix(taskID)+".err.txt",
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
			drawImageByTxtObjkeyPrefix(taskID)+".png",
		),
	})
}

func DrawByDalleHandler(ctx *gin.Context) {
	taskID := gutils.RandomStringWithLength(36)
	logger := gmw.GetLogger(ctx).Named("image").With(
		zap.String("task_id", taskID),
	)
	gmw.SetLogger(ctx, logger)

	req := new(DrawImageByTextRequest)
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
	if err := checkUserExternalBilling(ctx.Request.Context(), user, db.PriceTxt2Image, "txt2image"); AbortErr(ctx, err) {
		return
	}

	go func() {
		logger.Debug("start image drawing task")
		taskCtx, cancel := context.WithTimeout(ctx, time.Minute*3)
		defer cancel()

		switch {
		case strings.Contains(user.ImageUrl, "openai.com"):
			err = drawImageByOpenaiDalle(taskCtx, user, req.Prompt, taskID)
		case strings.Contains(user.ImageUrl, "azure.com"):
			err = drawImageByAzureDalle(taskCtx, user, req.Prompt, taskID)
		default:
			err = errors.Errorf("unknown txt2image service url %s", user.ImageUrl)
		}

		if err != nil {
			// upload error msg
			if _, err := s3.GetCli().PutObject(taskCtx,
				config.Config.S3.Bucket,
				drawImageByTxtObjkeyPrefix(taskID)+".err.txt",
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
			drawImageByTxtObjkeyPrefix(taskID)+".png",
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

	if err = uploadImage2Minio(ctx, drawImageByTxtObjkeyPrefix(taskID), prompt, imgContent); err != nil {
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

	if err = uploadImage2Minio(ctx, drawImageByTxtObjkeyPrefix(taskID), prompt, imgContent); err != nil {
		return errors.Wrap(err, "upload image")
	}

	return nil
}

func drawImageByTxtObjkeyPrefix(taskid string) string {
	return fmt.Sprintf("create-images/%s/%s/%s", taskid[:2], taskid[2:4], taskid)
}

func drawImageByImageObjkeyPrefix(taskid string) string {
	return fmt.Sprintf("image-by-image/%s/%s/%s", taskid[:2], taskid[2:4], taskid)
}

func uploadImage2Minio(ctx context.Context, objkeyPrefix, prompt string, img_content []byte) (err error) {
	logger := gmw.GetLogger(ctx)
	s3cli := s3.GetCli()

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
