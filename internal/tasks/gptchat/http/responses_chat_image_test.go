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
	config.Config.OneAPIBase = server.URL

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

// imageInferenceFixture serves only image inference and records uploaded objects.
type imageInferenceFixture struct {
	produced int // images the upstream returns; 0 with empty 0 makes generation fail
	empty    int // entries without image data (e.g. only revised_prompt) sent first

	failUpload bool // every object store upload fails
	t          *testing.T
	rec        *httptest.ResponseRecorder
	// override, when set, serves a request before the default routes and reports
	// whether it did.
	override func(http.ResponseWriter, *http.Request) bool
	mu       sync.Mutex
	auth     []string

	keys []string // object keys of the stored images
}

// handler serves OpenAI-style image inference. Any unexpected route fails the test.
func (f *imageInferenceFixture) handler(w http.ResponseWriter, r *http.Request) {
	if f.override != nil && f.override(w, r) {
		return
	}
	f.mu.Lock()
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	f.mu.Unlock()
	switch r.URL.Path {
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
		f.t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected path", http.StatusNotFound)
	}
}

// writeJSON encodes v as the response body and reports an encoding or write
// failure on the fixture's test.
func (f *imageInferenceFixture) writeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		f.t.Errorf("write fixture response: %v", err)
	}
}

// writeBody writes body as the response and reports a write failure on the
// fixture's test.
func (f *imageInferenceFixture) writeBody(w http.ResponseWriter, body []byte) {
	if _, err := w.Write(body); err != nil {
		f.t.Errorf("write fixture response: %v", err)
	}
}

// stored returns how many images were uploaded to the object store.
func (f *imageInferenceFixture) stored() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.keys)
}

// generatedImageURLRegexp matches the image URLs a response hands to the client.
var generatedImageURLRegexp = regexp.MustCompile(`https://[^\s"()\[\]]+\.png`)

// deliveredURLs returns the distinct image URLs the response hands to the client,
// whether as JSON fields or as Markdown links in a chat answer.
func (f *imageInferenceFixture) deliveredURLs() []string {
	var urls []string
	for _, url := range generatedImageURLRegexp.FindAllString(f.rec.Body.String(), -1) {
		if !slices.Contains(urls, url) {
			urls = append(urls, url)
		}
	}
	return urls
}

// delivered returns how many distinct images the response hands to the client.
func (f *imageInferenceFixture) delivered() int {
	return len(f.deliveredURLs())
}

// install wires the fixture into an isolated request context and stubs uploads.
func (f *imageInferenceFixture) install(t *testing.T) *gin.Context {
	t.Helper()
	f.t = t
	gctx, rec := imageTestContextWithRecorder(t, f.handler)
	f.rec = rec
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

// inferenceImageUser returns a synthetic user for image inference.
func inferenceImageUser(free bool) *config.UserConfig {
	key := "sk-synthetic-image"
	if free {
		key = "sk-shared-fixture"
	}
	return &config.UserConfig{
		Token: key, OpenaiToken: key, ImageToken: key,
		UserName: "image-tester", APIBase: config.Config.API, IsFree: free,
		AllowedModels: []string{"*"}, NoLimitExpensiveModels: true, BYOK: !free,
	}
}

// imageDeliveryCases covers delivery and storage bounds without model billing.
var imageDeliveryCases = []struct {
	name      string
	requested int
	free      bool
	produced  int
	empty     int

	failUpload bool

	delivered int
}{
	{name: "eight requested, eight delivered", requested: 8, produced: 8, delivered: 8},
	{name: "eight requested, one delivered", requested: 8, produced: 1, delivered: 1},
	{name: "upstream rejects the request", requested: 8, produced: 0, delivered: 0},
	{name: "upstream returns more than requested", requested: 2, produced: 5, delivered: 2},
	{name: "huge count is clamped", requested: hugeImageCount, produced: 8, delivered: 8},
	{name: "free user", requested: 5, free: true, produced: 1, delivered: 1},
	{name: "an entry without image data does not count", requested: 2, empty: 1, produced: 2, delivered: 2},
	{name: "leading entries without image data hide nothing", requested: 2, empty: 2, produced: 2, delivered: 2},
	{name: "no entry carries an image", requested: 2, empty: 2, delivered: 0},
	{name: "upload fails", requested: 3, produced: 3, failUpload: true, delivered: 0},
}

// TestSendChatImageGenerationDelegatesBilling verifies image delivery with no local charges.
func TestSendChatImageGenerationDelegatesBilling(t *testing.T) {
	for _, tc := range imageDeliveryCases {
		t.Run(tc.name, func(t *testing.T) {
			f := &imageInferenceFixture{produced: tc.produced, empty: tc.empty, failUpload: tc.failUpload}
			gctx := f.install(t)

			err := sendChatImageGeneration(gctx, imageCountTestRequest(tc.requested), inferenceImageUser(tc.free))
			if tc.delivered == 0 {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.delivered, f.delivered())
			require.Equal(t, []string{"Bearer " + inferenceImageUser(tc.free).OpenaiToken}, f.auth)
		})
	}
}

// TestDrawByDalleHandlerDelegatesBilling verifies batch generation with no local charges.
func TestDrawByDalleHandlerDelegatesBilling(t *testing.T) {
	for _, tc := range imageDeliveryCases {
		t.Run(tc.name, func(t *testing.T) {
			f := &imageInferenceFixture{produced: tc.produced, empty: tc.empty, failUpload: tc.failUpload}
			gctx := f.install(t)
			body, err := json.Marshal(map[string]any{"model": "dall-e-3", "prompt": "draw a tree", "n": tc.requested})
			require.NoError(t, err)
			gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/images/generations", bytes.NewReader(body))
			gctx.Request.Header.Set("Content-Type", "application/json")
			gctx.Set(ctxKeyUser, inferenceImageUser(tc.free))

			DrawByDalleHandler(gctx)
			if tc.delivered == 0 {
				require.GreaterOrEqual(t, gctx.Writer.Status(), http.StatusBadRequest, "a request without images must fail")
			} else {
				require.Equal(t, http.StatusOK, gctx.Writer.Status())
			}
			require.Equal(t, tc.delivered, f.delivered())
			require.Equal(t, []string{"Bearer " + inferenceImageUser(tc.free).OpenaiToken}, f.auth)
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
	f := &imageInferenceFixture{produced: 1}
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
	f := &imageInferenceFixture{}
	gctx := f.install(t)
	serverURL := config.Config.API
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/images/generations",
		bytes.NewReader([]byte(`{"model":"dall-e-3","prompt":"draw a tree","n":2}`)))
	gctx.Request.Header.Set("Content-Type", "application/json")
	gctx.Set(ctxKeyUser, inferenceImageUser(false))

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
	require.LessOrEqual(t, downloads.Load(), int32(requested), "surplus upstream images were downloaded")
}
