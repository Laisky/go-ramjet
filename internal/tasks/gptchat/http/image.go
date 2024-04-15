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

	if err = user.IsModelAllowed(ctx, req.Model, 0); AbortErr(ctx, err) {
		return
	}

	const nSubTask = 2
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
					drawImageByImageObjkeyPrefix(taskID)+"-"+subtask, req.Prompt, img)
			}(); err != nil {
				// upload error msg
				msg := []byte(fmt.Sprintf("failed to draw image for %q, got %s", req.Prompt, err.Error()))
				objkey := drawImageByImageObjkeyPrefix(taskID) + ".err.txt"
				if _, err := s3.GetCli().PutObject(taskCtx,
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

func DrawBySdxlturboHandler(ctx *gin.Context) {
	taskID := gutils.RandomStringWithLength(36)

	req := new(DrawImageBySdxlturboRequest)
	if err := ctx.BindJSON(req); AbortErr(ctx, err) {
		return
	}

	logger := gmw.GetLogger(ctx).Named("image").With(
		zap.String("task_id", taskID),
		zap.Int("n", req.N),
	)
	gmw.SetLogger(ctx, logger)

	user, err := getUserByAuthHeader(ctx)
	if AbortErr(ctx, err) {
		return
	}

	if err = user.IsModelAllowed(ctx, req.Model, 0); AbortErr(ctx, err) {
		return
	}

	req.N = 2
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

			resp, err := httpcli.Do(upstreamReq) //nolint: bodyclose
			if err != nil {
				return errors.Wrap(err, "do request")
			}
			defer gutils.LogErr(resp.Body.Close, logger)

			if resp.StatusCode != http.StatusOK {
				payload, _ := io.ReadAll(resp.Body)
				return errors.Errorf("bad status code [%d]%s", resp.StatusCode, string(payload))
			}

			resoData := new(DrawImageBySdxlturboResponse)
			if err = json.NewDecoder(resp.Body).Decode(resoData); err != nil {
				return errors.Wrap(err, "decode response")
			}

			var pool errgroup.Group
			for i, img := range resoData.B64Images {
				subtask := strconv.Itoa(i)
				imgBytes, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(img, "data:image/png;base64,"))
				if err != nil {
					return errors.Wrap(err, "decode image")
				}

				pool.Go(func() error {
					return uploadImage2Minio(taskCtx,
						drawImageByImageObjkeyPrefix(taskID)+"-"+subtask, req.Text, imgBytes)
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
			if _, err := s3.GetCli().PutObject(taskCtx,
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

func DrawByDalleHandler(ctx *gin.Context) {
	taskID := gutils.RandomStringWithLength(36)

	req := new(DrawImageByTextRequest)
	if err := ctx.BindJSON(req); AbortErr(ctx, err) {
		return
	}

	logger := gmw.GetLogger(ctx).Named("image").With(
		zap.String("task_id", taskID),
	)
	gmw.SetLogger(ctx, logger)

	user, err := getUserByAuthHeader(ctx)
	if AbortErr(ctx, err) {
		return
	}

	if err = user.IsModelAllowed(ctx, req.Model, 0); AbortErr(ctx, err) {
		return
	}
	if err := checkUserExternalBilling(gmw.Ctx(ctx),
		user, db.PriceTxt2Image, "txt2image"); AbortErr(ctx, err) {
		return
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
			if _, err := s3.GetCli().PutObject(taskCtx,
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

	if err = uploadImage2Minio(ctx, drawImageByTxtObjkeyPrefix(taskID)+"-0", prompt, imgContent); err != nil {
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

	if err = uploadImage2Minio(ctx, drawImageByTxtObjkeyPrefix(taskID)+"-0", prompt, imgContent); err != nil {
		return errors.Wrap(err, "upload image")
	}

	return nil
}

func drawImageByTxtObjkeyPrefix(taskid string) string {
	year := time.Now().Format("2006")
	month := time.Now().Format("01")
	return fmt.Sprintf("create-images/%s/%s/%s", year, month, taskid)
}

func drawImageByImageObjkeyPrefix(taskid string) string {
	year := time.Now().Format("2006")
	month := time.Now().Format("01")
	return fmt.Sprintf("image-by-image/%s/%s/%s", year, month, taskid)
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
