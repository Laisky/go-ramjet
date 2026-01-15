package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
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
	"golang.org/x/sync/errgroup"

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

	if req.N <= 0 {
		req.N = 1
	}

	logger := gmw.GetLogger(ctx).Named("image").With(
		zap.String("task_id", taskID),
		zap.String("model", req.Model),
	)
	gmw.SetLogger(ctx, logger)

	user, err := getUserByAuthHeader(ctx)
	if web.AbortErr(ctx, err) {
		return
	}

	if user.IsFree && req.N > 1 {
		logger.Debug("n is limited to 1 for free user")
		req.N = 1
	}
	if req.N > 8 {
		logger.Debug("n is limited to 8")
		req.N = 8
	}

	if err = IsModelAllowed(ctx, user,
		&FrontendReq{
			N:     req.N,
			Model: req.Model,
		}); web.AbortErr(ctx, err) {
		return
	}

	if user.EnableExternalImageBilling {
		if err := checkUserExternalBilling(gmw.Ctx(ctx),
			user, GetImageModelPrice(req.Model), "txt2image"); web.AbortErr(ctx, err) {
			return
		}
	}

	logger.Debug("start image drawing task")
	taskCtx, cancel := context.WithTimeout(gmw.Ctx(ctx), time.Minute*5)
	defer cancel()

	var imgContents [][]byte
	switch {
	case strings.Contains(user.ImageUrl, "openai.azure.com"):
		var pool errgroup.Group
		imgContents = make([][]byte, req.N)
		for i := range req.N {
			i := i
			pool.Go(func() (err error) {
				imgContents[i], err = fetchImageFromAzureDalle(taskCtx, user, req.Prompt)
				return err
			})
		}
		if err := pool.Wait(); web.AbortErr(ctx, err) {
			return
		}
	default:
		imgContents, err = fetchImageFromOpenaiDalle(taskCtx, user, req.Model, req.Prompt, req.N, req.Size)
		if web.AbortErr(ctx, err) {
			return
		}
	}

	var pool errgroup.Group
	for i, imgContent := range imgContents {
		i, imgContent := i, imgContent
		pool.Go(func() error {
			return uploadImage2Minio(taskCtx,
				fmt.Sprintf("%s-%d", drawImageByTxtObjkeyPrefix(taskID), i),
				req.Prompt,
				imgContent,
				".png",
			)
		})
	}

	if err := pool.Wait(); err != nil {
		// upload error msg
		msg := []byte(fmt.Sprintf("failed to draw image for %q, got %s", req.Prompt, err.Error()))
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

		web.AbortErr(ctx, errors.Wrapf(err, "failed to draw image, objkey %s", objkey))
		return
	}

	logger.Info("succeed draw image done")

	var imgUrls []string
	var openaiData []gin.H
	for i := range imgContents {
		url := fmt.Sprintf("https://%s/%s/%s-%d.%s",
			config.Config.S3.Endpoint,
			config.Config.S3.Bucket,
			drawImageByTxtObjkeyPrefix(taskID), i, "png",
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

func fetchImageFromOpenaiDalle(ctx context.Context,
	user *config.UserConfig, model, prompt string, n int, size string) (imgs [][]byte, err error) {
	logger := gmw.GetLogger(ctx).Named("openai")
	apiUrl := user.APIBase + "/v1/images/generations"
	logger.Debug("draw image by openai dalle",
		zap.String("url", apiUrl),
		zap.String("model", model),
		zap.Int("n", n),
		zap.String("size", size))

	openaiReq := NewOpenaiCreateImageRequest(prompt, n)
	openaiReq.Model = model
	if strings.Contains(model, "dall-e-3") {
		openaiReq.Style = "vivid"
	}
	if size != "" {
		openaiReq.Size = size
	}
	reqBody, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, errors.Wrap(err, "marshal request body")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiUrl, bytes.NewReader(reqBody))
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+user.OpenaiToken)

	resp := new(OpenaiCreateImageResponse)
	if httpresp, err := httpcli.Do(req); err != nil { //nolint: bodyclose
		return nil, errors.Wrap(err, "do request")
	} else {
		defer gutils.LogErr(httpresp.Body.Close, logger)
		payload, err := io.ReadAll(httpresp.Body)
		if err != nil {
			return nil, errors.Wrap(err, "read response body")
		}

		if httpresp.StatusCode != http.StatusOK {
			logger.Debug("openai dalle error",
				zap.Int("status", httpresp.StatusCode),
				zap.String("payload", string(payload)))
			return nil, errors.Errorf("bad status code [%d]%s", httpresp.StatusCode, string(payload))
		}

		if err = json.Unmarshal(payload, resp); err != nil {
			return nil, errors.Wrap(err, "decode response")
		}

		if len(resp.Data) == 0 {
			logger.Debug("empty response from openai", zap.String("payload", string(payload)))
			return nil, errors.New("empty response")
		}
	}

	logger.Debug("succeed get image from openai", zap.Int("n", len(resp.Data)))
	for i, data := range resp.Data {
		var imgContent []byte
		if data.B64Json != "" {
			logger.Debug("decode b64_json", zap.Int("index", i), zap.Int("len", len(data.B64Json)))
			imgContent, err = DecodeBase64(data.B64Json)
			if err != nil {
				prefix := data.B64Json
				if len(prefix) > 20 {
					prefix = prefix[:20]
				}
				return nil, errors.Wrapf(err, "decode image [%d] (len %d, prefix %q)",
					i, len(data.B64Json), prefix)
			}
		} else if data.Url != "" {
			logger.Debug("download from url", zap.Int("index", i), zap.String("url", data.Url))
			// download from url
			imgContent, err = downloadImage(ctx, data.Url)
			if err != nil {
				return nil, errors.Wrap(err, "download image from url")
			}
		}

		if len(imgContent) > 0 {
			imgs = append(imgs, imgContent)
		}
	}

	return imgs, nil
}

func fetchImageFromAzureDalle(ctx context.Context,
	user *config.UserConfig, prompt string) (img []byte, err error) {
	logger := gmw.GetLogger(ctx).Named("azure")
	logger.Debug("draw image by azure dalle")

	reqBody, err := json.Marshal(OpenaiCreateImageRequest{
		Prompt: prompt,
		Size:   "1024x1024",
		N:      1,
	})
	if err != nil {
		return nil, errors.Wrap(err, "marshal request body")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		user.ImageUrl, bytes.NewReader(reqBody))
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Api-Key", user.ImageToken)

	resp := new(AzureCreateImageResponse)
	if httpresp, err := httpcli.Do(req); err != nil { //nolint: bodyclose
		return nil, errors.Wrap(err, "do request")
	} else {
		defer gutils.LogErr(httpresp.Body.Close, logger)
		if httpresp.StatusCode != http.StatusOK {
			payload, _ := io.ReadAll(httpresp.Body)
			return nil, errors.Errorf("bad status code [%d]%s", httpresp.StatusCode, string(payload))
		}

		if err = json.NewDecoder(httpresp.Body).Decode(resp); err != nil {
			return nil, errors.Wrap(err, "decode response")
		}

		if len(resp.Data) == 0 {
			return nil, errors.New("empty response")
		}
	}

	// download image
	return downloadImage(ctx, resp.Data[0].Url)
}

func downloadImage(ctx context.Context, url string) ([]byte, error) {
	logger := gmw.GetLogger(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}
	imgResp, err := httpcli.Do(req) //nolint: bodyclose
	if err != nil {
		return nil, errors.Wrap(err, "download image")
	}
	defer gutils.LogErr(imgResp.Body.Close, logger)

	if imgResp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("download image got bad status code %d", imgResp.StatusCode)
	}

	return io.ReadAll(imgResp.Body)
}

func EditImageHandler(ctx *gin.Context) {
	taskID := gutils.RandomStringWithLength(36)
	logger := gmw.GetLogger(ctx).Named("image_edit").With(zap.String("task_id", taskID))

	// The frontend sends JSON with base64-encoded images or URLs
	var req struct {
		Prompt string `json:"prompt" binding:"required"`
		Model  string `json:"model"`
		Image  string `json:"image" binding:"required"`
		Mask   string `json:"mask"`
	}
	if err := ctx.BindJSON(&req); web.AbortErr(ctx, err) {
		return
	}

	user, err := getUserByAuthHeader(ctx)
	if web.AbortErr(ctx, err) {
		return
	}

	if err = IsModelAllowed(ctx, user, &FrontendReq{Model: req.Model}); web.AbortErr(ctx, err) {
		return
	}

	if user.EnableExternalImageBilling {
		price := GetImageModelPrice(req.Model)
		if req.Model == "flux-fill-pro" || req.Model == "black-forest-labs/flux-fill-pro" {
			price = db.PriceTxt2ImageFluxFillPro
		}

		if err := checkUserExternalBilling(gmw.Ctx(ctx),
			user, price, "image-edit"); web.AbortErr(ctx, err) {
			return
		}
	}

	logger.Debug("start image editing task", zap.String("model", req.Model))
	taskCtx, cancel := context.WithTimeout(gmw.Ctx(ctx), time.Minute*5)
	defer cancel()

	// 1. Prepare image and mask
	imageContent, err := getDataFromUrlOrBase64(taskCtx, req.Image)
	if web.AbortErr(ctx, errors.Wrap(err, "get image content")) {
		return
	}

	var maskContent []byte
	if req.Mask != "" {
		maskContent, err = getDataFromUrlOrBase64(taskCtx, req.Mask)
		if web.AbortErr(ctx, errors.Wrap(err, "get mask content")) {
			return
		}
	}

	// 2. Call OpenAI edit endpoint
	imgContents, err := fetchImageEditFromOpenai(taskCtx, user, req.Model, req.Prompt, imageContent, maskContent)
	if web.AbortErr(ctx, err) {
		return
	}

	// 3. Upload to S3
	var pool errgroup.Group
	for i, imgContent := range imgContents {
		i, imgContent := i, imgContent
		pool.Go(func() error {
			return uploadImage2Minio(taskCtx,
				fmt.Sprintf("%s-%d", drawImageByImageObjkeyPrefix(taskID), i),
				req.Prompt,
				imgContent,
				".png",
			)
		})
	}

	if err := pool.Wait(); web.AbortErr(ctx, err) {
		return
	}

	var imgUrls []string
	var openaiData []gin.H
	for i := range imgContents {
		url := fmt.Sprintf("https://%s/%s/%s-%d.%s",
			config.Config.S3.Endpoint,
			config.Config.S3.Bucket,
			drawImageByImageObjkeyPrefix(taskID), i, "png",
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

func getDataFromUrlOrBase64(ctx context.Context, input string) ([]byte, error) {
	if strings.HasPrefix(input, "http") {
		return downloadImage(ctx, input)
	}

	return DecodeBase64(input)
}

func fetchImageEditFromOpenai(ctx context.Context,
	user *config.UserConfig, model, prompt string, image, mask []byte) (imgs [][]byte, err error) {
	logger := gmw.GetLogger(ctx)
	apiUrl := user.APIBase + "/v1/images/edits"

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err = writer.WriteField("prompt", prompt); err != nil {
		return nil, errors.Wrap(err, "write field prompt")
	}
	if model != "" {
		if err = writer.WriteField("model", model); err != nil {
			return nil, errors.Wrap(err, "write field model")
		}
	}
	if err = writer.WriteField("response_format", "b64_json"); err != nil {
		return nil, errors.Wrap(err, "write field response_format")
	}

	// OpenAI requires images to be PNG and < 4MB for dall-e-2
	// For other models it might differ, but PNG is safe.
	image, err = ConvertImageToPNG(image)
	if err != nil {
		return nil, errors.Wrap(err, "convert image to png")
	}
	part, err := writer.CreateFormFile("image", "image.png")
	if err != nil {
		return nil, errors.Wrap(err, "create form file image")
	}
	if _, err = part.Write(image); err != nil {
		return nil, errors.Wrap(err, "write image to multipart")
	}

	if len(mask) > 0 {
		mask, err = ConvertImageToPNG(mask)
		if err != nil {
			return nil, errors.Wrap(err, "convert mask to png")
		}
		part, err = writer.CreateFormFile("mask", "mask.png")
		if err != nil {
			return nil, errors.Wrap(err, "create form file mask")
		}
		if _, err = part.Write(mask); err != nil {
			return nil, errors.Wrap(err, "write mask to multipart")
		}
	}

	if err = writer.Close(); err != nil {
		return nil, errors.Wrap(err, "close writer")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiUrl, &body)
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+user.OpenaiToken)

	resp := new(OpenaiCreateImageResponse)
	if httpresp, err := httpcli.Do(req); err != nil { //nolint: bodyclose
		return nil, errors.Wrap(err, "do request")
	} else {
		defer gutils.LogErr(httpresp.Body.Close, logger)
		if httpresp.StatusCode != http.StatusOK {
			payload, _ := io.ReadAll(httpresp.Body)
			logger.Debug("openai image edit error",
				zap.Int("status", httpresp.StatusCode),
				zap.String("payload", string(payload)))
			return nil, errors.Errorf("bad status code [%d]%s", httpresp.StatusCode, string(payload))
		}

		if err = json.NewDecoder(httpresp.Body).Decode(resp); err != nil {
			return nil, errors.Wrap(err, "decode response")
		}
	}

	for _, data := range resp.Data {
		var imgContent []byte
		if data.B64Json != "" {
			imgContent, err = DecodeBase64(data.B64Json)
			if err != nil {
				return nil, errors.Wrap(err, "decode image")
			}
		} else if data.Url != "" {
			imgContent, err = downloadImage(ctx, data.Url)
			if err != nil {
				return nil, errors.Wrap(err, "download image from url")
			}
		}

		if len(imgContent) > 0 {
			imgs = append(imgs, imgContent)
		}
	}

	return imgs, nil
}
