package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	gutils "github.com/Laisky/go-utils/v5"
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

	for k, v := range resp.Header {
		if len(v) == 0 {
			continue
		}

		ctx.Header(k, v[0])
	}
	ctx.Data(resp.StatusCode, resp.Header.Get("Content-Type"), payload)
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
	user, cost, costReason, err := setUserAuth(ctx, req)
	if web.AbortErr(ctx, err) {
		return
	}

	shouldBill := user != nil && user.EnableExternalImageBilling && cost > 0
	var billingStart time.Time
	if shouldBill {
		billingStart = time.Now()
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

	if shouldBill {
		if err := checkUserExternalBilling(gmw.Ctx(ctx),
			user, cost, costReason, time.Since(billingStart)); web.AbortErr(ctx,
			errors.Wrapf(err, "check quota for user %q", user.UserName)) {
			return
		}
	}

	for k, v := range resp.Header {
		if len(v) == 0 {
			continue
		}

		ctx.Header(k, v[0])
	}
	ctx.Data(resp.StatusCode, resp.Header.Get("Content-Type"), payload)
}

// setUserAuth parse and set user auth to request header
func setUserAuth(gctx *gin.Context, req *http.Request) (
	user *config.UserConfig,
	cost db.Price,
	costReason string,
	err error,
) {
	user, err = getUserByAuthHeader(gctx)
	if err != nil {
		return nil, 0, "", errors.Wrap(err, "get user from token")
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
	token := user.OpenaiToken
	if strings.HasPrefix(req.URL.Path, "/gptchat/image/") {
		cost = db.PriceTxt2Image
		costReason = "txt2image"
		token = user.ImageToken
		model := "image-" + strings.TrimPrefix(req.URL.Path, "/gptchat/image/")
		if err = IsModelAllowed(gctx, user, &FrontendReq{
			Model: model,
		}); err != nil {
			return nil, 0, "", errors.Wrapf(err, "check model %q", model)
		}
	}

	req.Header.Set("Authorization", token)

	return user, cost, costReason, nil
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

// checkUserExternalBilling pushes the usage cost to the external billing API
// and attaches the elapsed processing time for observability.
func checkUserExternalBilling(ctx context.Context,
	user *config.UserConfig, cost db.Price, costReason string, elapsed time.Duration) (err error) {
	logger := log.Logger.Named("openai.billing")
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// push cost to remote billing
	var reqBody bytes.Buffer
	elapsedMillis := elapsed.Milliseconds()
	if elapsedMillis < 0 {
		elapsedMillis = 0
	}
	if err = json.NewEncoder(&reqBody).Encode(
		map[string]any{
			"add_used_quota":  cost,
			"add_reason":      costReason,
			"elapsed_time_ms": elapsedMillis,
		}); err != nil {
		return errors.Wrap(err, "marshal request body")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		config.Config.ExternalBillingAPI+"/api/token/consume", &reqBody)
	if err != nil {
		return errors.Wrap(err, "push cost to external billing api")
	}
	req.Header.Add("Authorization", user.OpenaiToken)

	resp, err := httpcli.Do(req) //nolint: bodyclose
	if err != nil {
		return errors.Wrap(err, "do request")
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger)

	if resp.StatusCode != http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return errors.Wrap(err, "read body")
		}

		return errors.Errorf("push cost to external billing api failed [%d]%s",
			resp.StatusCode, string(respBody))
	}
	logger.Info("push cost to external billing api success",
		zap.String("username", user.UserName),
		zap.Int("cost", cost.Int()),
		zap.Duration("elapsed", elapsed))

	// update or create
	// if cost != 0 {
	// 	if _, err = billingCol.UpdateOne(ctx,
	// 		bson.M{
	// 			"username": user.UserName,
	// 			"type":     db.BillTypeTxt2Image,
	// 		},
	// 		bson.M{
	// 			"$inc": bson.M{"used_quota": db.PriceTxt2Image.Int()},
	// 			"$set": bson.M{
	// 				"username": user.UserName,
	// 				"type":     db.BillTypeTxt2Image,
	// 			},
	// 		},
	// 		options.Update().SetUpsert(true),
	// 	); err != nil {
	// 		return errors.Wrapf(err, "update billing for user %q", user.UserName)
	// 	}
	// }

	return nil
}
