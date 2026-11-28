package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

type fixtureInferenceLimiter struct {
	permit bool
	calls  atomic.Int32
}

// Allow consumes one unit from the test limiter.
func (l *fixtureInferenceLimiter) Allow() bool { return l.AllowN(1) }

// AllowN records an admission attempt and returns the configured result.
func (l *fixtureInferenceLimiter) AllowN(_ int) bool {
	l.calls.Add(1)
	return l.permit
}

// Len reports no residual tokens when simulating an exhausted bucket.
func (l *fixtureInferenceLimiter) Len() int { return 0 }

// stubInferenceLimiters isolates admission from Redis for synthetic handler tests.
func stubInferenceLimiters(t *testing.T, permit bool) *fixtureInferenceLimiter {
	t.Helper()
	oldFree, oldExpensive := freeModelRateLimiter, expensiveModelRateLimiter
	limiter := &fixtureInferenceLimiter{permit: permit}
	freeModelRateLimiter, expensiveModelRateLimiter = limiter, limiter
	onceLimiter = sync.Once{}
	onceLimiter.Do(func() {})
	t.Cleanup(func() {
		freeModelRateLimiter, expensiveModelRateLimiter = oldFree, oldExpensive
		onceLimiter = sync.Once{}
		if oldFree != nil || oldExpensive != nil {
			onceLimiter.Do(func() {})
		}
	})
	return limiter
}

// TestNativeCredentialRejection checks that absent and invalid credentials
// cannot be mistaken for the same native authentication state.
func TestNativeCredentialRejection(t *testing.T) {
	byokAuditSetup(t)
	for _, header := range [][]string{{""}, {"Bearer "}, {"Basic abcdefghijklmnopqrst"}, {"sk-fixture-caller", "sk-another-caller"}} {
		ctx := newAuthContext("")
		ctx.Request.Header["Authorization"] = header
		_, err := getUserByAuthHeader(ctx)
		require.Error(t, err)
	}
	ctx := newAuthContext("")
	user, err := getUserByAuthHeader(ctx)
	require.NoError(t, err)
	require.True(t, user.IsFree)
	require.Equal(t, "SERVER_OPENAI_TOKEN", user.OpenaiToken)
}

// TestFreeTierBudgetGuardsAllResponsesAttempts uses only a synthetic transport.
// Rounds share a single bounded parent and do not re-charge its limiter.
func TestFreeTierBudgetGuardsAllResponsesAttempts(t *testing.T) {
	byokAuditSetup(t)
	limiter := stubInferenceLimiters(t, true)
	gctx := newAuthContext(config.FreetierUserToken)
	user, err := getUserByAuthHeader(gctx)
	require.NoError(t, err)
	var calls int
	httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "Bearer "+user.OpenaiToken, req.Header.Get("Authorization"))
		return byokAuditResponse(req, `{"id":"synthetic","output_text":"ok"}`), nil
	})}
	deps := UpstreamDeps{User: user}
	body := &OpenAIResponsesReq{Model: "gpt-4o-mini"}
	_, _, err = CallUpstreamResponsesCtx(context.Background(), deps, body)
	require.ErrorContains(t, err, "admitted parent")
	require.Zero(t, calls)

	require.NoError(t, IsModelAllowed(gctx, user, &FrontendReq{Model: body.Model}))
	grantInferenceBudget(gctx, user, body.Model, 2)
	ctx := gctx.Request.Context()
	_, _, err = CallUpstreamResponsesCtx(ctx, deps, &OpenAIResponsesReq{Model: "unlisted"})
	require.ErrorContains(t, err, "not allowed")
	for range 2 {
		_, _, err = CallUpstreamResponsesCtx(ctx, deps, body)
		require.NoError(t, err)
	}
	_, _, err = CallUpstreamResponsesCtx(ctx, deps, body)
	require.ErrorContains(t, err, "budget exhausted")
	require.Equal(t, 2, calls)
	require.EqualValues(t, 1, limiter.calls.Load())
}

// TestFreeTierDenialStopsChatBeforeModelPreparation verifies request-level admission.
func TestFreeTierDenialStopsChatBeforeModelPreparation(t *testing.T) {
	byokAuditSetup(t)
	limiter := stubInferenceLimiters(t, false)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", strings.NewReader(
		`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}],"laisky_extra":{"chat_switch":{"enable_google_search":true,"agent_mode":true}}}`))
	ctx.Request.Header.Set("Authorization", "Bearer "+config.FreetierUserToken)
	ChatHandler(ctx)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.EqualValues(t, 1, limiter.calls.Load())
	require.Contains(t, w.Body.String(), "restricts usage")
}

// TestOneShotPublicCredentialContract uses the public handler with mock
// provider results and verifies that no response triggers key substitution.
func TestOneShotPublicCredentialContract(t *testing.T) {
	byokAuditSetup(t)
	const callerKey = "sk-Synthetic_One_Shot_Original"
	config.Config.UserTokens[0].OpenaiToken = testSharedOneAPIToken
	var statuses []int
	var calls int
	httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "https://oneapi.laisky.com/v1/chat/completions", req.URL.String())
		require.Equal(t, "Bearer "+callerKey, req.Header.Get("Authorization"))
		calls++
		resp := byokAuditResponse(req, "{\"choices\":[{\"message\":{\"content\":\"synthetic answer\"}}]}")
		resp.StatusCode = statuses[0]
		statuses = statuses[1:]
		return resp, nil
	})}
	router := gin.New()
	router.POST("/gptchat/chat/oneshot", OneShotChatHandler)
	invoke := func(auth ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/gptchat/chat/oneshot", strings.NewReader("{\"user_prompt\":\"synthetic\"}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header["Authorization"] = auth
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	for _, auth := range [][]string{nil, {""}, {"Bearer "}, {config.FreetierUserToken},
		{"Bearer FREETIER-synthetic-client"}, {"Bearer " + testSharedOneAPIToken}, {"Bearer eyJ.synthetic.session"}, {"Bearer " + callerKey, "Bearer " + callerKey}} {
		require.Equal(t, http.StatusUnauthorized, invoke(auth...).Code)
	}
	require.Zero(t, calls)
	statuses = []int{200, 401, 403, 429, 402}
	success := invoke("bearer " + callerKey)
	require.Equal(t, http.StatusOK, success.Code)
	require.JSONEq(t, "{\"response\":\"synthetic answer\"}", success.Body.String())
	for range 4 {
		require.GreaterOrEqual(t, invoke(callerKey).Code, 400)
	}
	require.Equal(t, 5, calls)
	require.Empty(t, statuses)
}

// TestFreeTierPolicyRejectsUnsafeConfiguration checks model policy and the
// shared credential are required even for the existing anonymous identity.
func TestFreeTierPolicyRejectsUnsafeConfiguration(t *testing.T) {
	for _, mutate := range []func(*config.OpenAI){
		func(c *config.OpenAI) { c.UserTokens[0].OpenaiToken = "" },
		func(c *config.OpenAI) { c.UserTokens[0].AllowedModels = []string{"*"} },
		func(c *config.OpenAI) { c.UserTokens[0].NoLimitExpensiveModels = true },
		func(c *config.OpenAI) { c.UserTokens[0].BYOK = true },
		func(c *config.OpenAI) { c.RateLimitExpensiveModelsIntervalSeconds = 0 },
		func(c *config.OpenAI) { c.RateLimitFreeModelsIntervalSeconds = 0 },
	} {
		byokAuditSetup(t)
		mutate(config.Config)
		_, err := getUserByAuthHeader(newAuthContext(config.FreetierUserToken))
		require.Error(t, err)
	}
}
