package http

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
)

// maxImagesPerRequest caps how many images one request may generate.
const maxImagesPerRequest = 8

// normalizeImageCount returns how many images to generate for a request asking for
// n: at least 1, at most maxImagesPerRequest, and exactly 1 for free users. The
// count is request-controlled, so it must be bounded before it sizes allocations,
// goroutines or upstream requests.
func normalizeImageCount(n int, isFree bool) int {
	switch {
	case n <= 0, isFree:
		return 1
	case n > maxImagesPerRequest:
		return maxImagesPerRequest
	default:
		return n
	}
}

// uploadGeneratedImage stores one generated image; it is a package variable so
// tests can observe delivered images without an object store.
var uploadGeneratedImage = uploadImage2Minio

// chargeExtraImages settles a DALL-E request's charge once its images are stored.
// One image is charged upfront, which doubles as the balance probe; this charges
// the remaining stored images in one call.
//
// Parameters:
//   - ctx: Request context carrying the request logger.
//   - user: User being billed.
//   - model: Image model, used for billing diagnostics.
//   - unitPrice: Per-image quote in One API quota units; zero skips external billing.
//   - stored: Number of images stored for the client.
//
// It returns how many of the stored images may be delivered: all of them, or only
// the prepaid first one when the charge for the others fails. That failure is
// logged once at error level; like Flux, unbilled images are never exposed.
func chargeExtraImages(ctx context.Context, user *config.UserConfig, model string, unitPrice db.Price, stored int) int {
	extra := stored - 1
	if unitPrice <= 0 || extra <= 0 {
		return stored
	}

	if err := checkUserExternalBilling(ctx, user, unitPrice*db.Price(extra), "txt2image"); err != nil {
		gmw.GetLogger(ctx).Error("charge for extra generated images failed; delivering only the prepaid image",
			zap.String("model", model), zap.Int("stored", stored), zap.Error(err))
		return 1
	}

	return stored
}

// generatedImageKey returns a new object key, without extension, for the i-th
// image of task taskID. Each key ends in its own cryptographically random suffix,
// so the URL of one delivered image never reveals the key of another image of the
// same task that was stored but not paid for.
//
// It returns an error when no random suffix can be generated.
func generatedImageKey(taskID string, i int) (string, error) {
	suffix, err := gutils.SecRandomStringWithLength(20)
	if err != nil {
		return "", errors.Wrap(err, "generate image key suffix")
	}

	return fmt.Sprintf("%s-%d-%s", drawImageByTxtObjkeyPrefix(taskID), i, suffix), nil
}

// generatedImageURL returns the public URL of the PNG stored under key.
func generatedImageURL(key string) string {
	return fmt.Sprintf("https://%s/%s/%s.png", config.Config.S3.Endpoint, config.Config.S3.Bucket, key)
}

// storeAndBillImages stores generated images and only then charges for the ones
// beyond the prepaid first image, so a failed upload never bills images nobody
// receives.
//
// Parameters:
//   - ctx: Request context used for billing; it carries the request logger.
//   - taskCtx: Context bounding the uploads.
//   - user: User being billed.
//   - model: Image model, used for billing diagnostics.
//   - unitPrice: Per-image quote in One API quota units; zero skips external billing.
//   - n: Normalized number of requested images; never more are stored.
//   - taskID: Task whose key prefix the objects share.
//   - prompt: Prompt stored as object metadata.
//   - imgs: Images the upstream produced.
//
// It returns the URLs to deliver: every stored image, or only the prepaid first one
// when the charge for the others fails. It returns an error when a key cannot be
// generated or an upload fails; nothing beyond the upfront charge is billed then.
func storeAndBillImages(ctx, taskCtx context.Context, user *config.UserConfig,
	model string, unitPrice db.Price, n int, taskID, prompt string, imgs [][]byte) ([]string, error) {
	if len(imgs) > n {
		imgs = imgs[:n]
	}

	keys := make([]string, len(imgs))
	for i := range imgs {
		key, err := generatedImageKey(taskID, i)
		if err != nil {
			return nil, errors.Wrap(err, "image key")
		}
		keys[i] = key
	}

	var pool errgroup.Group
	for i, img := range imgs {
		pool.Go(func() error {
			return uploadGeneratedImage(taskCtx, keys[i], prompt, img, ".png")
		})
	}
	if err := pool.Wait(); err != nil {
		return nil, errors.Wrap(err, "upload images")
	}

	keys = keys[:chargeExtraImages(ctx, user, model, unitPrice, len(keys))]
	urls := make([]string, len(keys))
	for i, key := range keys {
		urls[i] = generatedImageURL(key)
	}

	return urls, nil
}

// generateImages asks user's image upstream for n images of model drawn from
// prompt, at size when it is not empty.
//
// An Azure image deployment answers one image per call, so n calls run
// concurrently; any other upstream gets one call asking for n images.
//
// Parameters:
//   - ctx: Context bounding every upstream call.
//   - user: User whose image upstream and credentials are used.
//   - model: Image model; ignored by Azure deployments.
//   - prompt: Text the images are drawn from.
//   - n: Normalized number of images (see normalizeImageCount).
//   - size: Requested image size for OpenAI-compatible upstreams; empty keeps the
//     upstream default.
//
// It returns the generated images, or the first error. A count outside
// 1..maxImagesPerRequest is refused before any allocation or upstream call, so the
// bound holds even for a caller that skipped normalizeImageCount.
func generateImages(ctx context.Context, user *config.UserConfig,
	model, prompt string, n int, size string) ([][]byte, error) {
	if n < 1 || n > maxImagesPerRequest {
		return nil, errors.Errorf("image count %d is outside 1..%d", n, maxImagesPerRequest)
	}
	if !strings.Contains(user.ImageUrl, "openai.azure.com") {
		imgs, err := fetchImageFromOpenaiDalle(ctx, user, model, prompt, n, size)
		if err != nil {
			return nil, errors.Wrap(err, "fetch openai images")
		}
		return imgs, nil
	}

	imgs := make([][]byte, n)
	var pool errgroup.Group
	for i := range n {
		pool.Go(func() (err error) {
			imgs[i], err = fetchImageFromAzureDalle(ctx, user, prompt)
			return err
		})
	}
	if err := pool.Wait(); err != nil {
		return nil, errors.Wrap(err, "fetch azure images")
	}
	return imgs, nil
}

// maxImageResponseBytesPerImage is the answer allowance for one image. The largest
// image a client may request through size is 4096x4096; as an incompressible RGBA
// PNG that is 64 MiB, which base64 inflates to 85.3 MiB. 96 MiB covers that plus
// JSON overhead. It is a variable so tests can lower it.
var maxImageResponseBytesPerImage int64 = 96 << 20

// readImageAPIResponse reads the body of an image API answer from which the call
// may take up to images images, refusing one larger than images times
// maxImageResponseBytesPerImage so an upstream cannot make the server buffer an
// unbounded body. images is clamped to 1..maxImagesPerRequest.
//
// A call asking for a batch passes maxImagesPerRequest rather than the requested
// count, because an answer may legitimately carry surplus entries that
// imageCollector ignores. The Azure path sends one call per image concurrently and
// passes 1, so every image request buffers at most maxImagesPerRequest allowances
// of upstream answers in total.
//
// It returns the body, or an error when reading fails or the body exceeds the cap.
func readImageAPIResponse(body io.Reader, images int) ([]byte, error) {
	limit := maxImageResponseBytesPerImage * int64(min(max(images, 1), maxImagesPerRequest))
	payload, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, errors.Wrap(err, "read image API response")
	}
	if int64(len(payload)) > limit {
		return nil, errors.Errorf("image API response exceeds %d bytes", limit)
	}

	return payload, nil
}

// imageCollector gathers the images decoded or downloaded from the entries of an
// image API answer, up to a limit. Entries without image data (for example one
// carrying only revised_prompt) do not count, and callers stop processing entries
// once the collector is full, so nothing beyond the limit is decoded or downloaded.
type imageCollector struct {
	limit int
	imgs  [][]byte
}

// newImageCollector returns a collector for at most limit images; a limit below 1
// counts as 1.
func newImageCollector(limit int) *imageCollector {
	return &imageCollector{limit: max(limit, 1)}
}

// add records img when it is not empty and reports whether the collector now holds
// limit images, in which case the caller must stop processing entries.
func (c *imageCollector) add(img []byte) (full bool) {
	if len(img) > 0 {
		c.imgs = append(c.imgs, img)
	}
	return len(c.imgs) >= c.limit
}

// images returns the collected images in entry order, or an error when no entry
// carried an image, so an empty answer is never reported as a success.
func (c *imageCollector) images() ([][]byte, error) {
	if len(c.imgs) == 0 {
		return nil, errors.New("image API response contains no image")
	}

	return c.imgs, nil
}
