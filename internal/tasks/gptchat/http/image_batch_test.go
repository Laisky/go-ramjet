package http

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// editTestRequest points gctx at the image edit endpoint with a one-pixel PNG
// source image and a paid user with external image billing.
func editTestRequest(t *testing.T, gctx *gin.Context) {
	t.Helper()
	var src bytes.Buffer
	require.NoError(t, png.Encode(&src, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	body, err := json.Marshal(map[string]string{
		"prompt": "make it blue",
		"model":  "gpt-image-1",
		"image":  base64.StdEncoding.EncodeToString(src.Bytes()),
	})
	require.NoError(t, err)
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/images/edits", bytes.NewReader(body))
	gctx.Request.Header.Set("Content-Type", "application/json")
	gctx.Set(ctxKeyUser, billedImageUser(false))
}

// TestEditImageHandlerFetchesOnlyOneImage verifies that the image edit endpoint,
// which charges one image, delivers one image and never downloads the surplus
// entries an upstream adds to its answer.
func TestEditImageHandlerFetchesOnlyOneImage(t *testing.T) {
	const returned = 40
	var downloads atomic.Int32
	f := &imageBillingFixture{}
	gctx := f.install(t)
	editTestRequest(t, gctx)
	f.override = func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.URL.Path == "/v1/images/edits":
			data := make([]map[string]string, returned)
			for i := range data {
				data[i] = map[string]string{"url": "http://" + r.Host + "/edited/" + strconv.Itoa(i) + ".png"}
			}
			f.writeJSON(w, map[string]any{"data": data})
			return true
		case strings.HasPrefix(r.URL.Path, "/edited/"):
			downloads.Add(1)
			f.writeBody(w, []byte("synthetic-image"))
			return true
		}
		return false
	}

	EditImageHandler(gctx)
	require.Equal(t, http.StatusOK, gctx.Writer.Status())
	require.Equal(t, 1, f.delivered())
	require.Equal(t, int64(GetImageModelPrice("gpt-image-1")), f.totalCharged())
	require.LessOrEqual(t, downloads.Load(), int32(1), "surplus edited images were downloaded")
}

// TestEditImageHandlerFailsWithoutImage verifies that an edit answer whose entries
// carry no image data is reported as a failure instead of an empty success.
func TestEditImageHandlerFailsWithoutImage(t *testing.T) {
	f := &imageBillingFixture{empty: 2}
	gctx := f.install(t)
	editTestRequest(t, gctx)

	EditImageHandler(gctx)
	require.GreaterOrEqual(t, gctx.Writer.Status(), http.StatusBadRequest)
	require.Zero(t, f.delivered())
}

// TestImageResponsesAreSizeBounded verifies that the generation and edit paths stop
// reading an image API answer once it exceeds the fixed cap (maxImagesPerRequest
// times the per-image allowance), and report that clearly instead of buffering
// the whole body.
func TestImageResponsesAreSizeBounded(t *testing.T) {
	origLimit := maxImageResponseBytesPerImage
	t.Cleanup(func() { maxImageResponseBytesPerImage = origLimit })
	maxImageResponseBytesPerImage = 4 << 10

	// Two 32 KiB base64 images exceed the fixed cap of maxImagesPerRequest (8)
	// times the lowered 4 KiB allowance.
	oversized := func(f *imageBillingFixture) func(http.ResponseWriter, *http.Request) bool {
		return func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path != "/v1/images/generations" && r.URL.Path != "/v1/images/edits" {
				return false
			}
			image := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 24<<10))
			f.writeJSON(w, map[string]any{"data": []map[string]string{{"b64_json": image}, {"b64_json": image}}})
			return true
		}
	}

	t.Run("generation", func(t *testing.T) {
		f := &imageBillingFixture{}
		gctx := f.install(t)
		f.override = oversized(f)

		err := sendChatImageGeneration(gctx, imageCountTestRequest(2), billedImageUser(false))
		require.ErrorContains(t, err, "exceeds")
		require.Zero(t, f.delivered())
	})

	t.Run("edit", func(t *testing.T) {
		f := &imageBillingFixture{}
		gctx := f.install(t)
		editTestRequest(t, gctx)
		f.override = oversized(f)

		EditImageHandler(gctx)
		require.GreaterOrEqual(t, gctx.Writer.Status(), http.StatusBadRequest)
		require.Zero(t, f.delivered())
	})
}

// TestDrawByDalleHandlerUndeliveredImagesStayUnguessable verifies that when the
// charge for the extra images fails after they were stored, the client gets only
// the prepaid image, and its URL does not reveal the keys of the stored but unpaid
// images: every image key carries its own random suffix.
func TestDrawByDalleHandlerUndeliveredImagesStayUnguessable(t *testing.T) {
	f := &imageBillingFixture{produced: 3, failChargeN: 2}
	gctx := f.install(t)
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/images/generations",
		bytes.NewReader([]byte(`{"model":"dall-e-3","prompt":"draw a tree","n":3}`)))
	gctx.Request.Header.Set("Content-Type", "application/json")
	gctx.Set(ctxKeyUser, billedImageUser(false))

	DrawByDalleHandler(gctx)
	require.Equal(t, http.StatusOK, gctx.Writer.Status())
	require.Equal(t, int64(GetImageModelPrice("dall-e-3")), f.totalCharged())
	require.Equal(t, 3, f.stored(), "images are stored before the extra charge")

	urls := f.deliveredURLs()
	require.Len(t, urls, 1)
	var delivered int
	for _, key := range f.keys {
		if strings.Contains(urls[0], key+".png") {
			delivered++
			continue
		}
		suffix := key[strings.LastIndex(key, "-")+1:]
		require.GreaterOrEqual(t, len(suffix), 16, "image key %q lacks a random suffix", key)
		require.NotContains(t, urls[0], suffix, "the delivered URL reveals an unpaid image key")
	}
	require.Equal(t, 1, delivered, "the delivered URL must name one stored image")
}

// TestImageResponseCapToleratesSurplusEntries verifies that the answer size cap does
// not scale with the requested count: surplus entries, which the collector ignores,
// must not turn a valid one-image answer into a failure.
func TestImageResponseCapToleratesSurplusEntries(t *testing.T) {
	origLimit := maxImageResponseBytesPerImage
	t.Cleanup(func() { maxImageResponseBytesPerImage = origLimit })
	maxImageResponseBytesPerImage = 4 << 10

	f := &imageBillingFixture{}
	gctx := f.install(t)
	f.override = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/v1/images/generations" {
			return false
		}
		image := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 3<<10))
		f.writeJSON(w, map[string]any{"data": []map[string]string{{"b64_json": image}, {"b64_json": image}}})
		return true
	}

	require.NoError(t, sendChatImageGeneration(gctx, imageCountTestRequest(1), billedImageUser(false)))
	require.Equal(t, 1, f.delivered())
	require.Equal(t, int64(GetImageModelPrice("dall-e-3")), f.totalCharged())
}

// TestImageResponseAllowanceFitsLargestImage verifies the per-image answer allowance
// holds the largest image a client may request through size: an incompressible
// 4096x4096 RGBA PNG, base64-encoded, plus JSON overhead.
func TestImageResponseAllowanceFitsLargestImage(t *testing.T) {
	largestImage := base64.StdEncoding.EncodedLen(4096 * 4096 * 4)
	require.Greater(t, maxImageResponseBytesPerImage, int64(largestImage)+1<<20)
}
