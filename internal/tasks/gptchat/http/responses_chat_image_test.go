package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

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
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	logger, _, _ := embeddingTestSetup(t, server)

	gin.SetMode(gin.TestMode)
	gctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", nil)
	gmw.SetLogger(gctx, logger)
	return gctx
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
				payload, err := io.ReadAll(r.Body)
				if err == nil {
					var body struct {
						N int64 `json:"n"`
					}
					if json.Unmarshal(payload, &body) == nil {
						sentN.Store(body.N)
					}
				}
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

// imageBillingRecorder serves the external billing API and a failing image
// upstream, recording every charged amount.
type imageBillingRecorder struct {
	mu      sync.Mutex
	charges []int64
}

// handler records billing requests and rejects everything else.
func (b *imageBillingRecorder) handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/token/consume" {
		http.Error(w, "synthetic upstream failure", http.StatusBadGateway)
		return
	}
	var usage struct {
		Cost int64 `json:"add_used_quota"`
	}
	payload, err := io.ReadAll(r.Body)
	if err == nil && json.Unmarshal(payload, &usage) == nil {
		b.mu.Lock()
		b.charges = append(b.charges, usage.Cost)
		b.mu.Unlock()
	}
	_, _ = w.Write([]byte("{}"))
}

// total returns the sum of all recorded charges.
func (b *imageBillingRecorder) total() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	var sum int64
	for _, c := range b.charges {
		sum += c
	}
	return sum
}

// billedImageUser returns a paid user with external image billing enabled.
func billedImageUser(free bool) *config.UserConfig {
	return &config.UserConfig{
		Token: "sk-synthetic-image", OpenaiToken: "sk-synthetic-image", ImageToken: "sk-synthetic-image",
		UserName: "image-tester", APIBase: config.Config.API, IsFree: free,
		AllowedModels: []string{"*"}, NoLimitExpensiveModels: true, EnableExternalImageBilling: true,
	}
}

// imageBillingCases are requested image counts and the counts that must be billed.
var imageBillingCases = []struct {
	name      string
	requested int
	free      bool
	billed    int
}{
	{name: "three images", requested: 3, billed: 3},
	{name: "huge count is clamped", requested: hugeImageCount, billed: maxImagesPerRequest},
	{name: "unset", requested: 0, billed: 1},
	{name: "free user", requested: 5, free: true, billed: 1},
}

// TestSendChatImageGenerationBillsPerImage verifies the upfront charge of an
// image-model chat request scales with the clamped image count, like Flux, which
// bills every generated image.
func TestSendChatImageGenerationBillsPerImage(t *testing.T) {
	for _, tc := range imageBillingCases {
		t.Run(tc.name, func(t *testing.T) {
			billing := &imageBillingRecorder{}
			gctx := imageCountTestContext(t, billing.handler)
			config.Config.ExternalBillingAPI = config.Config.API

			err := sendChatImageGeneration(gctx, imageCountTestRequest(tc.requested), billedImageUser(tc.free))
			require.Error(t, err) // the synthetic upstream fails after billing
			require.Equal(t, int64(GetImageModelPrice("dall-e-3"))*int64(tc.billed), billing.total())
		})
	}
}

// TestDrawByDalleHandlerBillsPerImage verifies the image generation endpoint charges
// for every requested (clamped) image instead of once per request.
func TestDrawByDalleHandlerBillsPerImage(t *testing.T) {
	for _, tc := range imageBillingCases {
		t.Run(tc.name, func(t *testing.T) {
			billing := &imageBillingRecorder{}
			gctx := imageCountTestContext(t, billing.handler)
			config.Config.ExternalBillingAPI = config.Config.API
			body, err := json.Marshal(map[string]any{"model": "dall-e-3", "prompt": "draw a tree", "n": tc.requested})
			require.NoError(t, err)
			gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/images/generations", bytes.NewReader(body))
			gctx.Request.Header.Set("Content-Type", "application/json")
			gctx.Set(ctxKeyUser, billedImageUser(tc.free))

			DrawByDalleHandler(gctx)
			require.Equal(t, int64(GetImageModelPrice("dall-e-3"))*int64(tc.billed), billing.total())
		})
	}
}
