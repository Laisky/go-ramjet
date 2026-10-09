package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/web"
)

// OneapiProxyHandler proxy to oneapi url
func OneapiProxyHandler(ctx *gin.Context) {
	logger := gmw.GetLogger(ctx).Named("oneapi_proxy")

	url := ctx.Request.URL
	targetUrl := "https://oneapi.laisky.com" + "/" + strings.TrimPrefix(
		strings.TrimPrefix(url.Path, "/"), "gptchat/oneapi/")
	targetUrl += "?" + url.RawQuery

	req, err := http.NewRequestWithContext(gmw.Ctx(ctx),
		ctx.Request.Method,
		targetUrl,
		ctx.Request.Body,
	)
	if web.AbortErr(ctx, err) {
		return
	}

	req.Header = ctx.Request.Header

	user, err := getUserByAuthHeader(ctx)
	if web.AbortErr(ctx, errors.Wrap(err, "get user by auth header")) {
		return
	}
	req.Header.Set("Authorization", user.OpenaiToken)

	// just for test: fake response
	// {
	// 	resp, err := httpcli.Get("https://s3.laisky.com/embeddings/image-by-image/2024/04/mJMYRFmprERonrhEfuHwMnaYvzXQuzFLPQuo-1.png")
	// 	if web.AbortErr(ctx, err) {
	// 		return
	// 	}
	// 	defer gutils.LogErr(resp.Body.Close, log.Logger)

	// 	respBody, err := io.ReadAll(resp.Body)
	// 	if web.AbortErr(ctx, err) {
	// 		return
	// 	}

	// 	b64img := base64.StdEncoding.EncodeToString(respBody)
	// 	ctx.JSON(http.StatusOK, gin.H{
	// 		"data": []gin.H{
	// 			{
	// 				"url": "data:image/png;base64," + b64img,
	// 			},
	// 		},
	// 	})
	// 	return
	// }

	resp, err := httpcli.Do(req) //nolint: bodyclose
	if web.AbortErr(ctx, err) {
		return
	}

	defer gutils.LogErr(resp.Body.Close, log.Logger)
	payload, err := io.ReadAll(resp.Body)
	if web.AbortErr(ctx, err) {
		return
	}

	if isOneAPIModelListRequest(ctx.Request) {
		filteredPayload, filteredCnt, upstreamCnt, ferr := filterOneAPIModelListPayloadByUser(user, payload)
		if ferr != nil {
			logger.Warn("failed to filter oneapi model list payload",
				zap.String("username", user.UserName),
				zap.Error(ferr),
			)
		} else {
			payload = filteredPayload
			if filteredCnt != upstreamCnt {
				logger.Debug("filtered oneapi model list by user allowlist",
					zap.String("username", user.UserName),
					zap.Int("upstream_models", upstreamCnt),
					zap.Int("filtered_models", filteredCnt),
					zap.Int("allowed_models", len(user.AllowedModels)),
				)
			}
		}
	}

	for k, v := range resp.Header {
		if len(v) == 0 {
			continue
		}

		ctx.Header(k, v[0])
	}
	ctx.Header("Access-Control-Expose-Headers", "x-oneapi-request-id, x-request-id")
	ctx.Data(resp.StatusCode, resp.Header.Get("Content-Type"), payload)
}

// isOneAPIModelListRequest returns whether the current proxied request targets OneAPI model listing.
//
// Parameters:
//   - req: Incoming client request.
//
// Returns:
//   - bool: True when request is GET /oneapi/v1/models.
func isOneAPIModelListRequest(req *http.Request) bool {
	if req == nil {
		return false
	}

	if req.Method != http.MethodGet {
		return false
	}

	path := strings.TrimPrefix(req.URL.Path, "/")
	path = strings.TrimPrefix(path, "gptchat/")
	path = strings.TrimPrefix(path, "oneapi/")
	return path == "v1/models"
}

// filterOneAPIModelListPayloadByUser filters OneAPI /v1/models payload by user allowlist.
//
// Parameters:
//   - user: Current resolved user.
//   - payload: Raw upstream response body.
//
// Returns:
//   - []byte: Filtered JSON payload; may equal the input payload when no filtering is needed.
//   - int: Number of models after filtering.
//   - int: Number of models before filtering.
//   - error: Non-nil when payload cannot be parsed for filtering.
func filterOneAPIModelListPayloadByUser(user *config.UserConfig, payload []byte) ([]byte, int, int, error) {
	if user == nil || user.BYOK || len(user.AllowedModels) == 0 || slices.Contains(user.AllowedModels, "*") {
		modelsCnt, err := countOneAPIModels(payload)
		if err != nil {
			return payload, 0, 0, errors.Wrap(err, "count oneapi models")
		}

		return payload, modelsCnt, modelsCnt, nil
	}

	var modelList map[string]any
	if err := json.Unmarshal(payload, &modelList); err != nil {
		return nil, 0, 0, errors.Wrap(err, "unmarshal oneapi model list payload")
	}

	rawData, ok := modelList["data"].([]any)
	if !ok {
		return nil, 0, 0, errors.New("oneapi model list payload missing data array")
	}

	allowedSet := make(map[string]struct{}, len(user.AllowedModels))
	for _, model := range user.AllowedModels {
		allowedSet[model] = struct{}{}
	}

	filtered := make([]any, 0, len(rawData))
	for _, item := range rawData {
		modelEntry, ok := item.(map[string]any)
		if !ok {
			continue
		}

		modelID, ok := modelEntry["id"].(string)
		if !ok {
			continue
		}

		if _, ok = allowedSet[modelID]; ok {
			filtered = append(filtered, item)
		}
	}

	modelList["data"] = filtered
	filteredPayload, err := json.Marshal(modelList)
	if err != nil {
		return nil, 0, 0, errors.Wrap(err, "marshal filtered oneapi model list payload")
	}

	return filteredPayload, len(filtered), len(rawData), nil
}

// countOneAPIModels counts model entries in a OneAPI /v1/models response payload.
//
// Parameters:
//   - payload: Raw upstream response body.
//
// Returns:
//   - int: Count of model entries.
//   - error: Non-nil when payload is not a valid model list payload.
func countOneAPIModels(payload []byte) (int, error) {
	var modelList map[string]any
	if err := json.Unmarshal(payload, &modelList); err != nil {
		return 0, errors.Wrap(err, "unmarshal oneapi model list payload")
	}

	rawData, ok := modelList["data"].([]any)
	if !ok {
		return 0, errors.New("oneapi model list payload missing data array")
	}

	return len(rawData), nil
}

// RamjetProxyHandler proxy to ramjet url
func RamjetProxyHandler(ctx *gin.Context) {
	defer gutils.LogErr(ctx.Request.Body.Close, log.Logger)
	url := ctx.Request.URL
	targetUrl := config.Config.RamjetURL + "/" + strings.TrimPrefix(
		strings.TrimPrefix(url.Path, "/"), "gptchat/ramjet/")
	targetUrl += "?" + url.RawQuery

	req, err := http.NewRequestWithContext(gmw.Ctx(ctx),
		ctx.Request.Method,
		targetUrl,
		ctx.Request.Body,
	)
	if web.AbortErr(ctx, err) {
		return
	}

	req.Header = ctx.Request.Header
	req.Header.Del("Accept-Encoding") // do not disable gzip
	if err = setUserAuth(ctx, req); web.AbortErr(ctx, err) {
		return
	}

	resp, err := httpcli.Do(req) //nolint: bodyclose
	if web.AbortErr(ctx, err) {
		return
	}

	defer gutils.LogErr(resp.Body.Close, log.Logger)
	payload, err := io.ReadAll(resp.Body)
	if web.AbortErr(ctx, err) {
		return
	}

	for k, v := range resp.Header {
		if len(v) == 0 {
			continue
		}

		ctx.Header(k, v[0])
	}
	ctx.Header("Access-Control-Expose-Headers", "x-oneapi-request-id, x-request-id")
	ctx.Data(resp.StatusCode, resp.Header.Get("Content-Type"), payload)
}

// setUserAuth parse and set user auth to request header
func setUserAuth(gctx *gin.Context, req *http.Request) error {
	user, err := resolveRamjetUser(gctx)
	if err != nil {
		return errors.Wrap(err, "get user from token")
	}

	// req.Header.Set("X-Laisky-Image-Token-Type", user.ImageTokenType.String())
	req.Header.Set("X-Laisky-Openai-Api-Base", user.APIBase)
	req.Header.Set("X-Laisky-User-Id", user.UserName)
	if user.IsFree {
		req.Header.Set("X-Laisky-User-Is-Free", "true")
	}

	// if set header "Accept-Encoding" manually,
	// golang's http client will not auto decompress response body
	req.Header.Del("Accept-Encoding")

	// set token
	var (
		cost       db.Price
		costReason string
	)
	{
		token := user.OpenaiToken

		// generate image need special token
		if strings.HasPrefix(req.URL.Path, "/gptchat/image/") {
			cost = db.PriceTxt2Image
			costReason = "txt2image"
			token = user.ImageToken
			model := "image-" + strings.TrimPrefix(req.URL.Path, "/gptchat/image/")
			if err = IsModelAllowed(gctx, user, &FrontendReq{
				Model: model,
			}); err != nil {
				return errors.Wrapf(err, "check model %q", model)
			}
		}

		req.Header.Set("Authorization", token)
	}

	if user.EnableExternalImageBilling {
		if err := checkUserExternalBilling(gmw.Ctx(gctx), user, cost, costReason); err != nil {
			return errors.Wrapf(err, "check quota for user %q", user.UserName)
		}
	}

	return nil
}

// GetUserExternalBillingQuota get user external billing quota
// func GetUserExternalBillingQuota(ctx context.Context, user *config.UserConfig) (
// 	externalBalanceResp *ExternalBillingUserResponse, err error) {
// 	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
// 	defer cancel()

// 	// get balance
// 	url := config.Config.ExternalBillingAPI + "/api/token/" + user.ExternalImageBillingUID
// 	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
// 	if err != nil {
// 		return nil, errors.Wrap(err, "new request")
// 	}

// 	req.Header.Set("Authorization", "Bearer "+config.Config.ExternalBillingToken)
// 	resp, err := httpcli.Do(req) //nolint: bodyclose
// 	if err != nil {
// 		return nil, errors.Wrap(err, "do request")
// 	}
// 	defer gutils.LogErr(resp.Body.Close, log.Logger)

// 	if resp.StatusCode != http.StatusOK {
// 		return nil, errors.Errorf("get balance failed: %d", resp.StatusCode)
// 	}

// 	payload, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		return nil, errors.Wrap(err, "read body")
// 	}

// 	externalBalanceResp = new(ExternalBillingUserResponse)
// 	if err = json.Unmarshal(payload, externalBalanceResp); err != nil {
// 		return nil, errors.Wrap(err, "unmarshal")
// 	}

// 	if externalBalanceResp.Data.Status != ExternalBillingUserStatusActive {
// 		return nil, errors.Errorf("user %q is not active", user.UserName)
// 	}

// 	return externalBalanceResp, nil
// }

// GetUserInternalBill get user internal bill
func GetUserInternalBill(ctx context.Context,
	user *config.UserConfig, billType db.BillingType) (
	bill *db.Billing, err error) {
	openaiDB, err := db.GetOpenaiDB()
	if err != nil {
		return bill, errors.Wrap(err, "get openai db")
	}

	billingCol := openaiDB.GetCol("billing")

	// create index
	bill = &db.Billing{
		Username:    user.UserName,
		BillingType: billType,
	}
	if err = billingCol.FindOne(ctx, bson.M{
		"username": user.UserName,
		"type":     billType,
	}).Decode(bill); err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, errors.Wrapf(err, "get billing for user %q", user.UserName)
		}
	}

	return bill, nil
}
