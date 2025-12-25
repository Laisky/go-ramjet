package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
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

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/s3"
	"github.com/Laisky/go-ramjet/library/web"
)

func DrawByLcmHandler(ctx *gin.Context) {
	taskID := gutils.RandomStringWithLength(36)
	logger := gmw.GetLogger(ctx).Named("image").With(
		zap.String("task_id", taskID),
	)
	gmw.SetLogger(ctx, logger)

	req := new(DrawImageByImageRequest)
	if err := ctx.BindJSON(req); web.AbortErr(ctx, err) {
		return
	}

	user, err := getUserByAuthHeader(ctx)
	if web.AbortErr(ctx, err) {
		return
	}

	const nSubTask = 2
	if err = IsModelAllowed(ctx, user, &FrontendReq{
		N:     nSubTask,
		Model: req.Model}); web.AbortErr(ctx, err) {
		return
	}

	for i := 0; i < nSubTask; i++ {
		i := i
		subtask := strconv.Itoa(i)
		go func() {
			time.Sleep(time.Second * time.Duration(i) * 3)
			logger.Debug("start image drawing task", zap.String("subtask", subtask))
			taskCtx, cancel := context.WithTimeout(ctx, time.Minute*5)
			defer cancel()

			if err := func() (err error) {
				upstreamReqBody, err := json.Marshal(DrawImageByLcmRequest{
					Data: [6]any{
						strings.TrimSpace(req.Prompt),
						"data:image/png;base64," + req.ImageBase64,
						4,
						1,
						0.9,
						1000 + rand.Intn(1000),
					},
					FnIndex: 1,
				})
				if err != nil {
					return errors.Wrap(err, "marshal request body")
				}

				upstreamReq, err := http.NewRequestWithContext(taskCtx, http.MethodPost,
					"http://100.92.237.35:7860/run/predict", bytes.NewReader(upstreamReqBody))
				if web.AbortErr(ctx, errors.Wrap(err, "new request")) {
					return
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

				respBody := new(DrawImageByLcmResponse)
				if err = json.NewDecoder(resp.Body).Decode(respBody); err != nil {
					return errors.Wrap(err, "decode response")
				}

				if len(respBody.Data) == 0 {
					return errors.New("empty response")
				}

				img, err := base64.StdEncoding.DecodeString(
					strings.TrimPrefix(respBody.Data[0], "data:image/png;base64,"))
				if err != nil {
					return errors.Wrap(err, "decode image")
				}

				return uploadImage2Minio(taskCtx,
					drawImageByImageObjkeyPrefix(taskID)+"-"+subtask,
					req.Prompt,
					img,
					".png",
				)
			}(); err != nil {
				// upload error msg
				msg := []byte(fmt.Sprintf("failed to draw image for %q, got %s", req.Prompt, err.Error()))
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

			logger.Info("succeed draw image done")
		}()
	}

	imageUrls := []string{}
	for i := 0; i < nSubTask; i++ {
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
