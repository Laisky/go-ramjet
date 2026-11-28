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

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// editTestRequest points gctx at the image edit endpoint with a one-pixel PNG
// source image and a synthetic BYOK user.
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
	gctx.Set(ctxKeyUser, inferenceImageUser(false))
}

// TestEditImageHandlerFetchesOnlyOneImage verifies that the image edit endpoint,
// delivers one image and never downloads the surplus
// entries an upstream adds to its answer.
func TestEditImageHandlerFetchesOnlyOneImage(t *testing.T) {
	const returned = 40
	var downloads atomic.Int32
	f := &imageInferenceFixture{}
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
	require.LessOrEqual(t, downloads.Load(), int32(1), "surplus edited images were downloaded")
}

// TestEditImageHandlerFailsWithoutImage verifies that an edit answer whose entries
// carry no image data is reported as a failure instead of an empty success.
func TestEditImageHandlerFailsWithoutImage(t *testing.T) {
	f := &imageInferenceFixture{empty: 2}
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
	oversized := func(f *imageInferenceFixture) func(http.ResponseWriter, *http.Request) bool {
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
		f := &imageInferenceFixture{}
		gctx := f.install(t)
		f.override = oversized(f)

		err := sendChatImageGeneration(gctx, imageCountTestRequest(2), inferenceImageUser(false))
		require.ErrorContains(t, err, "exceeds")
		require.Zero(t, f.delivered())
	})

	t.Run("edit", func(t *testing.T) {
		f := &imageInferenceFixture{}
		gctx := f.install(t)
		editTestRequest(t, gctx)
		f.override = oversized(f)

		EditImageHandler(gctx)
		require.GreaterOrEqual(t, gctx.Writer.Status(), http.StatusBadRequest)
		require.Zero(t, f.delivered())
	})
}

// TestImageResponseCapToleratesSurplusEntries verifies that the answer size cap does
// not scale with the requested count: surplus entries, which the collector ignores,
// must not turn a valid one-image answer into a failure.
func TestImageResponseCapToleratesSurplusEntries(t *testing.T) {
	origLimit := maxImageResponseBytesPerImage
	t.Cleanup(func() { maxImageResponseBytesPerImage = origLimit })
	maxImageResponseBytesPerImage = 4 << 10

	f := &imageInferenceFixture{}
	gctx := f.install(t)
	f.override = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/v1/images/generations" {
			return false
		}
		image := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 3<<10))
		f.writeJSON(w, map[string]any{"data": []map[string]string{{"b64_json": image}, {"b64_json": image}}})
		return true
	}

	require.NoError(t, sendChatImageGeneration(gctx, imageCountTestRequest(1), inferenceImageUser(false)))
	require.Equal(t, 1, f.delivered())
}

// TestImageResponseAllowanceFitsLargestImage verifies the per-image answer allowance
// holds the largest image a client may request through size: an incompressible
// 4096x4096 RGBA PNG, base64-encoded, plus JSON overhead.
func TestImageResponseAllowanceFitsLargestImage(t *testing.T) {
	largestImage := base64.StdEncoding.EncodedLen(4096 * 4096 * 4)
	require.Greater(t, maxImageResponseBytesPerImage, int64(largestImage)+1<<20)
}

// TestGenerateImagesRefusesUnboundedCount verifies generateImages enforces the image
// count bound itself, before sizing any allocation or calling the upstream, so a
// caller that forgot normalizeImageCount cannot fan out a request-controlled count.
func TestGenerateImagesRefusesUnboundedCount(t *testing.T) {
	var calls atomic.Int32
	f := &imageInferenceFixture{}
	f.install(t)
	f.override = func(http.ResponseWriter, *http.Request) bool {
		calls.Add(1)
		return false
	}

	for _, n := range []int{0, -1, maxImagesPerRequest + 1, hugeImageCount} {
		for _, user := range []*config.UserConfig{inferenceImageUser(false), inferenceImageUser(true)} {
			_, err := generateImages(t.Context(), user, "dall-e-3", "draw a tree", n, "")
			require.ErrorContains(t, err, "outside", "count %d", n)
		}
	}
	require.Zero(t, calls.Load(), "an unbounded count reached the upstream")
}

// TestEditImageHandlerDelegatesInference verifies that editing sends only the
// selected upstream request with the user's credential and returns the image.
func TestEditImageHandlerDelegatesInference(t *testing.T) {
	for _, free := range []bool{true, false} {
		name := "caller-owned"
		if free {
			name = "managed"
		}
		t.Run(name, func(t *testing.T) {
			f := &imageInferenceFixture{produced: 1}
			gctx := f.install(t)
			editTestRequest(t, gctx)
			user := inferenceImageUser(free)
			gctx.Set(ctxKeyUser, user)

			EditImageHandler(gctx)

			require.Equal(t, http.StatusOK, gctx.Writer.Status())
			require.Equal(t, []string{"Bearer " + user.OpenaiToken}, f.authorizations())
			require.Equal(t, 1, f.delivered())
			require.Equal(t, 1, f.stored())
		})
	}
}

// TestDrawByDalleHandlerKeepsIndependentImageKeys verifies each stored object
// has an independent random suffix and every successful upload is returned.
func TestDrawByDalleHandlerKeepsIndependentImageKeys(t *testing.T) {
	f := &imageInferenceFixture{produced: 3}
	gctx := f.install(t)
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/images/generations",
		strings.NewReader(`{"model":"dall-e-3","prompt":"draw a tree","n":3}`))
	gctx.Request.Header.Set("Content-Type", "application/json")
	gctx.Set(ctxKeyUser, inferenceImageUser(false))

	DrawByDalleHandler(gctx)

	require.Equal(t, http.StatusOK, gctx.Writer.Status())
	require.Equal(t, 3, f.stored())
	require.Equal(t, 3, f.delivered())
	suffixes := map[string]struct{}{}
	for _, key := range f.keys {
		suffix := key[strings.LastIndex(key, "-")+1:]
		require.Len(t, suffix, 20)
		require.NotContains(t, suffixes, suffix)
		suffixes[suffix] = struct{}{}
	}
}

// TestManagedMediaUsesDeclaredOneAPI verifies the managed key reaches only the
// declared relay, not the obsolete image URL or a different managed base.
func TestManagedMediaUsesDeclaredOneAPI(t *testing.T) {
	var alternateCalls atomic.Int32
	alternate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		alternateCalls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(alternate.Close)
	f := &imageInferenceFixture{produced: 1}
	gctx := f.install(t)
	user := inferenceImageUser(true)
	grantInferenceBudget(gctx, user, "dall-e-3", 1)

	images, err := generateImages(gctx.Request.Context(), user, "dall-e-3", "draw a tree", 1, "")
	require.NoError(t, err)
	require.Len(t, images, 1)
	require.Equal(t, []string{"Bearer " + user.OpenaiToken}, f.authorizations())
	require.Zero(t, alternateCalls.Load())

	user.APIBase = alternate.URL
	_, err = generateImages(gctx.Request.Context(), user, "dall-e-3", "draw a tree", 1, "")
	require.ErrorContains(t, err, "requires the configured One API endpoint")
	_, err = requestSpeech(gctx, user, "hello")
	require.ErrorContains(t, err, "requires the configured One API endpoint")
	require.Zero(t, alternateCalls.Load())
	require.Len(t, f.authorizations(), 1)
}
