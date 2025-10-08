package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/go-utils/v5/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
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

	req := new(DrawImageByFluxReplicateRequest)
	if err := ctx.BindJSON(req); web.AbortErr(ctx, err) {
		return
	}

	replicateFluxHandler(ctx, req.Input.NImages, model, req.Input.Prompt, req)
}

func InpaitingByFlux(ctx *gin.Context) {
	model := strings.TrimSpace(ctx.Param("model"))
	if model == "" {
		web.AbortErr(ctx, errors.New("empty model"))
	}

	req := new(InpaintingImageByFlusReplicateRequest)
	if err := ctx.BindJSON(req); web.AbortErr(ctx, err) {
		return
	}

	replicateFluxHandler(ctx, 1, model, req.Input.Prompt, req)
}

func replicateFluxHandler(ctx *gin.Context, nImage int, model, prompt string, req any) {
	var price db.Price
	imgExt := ".png"
	switch model {
	case "flux-dev":
		price = db.PriceTxt2ImageFluxDev
	case "flux-schnell":
		price = db.PriceTxt2ImageSchnell
	case "flux-pro":
		price = db.PriceTxt2ImageFluxPro
	case "flux-fill-pro":
		price = db.PriceTxt2ImageFluxFillPro
	case "flux-1.1-pro":
		price = db.PriceTxt2ImageFluxPro11
	case "flux-kontext-pro":
		price = db.PriceTxt2ImageFluxKontextPro
	case "flux-1.1-pro-ultra":
		price = db.PriceTxt2ImageFluxProUltra11
	default:
		web.AbortErr(ctx, errors.Errorf("unknown model %q", model))
		return
	}

	taskID := gutils.RandomStringWithLength(36)
	logger := gmw.GetLogger(ctx).Named("image_flux").With(
		zap.String("task_id", taskID),
		zap.String("model", model),
		zap.Int("n", nImage),
	)
	gmw.SetLogger(ctx, logger)

	user, err := getUserByAuthHeader(ctx)
	if web.AbortErr(ctx, err) {
		return
	}

	if err = IsModelAllowed(ctx, user, &FrontendReq{
		N:     nImage,
		Model: model}); web.AbortErr(ctx, err) {
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
				start := time.Now()

				var imgContent []byte
				switch r := req.(type) {
				case *DrawImageByFluxReplicateRequest:
					imgContent, err = drawFluxByReplicate(taskCtx, model, r)
				case *InpaintingImageByFlusReplicateRequest:
					imgContent, err = inpaitingFluxByReplicate(taskCtx, model, r)
				default:
					err = errors.Errorf("unknown request type %T", req)
				}
				if err != nil {
					return errors.WithStack(err)
				}

				if err := checkUserExternalBilling(taskCtx,
					user, price, "txt2image:"+model, time.Since(start)); err != nil {
					return errors.Wrapf(err, "check user external billing for %d image", i)
				}
				atomic.AddInt32(&anySucceed, 1)
				return uploadImage2Minio(taskCtx,
					fmt.Sprintf("%s-%d", drawImageByTxtObjkeyPrefix(taskID), i),
					prompt,
					imgContent,
					imgExt,
				)
			})
		}

		err = pool.Wait()
		if err != nil {
			// upload error msg
			msg := []byte(fmt.Sprintf("failed to draw image for %q, got %s",
				prompt, err.Error()))
			objkey := drawImageByTxtObjkeyPrefix(taskID) + ".err.txt"
			s3cli, errS3 := s3.GetCli()
			if errS3 != nil {
				logger.Error("get s3 client", zap.Error(errS3))
			}

			if _, errS3 := s3cli.PutObject(taskCtx,
				config.Config.S3.Bucket,
				objkey,
				bytes.NewReader(msg),
				int64(len(msg)),
				minio.PutObjectOptions{
					ContentType: "text/plain",
				}); errS3 != nil {
				logger.Error("upload error msg", zap.Error(errS3))
			}

			logger.Error("failed to draw some images",
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

	req.Input.Seed = rand.Int()

	if model == "flux-kontext-pro" && req.Input.ImagePrompt != nil {
		req.Input.InputImage = req.Input.ImagePrompt
		req.Input.ImagePrompt = nil
	}

	upstreamReqBody, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "marshal request")
	}

	img, err = requestFluxImageAPI(ctx, model, upstreamReqBody)
	if err != nil {
		return nil, errors.Wrap(err, "request flux image api")
	}

	return img, nil
}

func inpaitingFluxByReplicate(ctx context.Context,
	model string, req *InpaintingImageByFlusReplicateRequest) (img []byte, err error) {
	logger := gmw.GetLogger(ctx)
	logger.Debug("inpaiting image by replicate")

	req.Input.Seed = rand.Int()
	req.Input.OutputFormat = "png"

	upstreamReqBody, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "marshal request")
	}

	img, err = requestFluxImageAPI(ctx, model, upstreamReqBody)
	if err != nil {
		return nil, errors.Wrap(err, "request flux image api")
	}

	return img, nil
}

func requestFluxImageAPI(ctx context.Context,
	model string, reqBody []byte) (img []byte, err error) {
	logger := gmw.GetLogger(ctx)

	api := fmt.Sprintf("https://api.replicate.com/v1/models/black-forest-labs/%s/predictions", model)
	upstreamReq, err := http.NewRequestWithContext(ctx,
		http.MethodPost, api, bytes.NewReader(reqBody))
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read response")
	}

	respData := new(DrawImageByFluxProResponse)
	if err = json.Unmarshal(respBody, respData); err != nil {
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
				return errors.Wrap(err, "decode task response")
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

			output, err := taskData.GetOutput()
			if err != nil {
				return errors.Wrap(err, "get output")
			}
			if len(output) == 0 {
				return errors.New("empty image url")
			}

			// download image
			logger.Debug("try to download image",
				zap.String("img_url", output[0]))
			downloadReq, err := http.NewRequestWithContext(ctx,
				http.MethodGet, output[0], nil)
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

	imgContent, err = ConvertImageToPNG(imgContent)
	if err != nil {
		return nil, errors.Wrap(err, "convert webp to png")
	}

	return imgContent, nil
}

// ConvertImageToPNG converts a WebP image to PNG format
func ConvertImageToPNG(webpData []byte) ([]byte, error) {
	// bypass if it's already a PNG image
	if bytes.HasPrefix(webpData, []byte("\x89PNG")) {
		return webpData, nil
	}

	// check if is jpeg, convert to png
	if bytes.HasPrefix(webpData, []byte("\xff\xd8\xff")) {
		img, _, err := image.Decode(bytes.NewReader(webpData))
		if err != nil {
			return nil, errors.Wrap(err, "decode jpeg")
		}

		var pngBuffer bytes.Buffer
		if err := png.Encode(&pngBuffer, img); err != nil {
			return nil, errors.Wrap(err, "encode png")
		}

		return pngBuffer.Bytes(), nil
	}

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

	go func() {
		logger.Debug("start image drawing task")
		taskCtx, cancel := context.WithTimeout(ctx, time.Minute*5)
		defer cancel()
		start := time.Now()

		switch {
		case strings.Contains(user.ImageUrl, "openai.azure.com"):
			err = drawImageByAzureDalle(taskCtx, user, req.Prompt, taskID)
		default:
			err = drawImageByOpenaiDalle(taskCtx, user, req.Prompt, taskID)
		}

		if err == nil && user.EnableExternalImageBilling {
			if billErr := checkUserExternalBilling(taskCtx,
				user, db.PriceTxt2Image, "txt2image", time.Since(start)); billErr != nil {
				err = errors.Wrap(billErr, "check user external billing")
			}
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
	s3cli, err := s3.GetCli()
	if err != nil {
		return errors.Wrap(err, "get s3 client")
	}

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
