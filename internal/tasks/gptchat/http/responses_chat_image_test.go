package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
