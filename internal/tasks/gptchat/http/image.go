package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/go-utils/v4/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"
	"github.com/minio/minio-go/v7"
	"golang.org/x/image/webp"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/s3"
	"github.com/Laisky/go-ramjet/library/web"
)

// DrawByFlux draw image by flux-pro
func DrawByFlux(ctx *gin.Context) {
	model := strings.TrimSpace(ctx.Param("model"))
	if model == "" {
		web.AbortErr(ctx, errors.New("empty model"))
	}

	var price db.Price
	imgExt := ".png"
	nImage := 1
	switch model {
	case "flux-pro":
		price = db.PriceTxt2ImageFluxPro
	case "flux-schnell":
		nImage = 4
		price = db.PriceTxt2ImageSchnell
	default:
		web.AbortErr(ctx, errors.Errorf("unknown model %q", model))
		return
	}

	taskID := gutils.RandomStringWithLength(36)
	logger := gmw.GetLogger(ctx).Named("image_flux").With(
		zap.String("task_id", taskID),
		zap.String("model", model),
	)
	gmw.SetLogger(ctx, logger)

	req := new(DrawImageByFluxReplicateRequest)
	if err := ctx.BindJSON(req); web.AbortErr(ctx, err) {
		return
	}

	user, err := getUserByAuthHeader(ctx)
	if web.AbortErr(ctx, err) {
		return
	}

	if err = user.IsModelAllowed(ctx, model, 0); web.AbortErr(ctx, err) {
		return
	}

	go func() {
		logger.Debug("start image drawing task")
		taskCtx, cancel := context.WithTimeout(ctx, time.Minute*5)
		defer cancel()

		var pool errgroup.Group
		anySucceed := int32(0)
		for i := range nImage {
			i := i

			pool.Go(func() (err error) {
				logger := logger.With(zap.Int("n_img", i))
				taskCtx := gmw.SetLogger(taskCtx, logger)

				// first try segmind, since of segmind is free
				imgContent, err := drawFluxBySegmind(taskCtx, model, req)
				if err != nil {
					logger.Warn("failed to draw image by segmind, try replicate", zap.Error(err))

					imgContent, err = drawFluxByReplicate(taskCtx, model, req)
					if err != nil {
						return errors.Wrap(err, "draw image")
					}
				}

				if err := checkUserExternalBilling(taskCtx,
					user, price, "txt2image:"+model); err != nil {
					return errors.Wrapf(err, "check user external billing for %d image", i)
				}
				atomic.AddInt32(&anySucceed, 1)
				return uploadImage2Minio(taskCtx,
					fmt.Sprintf("%s-%d", drawImageByTxtObjkeyPrefix(taskID), i),
					req.Input.Prompt,
					imgContent,
					imgExt,
				)
			})
		}

		err = pool.Wait()
		if err != nil {
			// upload error msg
			msg := []byte(fmt.Sprintf("failed to draw image for %q, got %s",
				req.Input.Prompt, err.Error()))
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

			logger.Error("failed to draw som image",
				zap.Int("required", nImage),
				zap.Int32("succeed", anySucceed),
				zap.String("objkey", objkey),
				zap.Error(err),
			)
			return
		}

		logger.Info("succeed draw image done")
	}()

	var imgUrls []string
	for i := 0; i < nImage; i++ {
		imgUrls = append(imgUrls, fmt.Sprintf("https://%s/%s/%s-%d%s",
			config.Config.S3.Endpoint,
			config.Config.S3.Bucket,
			drawImageByTxtObjkeyPrefix(taskID), i, imgExt,
		))
	}

	ctx.JSON(http.StatusOK, gin.H{
		"task_id":    taskID,
		"image_urls": imgUrls,
	})
}

// drawFluxByReplicate draw image by replicate service
func drawFluxByReplicate(ctx context.Context,
	model string, req *DrawImageByFluxReplicateRequest) (img []byte, err error) {
	logger := gmw.GetLogger(ctx)
	logger.Debug("draw image by replicate")

	upstreamReqData := new(DrawImageByFluxReplicateRequest)
	if err = copier.Copy(upstreamReqData, req); err != nil {
		return nil, errors.Wrap(err, "copy request")
	}
	upstreamReqData.Input.Seed = rand.Int()

	upstreamReqBody, err := json.Marshal(upstreamReqData)
	if err != nil {
		return nil, errors.Wrap(err, "marshal request")
	}

	var api string
	switch model {
	case "flux-pro":
		api = "https://api.replicate.com/v1/models/black-forest-labs/flux-pro/predictions"
	case "flux-schnell":
		api = "https://api.replicate.com/v1/models/black-forest-labs/flux-schnell/predictions"
	}

	upstreamReq, err := http.NewRequestWithContext(ctx,
		http.MethodPost, api, bytes.NewReader(upstreamReqBody))
	if err != nil {
		return nil, errors.Wrap(err, "new request to draw image")
	}

	upstreamReq.Header.Add("Content-Type", "application/json")
	upstreamReq.Header.Add("Authorization", "Bearer "+config.Config.ReplicateApikey)

	resp, err := httpcli.Do(upstreamReq) //nolint: bodyclose
	if err != nil {
		return nil, errors.Wrap(err, "do request")
	}
	defer gutils.LogErr(resp.Body.Close, logger)

	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("bad status code [%d]%s",
			resp.StatusCode, string(payload))
	}

	respData := new(DrawImageByFluxProResponse)
	if err = json.NewDecoder(resp.Body).Decode(respData); err != nil {
		return nil, errors.Wrap(err, "decode response")
	}

	var imgContent []byte
	for {
		err = func() error {
			// get task
			taskReq, err := http.NewRequestWithContext(ctx,
				http.MethodGet, respData.URLs.Get, nil)
			if err != nil {
				return errors.Wrap(err, "new request")
			}

			taskReq.Header.Set("Authorization", "Bearer "+config.Config.ReplicateApikey)
			taskResp, err := httpcli.Do(taskReq) //nolint: bodyclose
			if err != nil {
				return errors.Wrap(err, "get task")
			}
			defer gutils.LogErr(taskResp.Body.Close, logger)

			if taskResp.StatusCode != http.StatusOK {
				payload, _ := io.ReadAll(taskResp.Body)
				return errors.Errorf("bad status code [%d]%s",
					taskResp.StatusCode, string(payload))
			}

			taskBody, err := io.ReadAll(taskResp.Body)
			if err != nil {
				return errors.Wrap(err, "read task response")
			}

			taskData := new(DrawImageByFluxProResponse)
			if err = json.Unmarshal(taskBody, taskData); err != nil {
				return errors.Wrapf(err, "decode task response %s", string(taskBody))
			}

			switch taskData.Status {
			case "succeeded":
			case "failed", "canceled":
				return errors.Errorf("task failed: %s", taskData.Status)
			default:
				logger.Debug("wait image task done",
					zap.String("status", taskData.Status))
				time.Sleep(time.Second * 3)
				return nil
			}

			if len(taskData.Output) == 0 {
				return errors.New("empty image url")
			}

			// download image
			logger.Debug("try to download image",
				zap.String("img_url", taskData.Output[0]))
			downloadReq, err := http.NewRequestWithContext(ctx,
				http.MethodGet, taskData.Output[0], nil)
			if err != nil {
				return errors.Wrap(err, "new request")
			}

			imgResp, err := httpcli.Do(downloadReq) //nolint: bodyclose
			if err != nil {
				return errors.Wrap(err, "download image")
			}
			defer gutils.LogErr(imgResp.Body.Close, logger)

			// upload image
			imgContent, err = io.ReadAll(imgResp.Body)
			if err != nil {
				return errors.Wrap(err, "read image")
			}

			return nil
		}()
		if err != nil {
			return nil, errors.Wrap(err, "wait image task done")
		}

		if len(imgContent) != 0 {
			break
		}
	}

	imgContent, err = ConvertWebPToPNG(imgContent)
	if err != nil {
		return nil, errors.Wrap(err, "convert webp to png")
	}

	return imgContent, nil
}

// ConvertWebPToPNG converts a WebP image to PNG format
func ConvertWebPToPNG(webpData []byte) ([]byte, error) {
	// Decode the WebP image
	img, err := webp.Decode(bytes.NewReader(webpData))
	if err != nil {
		return nil, errors.Wrap(err, "decode webp")
	}

	// Encode the image as PNG
	var pngBuffer bytes.Buffer
	if err := png.Encode(&pngBuffer, img); err != nil {
		return nil, errors.Wrap(err, "encode png")
	}

	return pngBuffer.Bytes(), nil
}

// drawFluxBySegmind draw image by replicate service
func drawFluxBySegmind(ctx context.Context,
	model string, req *DrawImageByFluxReplicateRequest) (img []byte, err error) {
	logger := gmw.GetLogger(ctx)
	logger.Debug("draw image by segmind")

	upstreamReqData := &DrawImageByFluxSegmind{
		Prompt:      req.Input.Prompt,
		Steps:       req.Input.Steps,
		Seed:        rand.Int(),
		SamplerName: "euler",
		Scheduler:   "normal",
		Samples:     1,
		Width:       2048,
		Height:      2048,
		Denoise:     1,
	}

	upstreamReqBody, err := json.Marshal(upstreamReqData)
	if err != nil {
		return nil, errors.Wrap(err, "marshal request")
	}

	var api string
	switch model {
	// case "flux-pro":
	// 	api = "https://api.segmind.com/v1/flux-pro"
	case "flux-schnell":
		api = "https://api.segmind.com/v1/flux-schnell"
	default:
		return nil, errors.Errorf("unknown model %q", model)
	}

	logger.Debug("send request to segmind", zap.String("api", api))
	upstreamReq, err := http.NewRequestWithContext(ctx,
		http.MethodPost, api, bytes.NewReader(upstreamReqBody))
	if err != nil {
		return nil, errors.Wrap(err, "new request to draw image")
	}

	upstreamReq.Header.Add("Content-Type", "application/json")
	upstreamReq.Header.Add("x-api-key", config.Config.SegmindApikey)

	resp, err := httpcli.Do(upstreamReq) //nolint: bodyclose
	if err != nil {
		return nil, errors.Wrap(err, "do request")
	}
	defer gutils.LogErr(resp.Body.Close, logger)

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("bad status code [%d]%s",
			resp.StatusCode, string(payload))
	}

	imgContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read image")
	}

	return imgContent, nil
}

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

	if err = user.IsModelAllowed(ctx, req.Model, 0); web.AbortErr(ctx, err) {
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

func DrawBySdxlturboHandlerByNvidia(ctx *gin.Context) {
	rawreq := new(DrawImageBySdxlturboRequest)
	if err := ctx.BindJSON(rawreq); web.AbortErr(ctx, err) {
		return
	}

	user, err := getUserByAuthHeader(ctx)
	if web.AbortErr(ctx, err) {
		return
	}

	if err = user.IsModelAllowed(ctx, rawreq.Model, 0); web.AbortErr(ctx, err) {
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
			if _, err := s3.GetCli().PutObject(ctx,
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

	if err = user.IsModelAllowed(ctx, req.Model, 0); web.AbortErr(ctx, err) {
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

	if err = user.IsModelAllowed(ctx, req.Model, 0); web.AbortErr(ctx, err) {
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

func uploadImage2Minio(ctx context.Context,
	objkeyPrefix,
	prompt string,
	imgContent []byte,
	imgExt string,
) (err error) {
	logger := gmw.GetLogger(ctx)
	s3cli := s3.GetCli()

	if imgExt == "" {
		imgExt = ".png"
	}

	// upload image
	var pool errgroup.Group
	pool.Go(func() (err error) {
		_, err = s3cli.PutObject(ctx,
			config.Config.S3.Bucket,
			objkeyPrefix+imgExt,
			bytes.NewReader(imgContent),
			int64(len(imgContent)),
			minio.PutObjectOptions{
				ContentType: "image/png",
			},
		)
		return errors.Wrap(err, "upload image")
	})

	// upload prompt
	pool.Go(func() (err error) {
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
