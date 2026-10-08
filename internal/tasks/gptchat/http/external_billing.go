package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
)

// externalBillingUsage describes one charge, with optional completed-service latency.
// ElapsedTimeMS is omitted for upfront charges because the service has not run yet.
type externalBillingUsage struct {
	Cost          db.Price `json:"add_used_quota"`
	Reason        string   `json:"add_reason"`
	ElapsedTimeMS *int64   `json:"elapsed_time_ms,omitempty"`
}

// checkUserExternalBilling submits the supplied cost and reason without latency.
// It preserves upfront charging for existing callers and returns any billing error.
func checkUserExternalBilling(ctx context.Context,
	user *config.UserConfig, cost db.Price, costReason string) error {
	return consumeExternalBilling(ctx, user, externalBillingUsage{Cost: cost, Reason: costReason})
}

// checkUserExternalBillingWithElapsed submits one charge and the completed service's
// duration in nonnegative milliseconds. It returns any billing error without retrying.
func checkUserExternalBillingWithElapsed(ctx context.Context,
	user *config.UserConfig, cost db.Price, costReason string, elapsed time.Duration) error {
	millis := max(elapsed.Milliseconds(), 0)
	return consumeExternalBilling(ctx, user, externalBillingUsage{
		Cost: cost, Reason: costReason, ElapsedTimeMS: &millis,
	})
}

// consumeExternalBilling sends usage for the supplied user using the request context.
// It limits the billing request to 15 seconds and returns transport or HTTP errors.
func consumeExternalBilling(ctx context.Context, user *config.UserConfig, usage externalBillingUsage) error {
	logger := gmw.GetLogger(ctx).Named("openai.billing")
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var reqBody bytes.Buffer
	if err := json.NewEncoder(&reqBody).Encode(usage); err != nil {
		return errors.Wrap(err, "marshal request body")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		config.Config.ExternalBillingAPI+"/api/token/consume", &reqBody)
	if err != nil {
		return errors.Wrap(err, "push cost to external billing api")
	}
	req.Header.Set("Authorization", user.OpenaiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpcli.Do(req) //nolint: bodyclose
	if err != nil {
		return errors.Wrap(err, "do request")
	}
	defer gutils.LogErr(resp.Body.Close, logger)

	if resp.StatusCode != http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return errors.Wrap(err, "read body")
		}
		return errors.Errorf("push cost to external billing api failed [%d]%s",
			resp.StatusCode, string(respBody))
	}

	fields := []zap.Field{zap.String("username", user.UserName), zap.Int("cost", usage.Cost.Int())}
	if usage.ElapsedTimeMS != nil {
		fields = append(fields, zap.Int64("elapsed_time_ms", *usage.ElapsedTimeMS))
	}
	logger.Info("push cost to external billing api success", fields...)
	return nil
}
