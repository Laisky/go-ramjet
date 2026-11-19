package http

import (
	"context"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"

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

// imageBatchPrice returns the charge for n images of model. Image prices are per
// image (db.PriceTxt2Image is one DALL-E image, and Flux bills every generated
// image), so the charge scales with n.
func imageBatchPrice(model string, n int) db.Price {
	return GetImageModelPrice(model) * db.Price(n)
}

// settleImageBilling reconciles a DALL-E request's charge with what the upstream
// produced. One image is charged upfront, which doubles as the balance probe; this
// charges the remaining delivered images in one call once generation succeeded.
//
// Parameters:
//   - ctx: Request context carrying the request logger.
//   - user: User being billed.
//   - model: Image model, which selects the per-image price.
//   - n: Normalized number of requested images; never more are delivered.
//   - imgs: Images the upstream produced.
//
// It returns the images to deliver: at most n of imgs. When the charge for the
// extra images fails, only the prepaid first image is delivered and the failure is
// logged once at error level; like Flux, unbilled images are never exposed.
func settleImageBilling(ctx context.Context, user *config.UserConfig, model string, n int, imgs [][]byte) [][]byte {
	if len(imgs) > n {
		imgs = imgs[:n]
	}
	extra := len(imgs) - 1
	if !user.EnableExternalImageBilling || extra <= 0 {
		return imgs
	}

	if err := checkUserExternalBilling(ctx, user, imageBatchPrice(model, extra), "txt2image"); err != nil {
		gmw.GetLogger(ctx).Error("charge for extra generated images failed; delivering only the prepaid image",
			zap.String("model", model), zap.Int("produced", len(imgs)), zap.Error(err))
		return imgs[:1]
	}

	return imgs
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
