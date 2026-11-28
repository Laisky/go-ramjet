package http

import (
	"context"
	"fmt"
	"io"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v6"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
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

// generatedImageKey returns a new object key, without extension, for the i-th
// image of task taskID. Each key ends in its own cryptographically random suffix,
// so one URL does not reveal another image key from the same task.
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

// storeGeneratedImages uploads up to n generated images under unique task keys.
// It returns their public URLs in input order, or an error if any upload fails.
func storeGeneratedImages(taskCtx context.Context, n int, taskID, prompt string, imgs [][]byte) ([]string, error) {
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

	urls := make([]string, len(keys))
	for i, key := range keys {
		urls[i] = generatedImageURL(key)
	}

	return urls, nil
}

// generateImages asks user's image upstream for n images of model drawn from
// prompt, at size when it is not empty.
//
// Parameters:
//   - ctx: Context bounding every upstream call.
//   - user: User whose image upstream and credentials are used.
//   - model: Image model.
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
	return fetchImageFromOpenaiDalle(ctx, user, model, prompt, n, size)
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
// imageCollector ignores.
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
