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
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/s3"
	s3lib "github.com/Laisky/go-ramjet/library/s3"
	"github.com/Laisky/go-ramjet/library/web"
)

// DrawByDalleHandler generates images for an OpenAI-compatible image request,
// stores them in the object store and answers with their URLs.
//
// The image count is bounded by normalizeImageCount. ctx is the gin context of
// the request; failures are reported to the client through web.AbortErr.
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

	if n := normalizeImageCount(req.N, user.IsFree); n != req.N {
		logger.Debug("image count limited",
			zap.Int("requested", req.N), zap.Int("limited", n), zap.Bool("free_user", user.IsFree))
		req.N = n
	}

	if err = IsModelAllowed(ctx, user,
		&FrontendReq{
			N:     req.N,
			Model: req.Model,
		}); web.AbortErr(ctx, err) {
		return
	}

	grantInferenceBudget(ctx, user, req.Model, 1)

	logger.Debug("start image drawing task")
	taskCtx, cancel := context.WithTimeout(gmw.Ctx(ctx), time.Minute*5)
	defer cancel()

	imgContents, err := generateImages(taskCtx, user, req.Model, req.Prompt, req.N, req.Size)
	if web.AbortErr(ctx, err) {
		return
	}
	imgUrls, err := storeGeneratedImages(taskCtx, req.N, taskID, req.Prompt, imgContents)
	if err != nil {
		// upload error msg
		msg := []byte(fmt.Sprintf("failed to draw image for %q, got %s", req.Prompt, err.Error()))
		objkey := drawImageByTxtObjkeyPrefix(taskID) + ".err.txt"
		if s3cli, errS3 := s3.GetCli(); errS3 != nil {
			logger.Error("get s3 client", zap.Error(errS3))
		} else if _, errS3 := s3lib.PutObjectCappingVersions(taskCtx,
			logger,
			s3cli,
			config.Config.S3.Bucket,
			objkey,
			bytes.NewReader(msg),
			int64(len(msg)),
			minio.PutObjectOptions{
				ContentType: "text/plain",
			},
			s3lib.DefaultVersionsToKeep,
		); errS3 != nil {
			logger.Error("upload error msg", zap.Error(errS3))
		}

		web.AbortErr(ctx, errors.Wrapf(err, "failed to draw image, objkey %s", objkey))
		return
	}

	logger.Info("succeed draw image done")

	openaiData := make([]gin.H, 0, len(imgUrls))
	for _, url := range imgUrls {
		openaiData = append(openaiData, gin.H{"url": url})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"task_id":    taskID,
		"image_urls": imgUrls,
		"created":    time.Now().Unix(),
		"data":       openaiData,
	})
}

// fetchImageFromOpenaiDalle asks user's OpenAI-compatible upstream for n images of
// model drawn from prompt, at size when it is not empty.
//
// It returns between 1 and n images: entries after the n-th image are never
// decoded or downloaded, and entries without image data are skipped. It returns an
// error when the upstream call fails, its answer exceeds the size allowance for n
// images, an image cannot be decoded or downloaded, or no entry carries an image.
func fetchImageFromOpenaiDalle(ctx context.Context,
	user *config.UserConfig, model, prompt string, n int, size string) (imgs [][]byte, err error) {
	logger := gmw.GetLogger(ctx).Named("openai")
	base, err := mediaInferenceBase(user)
	if err != nil {
		return nil, errors.Wrap(err, "select image generation upstream")
	}
	apiUrl := base + "/v1/images/generations"
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

	cli := upstreamHTTPClient(user)
	resp := new(OpenaiCreateImageResponse)
	if err := consumeInferenceCall(ctx, user, model); err != nil {
		return nil, errors.Wrap(err, "admit image request")
	}
	if httpresp, err := mediaInferenceClient(user).Do(req); err != nil { //nolint: bodyclose
		return nil, errors.Wrap(err, "do request")
	} else {
		defer gutils.LogErr(httpresp.Body.Close, logger)
		payload, err := readImageAPIResponse(httpresp.Body, maxImagesPerRequest)
		if err != nil {
			return nil, errors.Wrap(err, "read image response")
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
	// The upstream may answer with more entries than requested, and some entries
	// may carry no image. Stop once n images are collected, so no surplus entry is
	// decoded or downloaded and entries without an image do not use up the count.
	collector := newImageCollector(n)
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
			// The URL comes from the upstream, so it inherits the upstream's trust.
			imgContent, err = downloadImage(ctx, cli, data.Url)
			if err != nil {
				return nil, errors.Wrap(err, "download image from url")
			}
		}

		if collector.add(imgContent) {
			if i+1 < len(resp.Data) {
				logger.Debug("ignore surplus upstream images",
					zap.Int("requested", n), zap.Int("returned", len(resp.Data)))
			}
			break
		}
	}

	return collector.images()
}

// maxDownloadedImageBytes bounds one downloaded image or mask. Generated and
// uploaded images are a few megabytes; the cap stops a caller-chosen URL from
// streaming an unbounded body into memory.
const maxDownloadedImageBytes = 32 << 20

// downloadImage fetches the image at url with cli and returns its bytes.
//
// Parameters:
//   - ctx: Request-scoped context carrying the logger and cancellation.
//   - cli: Client matching the trust of url's origin; caller-chosen URLs need tenantEgressClient.
//   - url: Image location.
//
// Returns the body, at most maxDownloadedImageBytes long, or a wrapped error.
func downloadImage(ctx context.Context, cli *http.Client, url string) ([]byte, error) {
	logger := gmw.GetLogger(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}
	imgResp, err := cli.Do(req) //nolint: bodyclose
	if err != nil {
		return nil, errors.Wrap(err, "download image")
	}
	defer gutils.LogErr(imgResp.Body.Close, logger)

	if imgResp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("download image got bad status code %d", imgResp.StatusCode)
	}

	content, err := io.ReadAll(io.LimitReader(imgResp.Body, maxDownloadedImageBytes+1))
	if err != nil {
		return nil, errors.Wrap(err, "read image")
	}
	if len(content) > maxDownloadedImageBytes {
		return nil, errors.Errorf("image exceeds %d bytes", maxDownloadedImageBytes)
	}

	return content, nil
}

// EditImageHandler edits the caller's image (and optional mask) by prompt through
// the user's OpenAI-compatible upstream, stores the result in the object store and
// answers with its URL.
//
// Exactly one image is delivered. ctx is the gin context of the request;
// failures are reported to the client through web.AbortErr.
func EditImageHandler(ctx *gin.Context) {
	taskID := gutils.RandomStringWithLength(36)
	logger := gmw.GetLogger(ctx).Named("image_edit").With(zap.String("task_id", taskID))

	// The frontend sends JSON with base64-encoded images or URLs
	var req struct {
		Prompt string `binding:"required" json:"prompt"`
		Model  string `json:"model"`
		Image  string `binding:"required" json:"image"`
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

	grantInferenceBudget(ctx, user, req.Model, 1)

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
			return uploadGeneratedImage(taskCtx,
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

// getDataFromUrlOrBase64 returns the image named by input, which is either an
// http(s) URL or base64 content. The caller chose the URL, so it is always
// fetched through tenantEgressClient.
func getDataFromUrlOrBase64(ctx context.Context, input string) ([]byte, error) {
	if strings.HasPrefix(input, "http") {
		content, err := downloadImage(ctx, tenantEgressClient(), input)
		if err != nil {
			return nil, errors.Wrap(err, "download caller image")
		}
		return content, nil
	}

	content, err := DecodeBase64(input)
	if err != nil {
		return nil, errors.Wrap(err, "decode base64 image")
	}
	return content, nil
}

// fetchImageEditFromOpenai sends image, and mask when it is not empty, with prompt
// to the image edit API of user's OpenAI-compatible upstream for model.
//
// It returns exactly one image: entries after the first image are never decoded
// or downloaded. It returns an error when the request cannot be built, the upstream
// call fails, its answer exceeds the size allowance for one image, an image cannot
// be decoded or downloaded, or no entry carries an image.
func fetchImageEditFromOpenai(ctx context.Context,
	user *config.UserConfig, model, prompt string, image, mask []byte) (imgs [][]byte, err error) {
	logger := gmw.GetLogger(ctx)
	base, err := mediaInferenceBase(user)
	if err != nil {
		return nil, errors.Wrap(err, "select image editing upstream")
	}
	apiUrl := base + "/v1/images/edits"

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

	cli := upstreamHTTPClient(user)
	resp := new(OpenaiCreateImageResponse)
	if err := consumeInferenceCall(ctx, user, model); err != nil {
		return nil, errors.Wrap(err, "admit image request")
	}
	if httpresp, err := mediaInferenceClient(user).Do(req); err != nil { //nolint: bodyclose
		return nil, errors.Wrap(err, "do request")
	} else {
		defer gutils.LogErr(httpresp.Body.Close, logger)
		payload, err := readImageAPIResponse(httpresp.Body, maxImagesPerRequest)
		if err != nil {
			return nil, errors.Wrap(err, "read image edit response")
		}
		if httpresp.StatusCode != http.StatusOK {
			logger.Debug("openai image edit error",
				zap.Int("status", httpresp.StatusCode),
				zap.String("payload", string(payload)))
			return nil, errors.Errorf("bad status code [%d]%s", httpresp.StatusCode, string(payload))
		}

		if err = json.Unmarshal(payload, resp); err != nil {
			return nil, errors.Wrap(err, "decode response")
		}
	}

	// The edit request asks for one image and is charged for one, so stop at the
	// first image: surplus entries are never downloaded or delivered.
	collector := newImageCollector(1)
	for _, data := range resp.Data {
		var imgContent []byte
		if data.B64Json != "" {
			imgContent, err = DecodeBase64(data.B64Json)
			if err != nil {
				return nil, errors.Wrap(err, "decode image")
			}
		} else if data.Url != "" {
			// The URL comes from the upstream, so it inherits the upstream's trust.
			imgContent, err = downloadImage(ctx, cli, data.Url)
			if err != nil {
				return nil, errors.Wrap(err, "download image from url")
			}
		}

		if collector.add(imgContent) {
			break
		}
	}

	return collector.images()
}
