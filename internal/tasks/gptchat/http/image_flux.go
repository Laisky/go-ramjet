package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
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

	logger.Debug("start image drawing task")
	taskCtx, cancel := context.WithTimeout(gmw.Ctx(ctx), time.Minute*5)
	defer cancel()

	var pool errgroup.Group
	anySucceed := int32(0)
	for i := range nImage {
		i := i

		pool.Go(func() (err error) {
			logger := logger.With(zap.Int("n_img", i))
			taskCtx := gmw.SetLogger(taskCtx, logger)

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
				user, price, "txt2image:"+model); err != nil {
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

		web.AbortErr(ctx, errors.Wrapf(err, "failed to draw %d images, succeed %d, objkey %s",
			nImage, anySucceed, objkey))
		return
	}

	logger.Info("succeed draw image done")

	var imgUrls []string
	var openaiData []gin.H
	for i := 0; i < nImage; i++ {
		url := fmt.Sprintf("https://%s/%s/%s-%d%s",
			config.Config.S3.Endpoint,
			config.Config.S3.Bucket,
			drawImageByTxtObjkeyPrefix(taskID), i, imgExt,
		)
		imgUrls = append(imgUrls, url)
		openaiData = append(openaiData, gin.H{"url": url})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"task_id":    taskID,
		"image_urls": imgUrls,
		"created":    time.Now().Unix(),
		"data":       openaiData,
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

// drawFluxBySegmind draw image by replicate service
// func drawFluxBySegmind(ctx context.Context,
// 	model string, req *DrawImageByFluxReplicateRequest) (img []byte, err error) {
// 	logger := gmw.GetLogger(ctx)
// 	logger.Debug("draw image by segmind")

// 	upstreamReqData := &DrawImageByFluxSegmind{
// 		Prompt:      req.Input.Prompt,
// 		Steps:       req.Input.Steps,
// 		Seed:        rand.Int(),
// 		SamplerName: "euler",
// 		Scheduler:   "normal",
// 		Samples:     1,
// 		Width:       2048,
// 		Height:      2048,
// 		Denoise:     1,
// 	}

// 	upstreamReqBody, err := json.Marshal(upstreamReqData)
// 	if err != nil {
// 		return nil, errors.Wrap(err, "marshal request")
// 	}

// 	var api string
// 	switch model {
// 	// case "flux-pro":
// 	// 	api = "https://api.segmind.com/v1/flux-pro"
// 	case "flux-schnell":
// 		api = "https://api.segmind.com/v1/flux-schnell"
// 	default:
// 		return nil, errors.Errorf("unknown model %q", model)
// 	}

// 	logger.Debug("send request to segmind", zap.String("api", api))
// 	upstreamReq, err := http.NewRequestWithContext(ctx,
// 		http.MethodPost, api, bytes.NewReader(upstreamReqBody))
// 	if err != nil {
// 		return nil, errors.Wrap(err, "new request to draw image")
// 	}

// 	upstreamReq.Header.Add("Content-Type", "application/json")
// 	upstreamReq.Header.Add("x-api-key", config.Config.SegmindApikey)

// 	resp, err := httpcli.Do(upstreamReq) //nolint: bodyclose
// 	if err != nil {
// 		return nil, errors.Wrap(err, "do request")
// 	}
// 	defer gutils.LogErr(resp.Body.Close, logger)

// 	if resp.StatusCode != http.StatusOK {
// 		payload, _ := io.ReadAll(resp.Body)
// 		return nil, errors.Errorf("bad status code [%d]%s",
// 			resp.StatusCode, string(payload))
// 	}

// 	imgContent, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		return nil, errors.Wrap(err, "read image")
// 	}

// 	return imgContent, nil
// }
