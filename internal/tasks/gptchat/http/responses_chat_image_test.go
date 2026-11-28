package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// hugeImageCount is a request-controlled image count far beyond any sane limit.
const hugeImageCount = 1 << 40

// imageCountTestContext returns a chat request context for sendChatImageGeneration
// whose image upstream is served by handler; HTTP and configuration are isolated.
func imageCountTestContext(t *testing.T, handler http.HandlerFunc) *gin.Context {
	t.Helper()
	gctx, _ := imageTestContextWithRecorder(t, handler)
	return gctx
}

// imageTestContextWithRecorder is imageCountTestContext that also returns the
// recorder holding the response.
func imageTestContextWithRecorder(t *testing.T, handler http.HandlerFunc) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	logger, _, _ := embeddingTestSetup(t, server)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	gctx, _ := gin.CreateTestContext(rec)
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", nil)
	gmw.SetLogger(gctx, logger)
	return gctx, rec
}

// imageCountTestRequest builds an image-model chat request asking for n images.
func imageCountTestRequest(n int) *FrontendReq {
	return &FrontendReq{
		Model: "dall-e-3",
		N:     n,
		Messages: []FrontendReqMessage{{
			Role:    OpenaiMessageRoleUser,
			Content: FrontendReqMessageContent{StringContent: "draw a tree"},
		}},
	}
}

// TestSendChatImageGenerationBoundsAzureImageCount verifies a huge requested image
// count neither panics nor allocates per requested image, and fans out at most
// maxImagesPerRequest upstream calls.
func TestSendChatImageGenerationBoundsAzureImageCount(t *testing.T) {
	var calls atomic.Int32
	gctx := imageCountTestContext(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "synthetic azure failure", http.StatusBadGateway)
	})
	user := &config.UserConfig{
		Token: "sk-synthetic-image", ImageToken: "sk-synthetic-image", UserName: "image-tester",
		ImageUrl: config.Config.API + "/openai.azure.com/images", AllowedModels: []string{"*"},
	}

	require.NotPanics(t, func() {
		err := sendChatImageGeneration(gctx, imageCountTestRequest(hugeImageCount), user)
		require.Error(t, err)
	})
	require.Positive(t, calls.Load())
	require.LessOrEqual(t, calls.Load(), int32(maxImagesPerRequest))
}

// TestSendChatImageGenerationBoundsOpenAIImageCount verifies the image count sent
// upstream is clamped to 1..maxImagesPerRequest, and to 1 for free users.
func TestSendChatImageGenerationBoundsOpenAIImageCount(t *testing.T) {
	for _, tc := range []struct {
		name      string
		requested int
		free      bool
		want      int
	}{
		{name: "huge", requested: hugeImageCount, want: maxImagesPerRequest},
		{name: "within limit", requested: 3, want: 3},
		{name: "unset", requested: 0, want: 1},
		{name: "free user", requested: 5, free: true, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sentN atomic.Int64
			gctx := imageCountTestContext(t, func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					N int64 `json:"n"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "bad image payload: "+err.Error(), http.StatusBadRequest)
					return
				}
				sentN.Store(body.N)
				http.Error(w, "synthetic upstream failure", http.StatusBadGateway)
			})
			user := &config.UserConfig{
				Token: "sk-synthetic-image", OpenaiToken: "sk-synthetic-image", UserName: "image-tester",
				APIBase: config.Config.API, IsFree: tc.free, AllowedModels: []string{"*"},
			}

			err := sendChatImageGeneration(gctx, imageCountTestRequest(tc.requested), user)
			require.Error(t, err)
			require.Equal(t, int64(tc.want), sentN.Load())
		})
	}
}

// imageBillingFixture fakes the image upstream, the external billing API and the
// object store for the DALL-E paths, recording every charge and stored image.
type imageBillingFixture struct {
	produced    int  // images the upstream returns; 0 with empty 0 makes generation fail
	empty       int  // entries without image data (e.g. only revised_prompt) sent first
	failChargeN int  // 1-based billing call that is rejected; 0 accepts all
	failUpload  bool // every object store upload fails
	t           *testing.T
	rec         *httptest.ResponseRecorder
	// override, when set, serves a request before the default routes and reports
	// whether it did.
	override func(http.ResponseWriter, *http.Request) bool
	mu       sync.Mutex
	charges  []int64
	rejected int
	keys     []string // object keys of the stored images
}

// handler serves billing calls and OpenAI-style image generations.
func (f *imageBillingFixture) handler(w http.ResponseWriter, r *http.Request) {
	if f.override != nil && f.override(w, r) {
		return
	}
	switch r.URL.Path {
	case "/api/models/display":
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" {
			http.Error(w, "catalog must be read without user credentials", http.StatusBadRequest)
			return
		}
		f.writeJSON(w, map[string]any{"success": true, "data": map[string]any{
			"fixture": map[string]any{"models": map[string]any{
				"dall-e-3":                     map[string]any{"image_price": 0.031},
				"gpt-image-1":                  map[string]any{"image_price": 0.011},
				"black-forest-labs/flux-2-dev": map[string]any{"image_price": 0.012},
				"black-forest-labs/flux-dev":   map[string]any{"image_price": 0.025},
				"gpt-image-2.6-flare":          map[string]any{"image_price": 0.019},
			}},
		}})
	case "/api/token/consume":
		var usage struct {
			Cost int64 `json:"add_used_quota"`
		}
		if err := json.NewDecoder(r.Body).Decode(&usage); err != nil {
			http.Error(w, "bad billing payload: "+err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failChargeN == len(f.charges)+f.rejected+1 {
			f.rejected++
			http.Error(w, "insufficient balance", http.StatusForbidden)
			return
		}
		f.charges = append(f.charges, usage.Cost)
		f.writeJSON(w, map[string]any{})
	case "/v1/images/generations", "/v1/images/edits":
		if f.produced == 0 && f.empty == 0 {
			http.Error(w, "n must be 1 for this model", http.StatusBadRequest)
			return
		}
		data := make([]map[string]string, 0, f.empty+f.produced)
		for range f.empty {
			data = append(data, map[string]string{"revised_prompt": "a tree"})
		}
		for range f.produced {
			data = append(data, map[string]string{"b64_json": base64.StdEncoding.EncodeToString([]byte("synthetic-image"))})
		}
		f.writeJSON(w, map[string]any{"data": data})
	default:
		http.Error(w, "unexpected path", http.StatusNotFound)
	}
}

// writeJSON encodes v as the response body and reports an encoding or write
// failure on the fixture's test.
func (f *imageBillingFixture) writeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		f.t.Errorf("write fixture response: %v", err)
	}
}

// writeBody writes body as the response and reports a write failure on the
// fixture's test.
func (f *imageBillingFixture) writeBody(w http.ResponseWriter, body []byte) {
	if _, err := w.Write(body); err != nil {
		f.t.Errorf("write fixture response: %v", err)
	}
}

// totalCharged returns the sum of all accepted charges.
func (f *imageBillingFixture) totalCharged() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	var sum int64
	for _, c := range f.charges {
		sum += c
	}
	return sum
}

// stored returns how many images were uploaded to the object store.
func (f *imageBillingFixture) stored() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.keys)
}

// generatedImageURLRegexp matches the image URLs a response hands to the client.
var generatedImageURLRegexp = regexp.MustCompile(`https://[^\s"()\[\]]+\.png`)

// deliveredURLs returns the distinct image URLs the response hands to the client,
// whether as JSON fields or as Markdown links in a chat answer.
func (f *imageBillingFixture) deliveredURLs() []string {
	var urls []string
	for _, url := range generatedImageURLRegexp.FindAllString(f.rec.Body.String(), -1) {
		if !slices.Contains(urls, url) {
			urls = append(urls, url)
		}
	}
	return urls
}

// delivered returns how many distinct images the response hands to the client.
func (f *imageBillingFixture) delivered() int {
	return len(f.deliveredURLs())
}

// install wires the fixture into an isolated request context and stubs uploads.
func (f *imageBillingFixture) install(t *testing.T) *gin.Context {
	t.Helper()
	f.t = t
	gctx, rec := imageTestContextWithRecorder(t, f.handler)
	f.rec = rec
	config.Config.ExternalBillingAPI = config.Config.API
	origUpload := uploadGeneratedImage
	t.Cleanup(func() { uploadGeneratedImage = origUpload })
	uploadGeneratedImage = func(_ context.Context, key, _ string, _ []byte, _ string) error {
		if f.failUpload {
			return errors.New("synthetic object store failure")
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.keys = append(f.keys, key)
		return nil
	}
	return gctx
}

// billedImageUser returns a paid user with external image billing enabled.
func billedImageUser(free bool) *config.UserConfig {
	return &config.UserConfig{
		Token: "sk-synthetic-image", OpenaiToken: "sk-synthetic-image", ImageToken: "sk-synthetic-image",
		UserName: "image-tester", APIBase: config.Config.API, IsFree: free,
		AllowedModels: []string{"*"}, NoLimitExpensiveModels: true, EnableExternalImageBilling: true,
	}
}

// imageBillingCases pin how DALL-E requests are charged against what was produced:
// one image upfront (the balance probe), the rest only once delivered.
var imageBillingCases = []struct {
	name        string
	requested   int
	free        bool
	produced    int
	empty       int
	failChargeN int
	failUpload  bool
	billed      int
	delivered   int
}{
	{name: "eight requested, eight delivered", requested: 8, produced: 8, billed: 8, delivered: 8},
	{name: "eight requested, one delivered", requested: 8, produced: 1, billed: 1, delivered: 1},
	// Pre-existing: the prepaid image is not refunded when nothing is delivered; the
	// billing API has no refund call (owner decision).
	{name: "upstream rejects the request (prepaid image kept)", requested: 8, produced: 0, billed: 1, delivered: 0},
	{name: "upstream returns more than requested", requested: 2, produced: 5, billed: 2, delivered: 2},
	{name: "huge count is clamped", requested: hugeImageCount, produced: 8, billed: 8, delivered: 8},
	{name: "free user", requested: 5, free: true, produced: 1, billed: 1, delivered: 1},
	{name: "extra charge rejected keeps only the prepaid image", requested: 3, produced: 3, failChargeN: 2, billed: 1, delivered: 1},
	{name: "an entry without image data does not count", requested: 2, empty: 1, produced: 2, billed: 2, delivered: 2},
	{name: "leading entries without image data hide nothing", requested: 2, empty: 2, produced: 2, billed: 2, delivered: 2},
	{name: "no entry carries an image (prepaid image kept)", requested: 2, empty: 2, billed: 1, delivered: 0},
	{name: "upload fails, so the extra images are not charged (prepaid image kept)", requested: 3, produced: 3, failUpload: true, billed: 1, delivered: 0},
}

// TestSendChatImageGenerationBillsDeliveredImages verifies the image-model chat path
// charges one image upfront and the remaining delivered images after success.
func TestSendChatImageGenerationBillsDeliveredImages(t *testing.T) {
	for _, tc := range imageBillingCases {
		t.Run(tc.name, func(t *testing.T) {
			f := &imageBillingFixture{produced: tc.produced, empty: tc.empty, failChargeN: tc.failChargeN, failUpload: tc.failUpload}
			gctx := f.install(t)

			err := sendChatImageGeneration(gctx, imageCountTestRequest(tc.requested), billedImageUser(tc.free))
			if tc.delivered == 0 {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, int64(15500)*int64(tc.billed), f.totalCharged())
			require.Equal(t, tc.delivered, f.delivered())
		})
	}
}

// TestDrawByDalleHandlerBillsDeliveredImages verifies the image generation endpoint
// charges one image upfront and the remaining delivered images after success.
func TestDrawByDalleHandlerBillsDeliveredImages(t *testing.T) {
	for _, tc := range imageBillingCases {
		t.Run(tc.name, func(t *testing.T) {
			f := &imageBillingFixture{produced: tc.produced, empty: tc.empty, failChargeN: tc.failChargeN, failUpload: tc.failUpload}
			gctx := f.install(t)
			body, err := json.Marshal(map[string]any{"model": "dall-e-3", "prompt": "draw a tree", "n": tc.requested})
			require.NoError(t, err)
			gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/images/generations", bytes.NewReader(body))
			gctx.Request.Header.Set("Content-Type", "application/json")
			gctx.Set(ctxKeyUser, billedImageUser(tc.free))

			DrawByDalleHandler(gctx)
			if tc.delivered == 0 {
				require.GreaterOrEqual(t, gctx.Writer.Status(), http.StatusBadRequest, "a request without images must fail")
			} else {
				require.Equal(t, http.StatusOK, gctx.Writer.Status())
			}
			require.Equal(t, int64(15500)*int64(tc.billed), f.totalCharged())
			require.Equal(t, tc.delivered, f.delivered())
		})
	}
}

// TestSendChatImageGenerationSettlesFreeUserReservation verifies a free user's
// image-model chat request settles its token reservation like the text paths do.
// Unsettled, the full max_tokens estimate stays charged to the user's 10-minute
// quota window although the answer is a few Markdown image links.
func TestSendChatImageGenerationSettlesFreeUserReservation(t *testing.T) {
	isolateResponsesChatGlobals(t)
	quotaHook := setupFreeUserTestQuota(t)
	f := &imageBillingFixture{produced: 1}
	gctx := f.install(t)
	config.Config.RateLimitExpensiveModelsIntervalSeconds = 600

	user := &config.UserConfig{
		Token: "FREETIER-abcdefgh", UserName: "free-user", IsFree: true,
		APIBase: config.Config.API, OpenaiToken: "sk-shared", ImageToken: "sk-shared",
		AllowedModels: []string{"*"},
	}
	gctx.Set(ctxKeyUser, user)
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", bytes.NewReader([]byte(
		`{"model":"dall-e-3","stream":false,"max_tokens":500,"messages":[{"role":"user","content":"draw a tree"}]}`)))
	gctx.Request.Header.Set("Content-Type", "application/json")

	require.NoError(t, sendChatWithResponsesToolLoop(gctx))
	require.Equal(t, 1, f.delivered())
	require.Positive(t, quotaHook.reserved.Load(), "free-user image request must reserve quota")
	require.Equal(t, int32(1), quotaHook.adjustments.Load(), "token reservation was never settled")
	require.Less(t, quotaHook.net.Load(), quotaHook.reserved.Load(),
		"the unused max_tokens estimate must be released")
	require.Nil(t, getTokenReservation(gctx))
}

// TestDrawByDalleHandlerFetchesOnlyRequestedImages verifies that an upstream answering
// with more images than requested cannot make the server download (or decode) more
// than the requested count: surplus entries are dropped before any of them is fetched.
func TestDrawByDalleHandlerFetchesOnlyRequestedImages(t *testing.T) {
	const requested, returned = 2, 40
	var downloads atomic.Int32
	f := &imageBillingFixture{}
	gctx := f.install(t)
	serverURL := config.Config.API
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/images/generations",
		bytes.NewReader([]byte(`{"model":"dall-e-3","prompt":"draw a tree","n":2}`)))
	gctx.Request.Header.Set("Content-Type", "application/json")
	gctx.Set(ctxKeyUser, billedImageUser(false))

	// Serve every surplus entry as a URL so each processed entry costs one download.
	f.override = func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.URL.Path == "/v1/images/generations":
			data := make([]map[string]string, returned)
			for i := range data {
				data[i] = map[string]string{"url": serverURL + "/generated/" + strconv.Itoa(i) + ".png"}
			}
			f.writeJSON(w, map[string]any{"data": data})
			return true
		case strings.HasPrefix(r.URL.Path, "/generated/"):
			downloads.Add(1)
			f.writeBody(w, []byte("synthetic-image"))
			return true
		}
		return false
	}

	DrawByDalleHandler(gctx)
	require.Equal(t, requested, f.delivered())
	require.Equal(t, int64(15500)*requested, f.totalCharged())
	require.LessOrEqual(t, downloads.Load(), int32(requested), "surplus upstream images were downloaded")
}
