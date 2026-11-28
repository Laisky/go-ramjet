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
	require.Equal(t, int64(5500), f.totalCharged())
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
	require.Equal(t, int64(15500), f.totalCharged())
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
	require.Equal(t, int64(15500), f.totalCharged())
}

// TestImageResponseAllowanceFitsLargestImage verifies the per-image answer allowance
// holds the largest image a client may request through size: an incompressible
// 4096x4096 RGBA PNG, base64-encoded, plus JSON overhead.
func TestImageResponseAllowanceFitsLargestImage(t *testing.T) {
	largestImage := base64.StdEncoding.EncodedLen(4096 * 4096 * 4)
	require.Greater(t, maxImageResponseBytesPerImage, int64(largestImage)+1<<20)
}

// azureImageUser returns a paid user whose image URL selects the Azure path and
// points at the fixture server.
func azureImageUser() *config.UserConfig {
	user := billedImageUser(false)
	user.ImageUrl = config.Config.API + "/openai.azure.com/images"
	return user
}

// TestAzureImageAnswersAreBoundedPerImage verifies that each Azure answer, which
// carries one image URL, is held to one image's allowance. The Azure path sends one
// request per image concurrently, so a per-answer cap sized for a whole batch would
// let one request buffer maxImagesPerRequest times the batch cap.
func TestAzureImageAnswersAreBoundedPerImage(t *testing.T) {
	origLimit := maxImageResponseBytesPerImage
	t.Cleanup(func() { maxImageResponseBytesPerImage = origLimit })
	maxImageResponseBytesPerImage = 4 << 10

	var downloads atomic.Int32
	f := &imageBillingFixture{}
	gctx := f.install(t)
	serverURL := config.Config.API
	f.override = func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.URL.Path == "/openai.azure.com/images":
			// Twice the one-image allowance, far below the batch cap.
			answer, err := json.Marshal(map[string]any{"data": []map[string]string{{
				"url":            serverURL + "/generated/azure.png",
				"revised_prompt": strings.Repeat("a", 8<<10),
			}}})
			if err != nil {
				t.Errorf("encode azure answer: %v", err)
				return true
			}
			// A bounded client hangs up once the cap is reached, so the write may fail.
			if _, err := w.Write(answer); err != nil {
				t.Logf("oversized azure answer cut off: %v", err)
			}
			return true
		case strings.HasPrefix(r.URL.Path, "/generated/"):
			downloads.Add(1)
			f.writeBody(w, []byte("synthetic-image"))
			return true
		}
		return false
	}

	err := sendChatImageGeneration(gctx, imageCountTestRequest(2), azureImageUser())
	require.ErrorContains(t, err, "exceeds")
	require.Zero(t, downloads.Load(), "an oversized Azure answer was accepted")
	require.Zero(t, f.delivered())
}

// TestGenerateImagesRefusesUnboundedCount verifies generateImages enforces the image
// count bound itself, before sizing any allocation or calling the upstream, so a
// caller that forgot normalizeImageCount cannot fan out a request-controlled count.
func TestGenerateImagesRefusesUnboundedCount(t *testing.T) {
	var calls atomic.Int32
	f := &imageBillingFixture{}
	f.install(t)
	f.override = func(http.ResponseWriter, *http.Request) bool {
		calls.Add(1)
		return false
	}

	for _, n := range []int{0, -1, maxImagesPerRequest + 1, hugeImageCount} {
		for _, user := range []*config.UserConfig{billedImageUser(false), azureImageUser()} {
			_, err := generateImages(t.Context(), user, "dall-e-3", "draw a tree", n, "")
			require.ErrorContains(t, err, "outside", "count %d", n)
		}
	}
	require.Zero(t, calls.Load(), "an unbounded count reached the upstream")
}

// TestDrawByDalleHandlerCurrentModelTariffs verifies the model ID and charge used by the generation endpoint.
func TestDrawByDalleHandlerCurrentModelTariffs(t *testing.T) {
	for _, tt := range []struct {
		model string
		cost  int64
	}{
		{"black-forest-labs/flux-2-dev", 6000},
		{"black-forest-labs/flux-dev", 12500},
		{"flux-dev", 12500},
		{"dall-e-3", 15500},
	} {
		t.Run(tt.model, func(t *testing.T) {
			f := &imageBillingFixture{produced: 1}
			gctx := f.install(t)
			req, err := json.Marshal(map[string]any{"model": tt.model, "prompt": "draw a tree", "n": 1})
			require.NoError(t, err)
			gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/images/generations", bytes.NewReader(req))
			gctx.Request.Header.Set("Content-Type", "application/json")
			gctx.Set(ctxKeyUser, billedImageUser(false))
			var requestModel atomic.Value
			f.override = func(_ http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path == "/v1/images/generations" {
					var req struct {
						Model string `json:"model"`
					}
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Errorf("decode fixture request: %v", err)
					}
					requestModel.Store(req.Model)
				}
				return false
			}

			DrawByDalleHandler(gctx)
			require.Equal(t, http.StatusOK, gctx.Writer.Status())
			require.Equal(t, tt.model, requestModel.Load())
			require.Equal(t, 1, f.delivered())
			require.Equal(t, tt.cost, f.totalCharged())
		})
	}
}

// TestDrawByDalleHandlerDelegatesTokenBilling verifies that the legacy external
// billing option cannot add a local charge to a native One API inference.
func TestDrawByDalleHandlerDelegatesTokenBilling(t *testing.T) {
	f := &imageBillingFixture{produced: 1}
	gctx := f.install(t)
	relay := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/images/generations" {
			t.Errorf("unexpected relay operation: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected operation", http.StatusBadRequest)
			return
		}
		f.handler(w, r)
	}))
	t.Cleanup(relay.Close)
	httpcli = relay.Client()
	config.Config.ExternalBillingAPI = relay.URL
	user := billedImageUser(false)
	user.APIBase = relay.URL
	gctx.Set(ctxKeyUser, user)
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/images/generations", strings.NewReader(
		`{"model":"gpt-image-2.6-sunburst","prompt":"draw a tree","n":1}`))
	gctx.Request.Header.Set("Content-Type", "application/json")

	DrawByDalleHandler(gctx)
	require.Equal(t, http.StatusOK, gctx.Writer.Status())
	require.Equal(t, 1, f.delivered())
	require.Zero(t, f.totalCharged())
}

// TestDrawByDalleHandlerRequiresPublishedPrice ensures unknown and ambiguous
// direct-provider prices do not charge the account or invoke the image provider.
func TestDrawByDalleHandlerRequiresPublishedPrice(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"token price only", `{"success":true,"data":{"a":{"models":{"gpt-image-2.6-sunburst":{"output_price":30}}}}}`},
		{"ambiguous", `{"success":true,"data":{"a":{"models":{"gpt-image-2.6-sunburst":{"image_price":0.01}}},"b":{"models":{"gpt-image-2.6-sunburst":{"image_price":0.02}}}}}`},
		{"unavailable", `{"success":false,"data":{}}`},
		{"malformed", `{"success":true,"data":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &imageBillingFixture{produced: 1}
			gctx := f.install(t)
			var generated atomic.Int32
			f.override = func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path == "/api/models/display" {
					f.writeBody(w, []byte(tc.body))
					return true
				}
				if r.URL.Path == "/v1/images/generations" {
					generated.Add(1)
				}
				return false
			}
			gctx.Set(ctxKeyUser, billedImageUser(false))
			gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/images/generations", strings.NewReader(
				`{"model":"gpt-image-2.6-sunburst","prompt":"draw a tree","n":1}`))
			gctx.Request.Header.Set("Content-Type", "application/json")

			DrawByDalleHandler(gctx)
			require.GreaterOrEqual(t, gctx.Writer.Status(), http.StatusBadRequest)
			require.Zero(t, generated.Load())
			require.Zero(t, f.totalCharged())
			require.Zero(t, f.delivered())
		})
	}
}
