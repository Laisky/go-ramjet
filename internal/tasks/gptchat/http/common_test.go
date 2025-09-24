package http

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// helper to build a gin context with provided Authorization header
func newAuthContext(token string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	if token != "" {
		req.Header.Set("authorization", "Bearer "+token)
	}
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = req
	return ctx
}

// set up a minimal global config with distinct default image token
func setupTestConfig() {
	config.Config = &config.OpenAI{
		Token:              "SERVER_OPENAI_TOKEN",
		DefaultImageToken:  "SERVER_IMAGE_TOKEN",
		API:                "https://api.openai.com",
		DefaultImageUrl:    "https://api.openai.com/v1/images/generations",
		ExternalBillingAPI: "https://oneapi.laisky.com",
		RamjetURL:          "https://app.laisky.com",
	}
}

func TestGetUserByAuthHeader_LaiskyTokenUsesOwnImageToken(t *testing.T) {
	setupTestConfig()

	userToken := "laisky-123456789abcd" // length >= 15
	ctx := newAuthContext(userToken)

	user, err := getUserByAuthHeader(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.Token != userToken {
		t.Fatalf("expected user.Token to be %q, got %q", userToken, user.Token)
	}
	if user.OpenaiToken != userToken {
		t.Fatalf("expected user.OpenaiToken to be %q, got %q", userToken, user.OpenaiToken)
	}
	if user.ImageToken != userToken {
		t.Fatalf("expected user.ImageToken to be user's token %q, got %q", userToken, user.ImageToken)
	}
	if user.ImageToken == config.Config.DefaultImageToken {
		t.Fatalf("user.ImageToken should not fall back to default image token %q", config.Config.DefaultImageToken)
	}
	if !user.BYOK {
		t.Fatalf("expected BYOK to be true for laisky- token user")
	}
}

func TestGetUserByAuthHeader_SKTokenUsesOwnImageToken(t *testing.T) {
	setupTestConfig()

	userToken := "sk-123456789abcdef" // length >= 15
	ctx := newAuthContext(userToken)

	user, err := getUserByAuthHeader(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.ImageToken != userToken {
		t.Fatalf("expected user.ImageToken to be user's token %q, got %q", userToken, user.ImageToken)
	}
	if user.ImageToken == config.Config.DefaultImageToken {
		t.Fatalf("user.ImageToken should not fall back to default image token %q", config.Config.DefaultImageToken)
	}
	if !user.BYOK {
		t.Fatalf("expected BYOK to be true for sk- token user")
	}
}
