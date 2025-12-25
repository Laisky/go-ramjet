package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/go-utils/v5/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/s3"
	"github.com/Laisky/go-ramjet/library/web"
)

func DrawBySdxlturboHandlerByNvidia(ctx *gin.Context) {
	rawreq := new(DrawImageBySdxlturboRequest)
	if err := ctx.BindJSON(rawreq); web.AbortErr(ctx, err) {
		return
	}

	user, err := getUserByAuthHeader(ctx)
	if web.AbortErr(ctx, err) {
		return
	}

	if err = IsModelAllowed(ctx, user, &FrontendReq{
		N:     1,
		Model: rawreq.Model}); web.AbortErr(ctx, err) {
		return
	}

	taskID := gutils.RandomStringWithLength(36)
	objkeyPrefix := drawImageByTxtObjkeyPrefix(taskID)

	logger := gmw.GetLogger(ctx).Named("nvidia_sdxl_turbo").With(
		zap.String("task_id", taskID),
	)
	gmw.SetLogger(ctx, logger)

	go func() {
		logger.Debug("start image drawing task")
		err := func() error {
			taskCtx, cancel := context.WithTimeout(ctx, time.Minute*5)
			defer cancel()

			nvreq := NewNvidiaDrawImageBySdxlturboRequest(rawreq.Text)

			upstreamReqBody, err := json.Marshal(nvreq)
			if err != nil {
				return errors.Wrap(err, "marshal request body")
			}

			upstreamReq, err := http.NewRequestWithContext(taskCtx, http.MethodPost,
				"https://ai.api.nvidia.com/v1/genai/stabilityai/sdxl-turbo",
				bytes.NewReader(upstreamReqBody))
			if err != nil {
				return errors.Wrap(err, "new request to nvidia")
			}

			upstreamReq.Header.Add("Content-Type", "application/json")
			upstreamReq.Header.Set("Authorization", "Bearer "+config.Config.NvidiaApikey)

			resp, err := httpcli.Do(upstreamReq) //nolint: bodyclose
			if err != nil {
				return errors.Wrap(err, "do request")
			}
			defer gutils.LogErr(resp.Body.Close, logger)

			if resp.StatusCode != http.StatusOK {
				payload, _ := io.ReadAll(resp.Body)
				return errors.Errorf("bad status code [%d]%s", resp.StatusCode, string(payload))
			}

			respData := new(NvidiaDrawImageBySdxlturboResponse)
			if err = json.NewDecoder(resp.Body).Decode(respData); err != nil {
				return errors.Wrap(err, "decode response")
			}

			if len(respData.Artifacts) == 0 {
				return errors.New("empty response")
			}

			imgcontent, err := base64.StdEncoding.DecodeString(respData.Artifacts[0].Base64)
			if err != nil {
				return errors.Wrap(err, "decode image")
			}

			logger.Debug("succeed get image from nvidia")
			err = uploadImage2Minio(
				taskCtx,
				objkeyPrefix+"-0",
				rawreq.Text,
				imgcontent,
				".png",
			)
			if err != nil {
				return errors.Wrap(err, "upload image")
			}

			logger.Info("succeed draw image done")
			return nil
		}()
		if err != nil {
			// upload error msg
			msg := []byte(fmt.Sprintf("failed to draw image for %q, got %s", rawreq.Text, err.Error()))
			objkey := objkeyPrefix + ".err.txt"
			s3cli, err := s3.GetCli()
			if err != nil {
				logger.Error("get s3 client", zap.Error(err))
			}

			if _, err := s3cli.PutObject(ctx,
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
	}()

	imageUrls := []string{}
	imageUrls = append(imageUrls,
		fmt.Sprintf("https://%s/%s/%s-0.%s",
			config.Config.S3.Endpoint,
			config.Config.S3.Bucket,
			objkeyPrefix, "png",
		),
	)

	ctx.JSON(http.StatusOK, gin.H{
		"task_id":    taskID,
		"image_urls": imageUrls,
	})
}

func DrawBySdxlturboHandlerBySelfHosted(ctx *gin.Context) {
	taskID := gutils.RandomStringWithLength(36)

	req := new(DrawImageBySdxlturboRequest)
	if err := ctx.BindJSON(req); web.AbortErr(ctx, err) {
		return
	}

	logger := gmw.GetLogger(ctx).Named("image").With(
		zap.String("task_id", taskID),
		zap.Int("n", req.N),
	)
	gmw.SetLogger(ctx, logger)

	user, err := getUserByAuthHeader(ctx)
	if web.AbortErr(ctx, err) {
		return
	}

	req.N = 2
	if err = IsModelAllowed(ctx, user, &FrontendReq{
		N:     req.N,
		Model: req.Model}); web.AbortErr(ctx, err) {
		return
	}

	go func() {
		// time.Sleep(time.Second * time.Duration(i))
		logger.Debug("start image drawing task")
		taskCtx, cancel := context.WithTimeout(ctx, time.Minute*5)
		defer cancel()

		if err := func() (err error) {
			upstreamReqBody, err := json.Marshal(req)
			if err != nil {
				return errors.Wrap(err, "marshal request body")
			}

			upstreamReq, err := http.NewRequestWithContext(taskCtx, http.MethodPost,
				"http://100.92.237.35:7861/predict", bytes.NewReader(upstreamReqBody))
			if err != nil {
				return errors.Wrap(err, "new request to upstream")
			}

			upstreamReq.Header.Add("Content-Type", "application/json")
			if config.Config.LcmBasicAuthUsername != "" {
				upstreamReq.SetBasicAuth(
					config.Config.LcmBasicAuthUsername,
					config.Config.LcmBasicAuthPassword,
				)
			}

			resp, err := httpcli.Do(upstreamReq) //nolint: bodyclose
			if err != nil {
				return errors.Wrap(err, "do request")
			}
			defer gutils.LogErr(resp.Body.Close, logger)

			if resp.StatusCode != http.StatusOK {
				payload, _ := io.ReadAll(resp.Body)
				return errors.Errorf("bad status code [%d]%s", resp.StatusCode, string(payload))
			}

			respData := new(DrawImageBySdxlturboResponse)
			if err = json.NewDecoder(resp.Body).Decode(respData); err != nil {
				return errors.Wrap(err, "decode response")
			}

			var pool errgroup.Group
			for i, img := range respData.B64Images {
				subtask := strconv.Itoa(i)
				imgBytes, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(img, "data:image/png;base64,"))
				if err != nil {
					return errors.Wrap(err, "decode image")
				}

				pool.Go(func() (err error) {
					return uploadImage2Minio(taskCtx,
						drawImageByImageObjkeyPrefix(taskID)+"-"+subtask,
						req.Text,
						imgBytes,
						".png",
					)
				})
			}

			if err := pool.Wait(); err != nil {
				return errors.Wrap(err, "upload image result to s3")
			}

			return nil
		}(); err != nil {
			// upload error msg
			msg := []byte(fmt.Sprintf("failed to draw image for %q, got %s", req.Text, err.Error()))
			objkey := drawImageByImageObjkeyPrefix(taskID) + ".err.txt"
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

		logger.Info("succeed draw one image")
	}()

	imageUrls := []string{}
	for i := 0; i < req.N; i++ {
		imageUrls = append(imageUrls, fmt.Sprintf("https://%s/%s/%s-%d.%s",
			config.Config.S3.Endpoint,
			config.Config.S3.Bucket,
			drawImageByImageObjkeyPrefix(taskID), i, "png",
		))
	}

	ctx.JSON(http.StatusOK, gin.H{
		"task_id":    taskID,
		"image_urls": imageUrls,
	})
}
