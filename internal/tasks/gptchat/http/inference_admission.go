package http

import (
	"context"
	"crypto/subtle"
	"slices"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// freeTurnModelCalls caps all physical model requests in a single admitted
// chat turn, including concurrent auxiliary calls and application retries.
const freeTurnModelCalls = 24

type inferenceBudgetKey struct{}

type inferenceBudget struct {
	user      *config.UserConfig
	model     string
	remaining atomic.Int32
}

// checkFreeModel checks an exact capability on an authenticated free-tier user.
func checkFreeModel(user *config.UserConfig, model string) error {
	if user == nil || !user.IsFree || user.BYOK || user.NoLimitExpensiveModels || user.OpenaiToken == "" {
		return errors.New("Free-tier inference policy is unavailable")
	}
	if model == "" || model == "*" || !slices.Contains(user.AllowedModels, model) {
		return errors.Errorf("Model %q is not allowed for free-tier inference", model)
	}
	return nil
}

// grantInferenceBudget binds an already admitted operation to the request.
// A single-operation endpoint receives one slot; chat receives a bounded turn.
func grantInferenceBudget(ctx *gin.Context, user *config.UserConfig, model string, calls int32) {
	if !user.IsFree {
		return
	}
	budget := &inferenceBudget{user: user, model: model}
	budget.remaining.Store(calls)
	ctx.Request = ctx.Request.WithContext(context.WithValue(ctx.Request.Context(), inferenceBudgetKey{}, budget))
}

// consumeInferenceCall admits one actual outbound model call. Every free-tier
// call must retain its parent's budget, key, configured endpoint and model policy.
func consumeInferenceCall(ctx context.Context, user *config.UserConfig, model string) error {
	if user == nil {
		return errors.New("Model credential is unavailable")
	}
	if !user.IsFree {
		return nil
	}
	if err := checkFreeModel(user, model); err != nil {
		return errors.Wrap(err, "admit model invocation")
	}
	budget, _ := ctx.Value(inferenceBudgetKey{}).(*inferenceBudget)
	if budget == nil || config.Config == nil || user.APIBaseUserSelected ||
		user.APIBase != config.Config.ManagedAPIBase() ||
		budget.user.UserName != user.UserName ||
		subtle.ConstantTimeCompare([]byte(budget.user.OpenaiToken), []byte(user.OpenaiToken)) != 1 {
		return errors.New("Free-tier inference requires an admitted parent request")
	}
	if model != budget.model && !isLowCostFreeModel(model) {
		return errors.Errorf("Free-tier auxiliary model %q requires separate admission", model)
	}
	for {
		remaining := budget.remaining.Load()
		if remaining <= 0 {
			return errors.New("Free-tier inference budget exhausted")
		}
		if budget.remaining.CompareAndSwap(remaining, remaining-1) {
			return nil
		}
	}
}

// limitFreeResponsesOutput caps a free-tier model response even when an
// auxiliary caller omits max_output_tokens. It leaves caller-owned input alone.
func limitFreeResponsesOutput(user *config.UserConfig, req *OpenAIResponsesReq) error {
	if req == nil {
		return errors.New("Model request is missing")
	}
	if user != nil && user.IsFree && (req.MaxOutputTokens == 0 || req.MaxOutputTokens > freeUserMaxTokens) {
		req.MaxOutputTokens = freeUserMaxTokens
	}
	return nil
}
