package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
	"github.com/Laisky/go-ramjet/library/log"
)

// RamjetProxyHandler proxy to ramjet url
func RamjetProxyHandler(ctx *gin.Context) {
	defer gutils.LogErr(ctx.Request.Body.Close, log.Logger)
	url := ctx.Request.URL
	targetUrl := ramjetURL + "/" + strings.TrimPrefix(
		strings.TrimPrefix(url.Path, "/"), "gptchat/ramjet/")
	targetUrl += "?" + url.RawQuery

	req, err := http.NewRequestWithContext(ctx.Request.Context(),
		ctx.Request.Method,
		targetUrl,
		ctx.Request.Body,
	)
	if AbortErr(ctx, err) {
		return
	}

	req.Header = ctx.Request.Header
	req.Header.Del("Accept-Encoding") // do not disable gzip
	if err = setUserAuth(ctx, req); AbortErr(ctx, err) {
		return
	}

	resp, err := httpcli.Do(req) //nolint: bodyclose
	if AbortErr(ctx, err) {
		return
	}

	defer gutils.LogErr(resp.Body.Close, log.Logger)
	payload, err := io.ReadAll(resp.Body)
	if AbortErr(ctx, err) {
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

// setUserAuth parse and set user auth to request header
func setUserAuth(ctx *gin.Context, req *http.Request) error {
	user, err := getUserByAuthHeader(ctx)
	if err != nil {
		return errors.Wrap(err, "get user from token")
	}

	req.Header.Set("X-Laisky-Image-Token-Type", user.ImageTokenType.String())
	req.Header.Set("X-Laisky-Openai-Api-Base", user.APIBase)
	req.Header.Set("X-Laisky-User-Id", user.UserName)

	// set token
	cost := db.Price(0)
	{
		cost = db.PriceTxt2Image
		token := user.OpenaiToken

		// generate image need special token
		if strings.HasPrefix(req.URL.Path, "/gptchat/image/") {

			token = user.ImageToken

			model := "image-" + strings.TrimPrefix(req.URL.Path, "/gptchat/image/")
			if err = user.IsModelAllowed(model); err != nil {
				return errors.Wrapf(err, "check model %q", model)
			}
		}

		req.Header.Set("Authorization", token)
	}

	if err := checkUserTotalQuota(ctx.Request.Context(), user, cost); err != nil {
		return errors.Wrapf(err, "check quota for user %q", user.UserName)
	}

	return nil
}

// GetUserExternalBillingQuota get user external billing quota
func GetUserExternalBillingQuota(ctx context.Context, user *config.UserConfig) (
	externalBalanceResp *ExternalBillingUserResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// get balance
	url := config.Config.ExternalBillingAPI + "/api/token/" + user.ExternalImageBillingUID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}

	req.Header.Set("Authorization", "Bearer "+config.Config.ExternalBillingToken)
	resp, err := httpcli.Do(req) //nolint: bodyclose
	if err != nil {
		return nil, errors.Wrap(err, "do request")
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger)

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("get balance failed: %d", resp.StatusCode)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read body")
	}

	externalBalanceResp = new(ExternalBillingUserResponse)
	if err = json.Unmarshal(payload, externalBalanceResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}

	if externalBalanceResp.Data.Status != ExternalBillingUserStatusActive {
		return nil, errors.Errorf("user %q is not active", user.UserName)
	}

	return externalBalanceResp, nil
}

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

// checkUserTotalQuota save and check billing for text-to-image models
func checkUserTotalQuota(ctx context.Context, user *config.UserConfig, cost db.Price) (err error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	logger := log.Logger.Named("openai.billing")
	if !user.EnableExternalImageBilling {
		logger.Debug("skip billing for user", zap.String("username", user.UserName))
		return nil
	}

	openaiDB, err := db.GetOpenaiDB()
	if err != nil {
		return errors.Wrap(err, "get openai db")
	}

	billingCol := openaiDB.GetCol("billing")

	// create index
	if name, err := billingCol.Indexes().CreateOne(ctx,
		mongo.IndexModel{Keys: bson.D{
			{Key: "username", Value: 1},
			{Key: "type", Value: 1},
		},
		}); err != nil {
		logger.Warn("create index for openai.billing", zap.String("name", name), zap.Error(err))
	}

	// get current quota
	bill, err := GetUserInternalBill(ctx, user, db.BillTypeTxt2Image)
	if err != nil {
		return errors.Wrapf(err, "get billing for user %q", user.UserName)
	}

	externalBalanceResp, err := GetUserExternalBillingQuota(ctx, user)
	if err != nil {
		return errors.Wrapf(err, "get billing for user %q", user.UserName)
	}

	// check balance
	if externalBalanceResp.Data.RemainQuota <= bill.UsedQuota+db.PriceTxt2Image {
		return errors.Errorf("user %q has not enough quota", user.UserName)
	}

	// update or create
	if cost != 0 {
		if _, err = billingCol.UpdateOne(ctx,
			bson.M{
				"username": user.UserName,
				"type":     db.BillTypeTxt2Image,
			},
			bson.M{
				"$inc": bson.M{"used_quota": db.PriceTxt2Image.Int()},
				"$set": bson.M{
					"username": user.UserName,
					"type":     db.BillTypeTxt2Image,
				},
			},
			options.Update().SetUpsert(true),
		); err != nil {
			return errors.Wrapf(err, "update billing for user %q", user.UserName)
		}
	}

	return nil
}
