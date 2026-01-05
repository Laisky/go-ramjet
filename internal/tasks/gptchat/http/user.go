package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v5"
	gcompress "github.com/Laisky/go-utils/v5/compress"
	gcrypto "github.com/Laisky/go-utils/v5/crypto"
	"github.com/Laisky/go-utils/v5/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/s3"
	"github.com/Laisky/go-ramjet/library/log"
	rlimiter "github.com/Laisky/go-ramjet/library/ratelimit"
	"github.com/Laisky/go-ramjet/library/web"
)

var (
	onceLimiter                                     sync.Once
	freeModelRateLimiter, expensiveModelRateLimiter rlimiter.Limiter
)

const (
	freeModelRateLimitCost = 3
)

// GetCurrentUser get current user
func GetCurrentUser(ctx *gin.Context) {
	user, err := getUserByAuthHeader(ctx)
	if web.AbortErr(ctx, err) {
		return
	}

	payload, err := json.Marshal(user)
	if web.AbortErr(ctx, err) {
		return
	}

	ctx.Data(200, "application/json", payload)
}

// func GetCurrentUserQuota(ctx *gin.Context) {
// 	usertoken := ctx.Query("apikey")
// 	user, err := getUserByToken(ctx, usertoken)
// 	if web.AbortErr(ctx, err) {
// 		return
// 	}

// 	externalBill, err := GetUserExternalBillingQuota(gmw.Ctx(ctx), user)
// 	if err != nil {
// 		log.Logger.Error("get user external billing quota", zap.Error(err))
// 	}

// 	// internalBill, err := GetUserInternalBill(gmw.Ctx(ctx), user, db.BillTypeTxt2Image)
// 	// if err != nil {
// 	// 	log.Logger.Error("get user internal billing quota", zap.Error(err))
// 	// }

// 	ctx.JSON(http.StatusOK, map[string]any{
// 		"external": externalBill,
// 	})
// }

func userConfigS3Key(apikey string) string {
	hashed := sha256.Sum256([]byte(apikey))
	return "user-configs/" + hex.EncodeToString(hashed[:])
}

// syncKeyFingerprint returns a stable, non-sensitive fingerprint for a sync key.
//
// syncKeyFingerprint hashes the sync key and returns a short prefix suitable for logs.
func syncKeyFingerprint(apikey string) string {
	hashed := sha256.Sum256([]byte(apikey))
	return hex.EncodeToString(hashed[:])[:12]
}

// isS3NoSuchKey returns true if an error indicates the S3 object key does not exist.
func isS3NoSuchKey(err error) bool {
	if err == nil {
		return false
	}

	var resp minio.ErrorResponse
	if errors.As(err, &resp) {
		if resp.Code == "NoSuchKey" || resp.StatusCode == 404 {
			return true
		}
		msg := strings.ToLower(resp.Message)
		if strings.Contains(msg, "does not exist") || strings.Contains(msg, "no such key") {
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") || strings.Contains(msg, "no such key")
}

// readAllOrNotFound reads all bytes from r.
//
// readAllOrNotFound returns (nil, false, nil) when the underlying read fails with a
// missing-key S3 error.
func readAllOrNotFound(r io.Reader) ([]byte, bool, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		if isS3NoSuchKey(err) {
			return nil, false, nil
		}
		return nil, false, errors.Wrap(err, "read body")
	}

	return body, true, nil
}

func UploadUserConfig(ctx *gin.Context) {
	logger := gmw.GetLogger(ctx)
	apikey := strings.TrimSpace(ctx.Request.Header.Get("X-LAISKY-SYNC-KEY"))
	if apikey == "" {
		web.AbortErr(ctx, errors.New("empty apikey"))
		return
	}

	logger = logger.With(zap.String("sync_key_fp", syncKeyFingerprint(apikey)))

	body, err := ctx.GetRawData()
	if web.AbortErr(ctx, errors.Wrap(err, "get raw data")) {
		return
	}

	if len(body) > 100*1024*1024 {
		web.AbortErr(ctx, errors.New("body too large"))
		return
	}

	var gzout bytes.Buffer
	err = gcompress.GzCompress(bytes.NewReader(body), &gzout)
	if web.AbortErr(ctx, errors.Wrap(err, "compress body")) {
		return
	}

	encryptKey, err := gcrypto.DeriveKeyByHKDF([]byte(apikey), nil, 32)
	if web.AbortErr(ctx, errors.Wrap(err, "derive key")) {
		return
	}

	cipher, err := gcrypto.AEADEncrypt(encryptKey, gzout.Bytes(), nil)
	if web.AbortErr(ctx, errors.Wrap(err, "encrypt body")) {
		return
	}

	// upload cipher to s3
	s3cli, err := s3.GetCli()
	if web.AbortErr(ctx, errors.Wrap(err, "get s3 client")) {
		return
	}

	logger.Debug("try to upload user config", zap.Int("cipher_len", len(cipher)))
	if _, err := s3cli.PutObject(gmw.Ctx(ctx),
		config.Config.S3.Bucket,
		userConfigS3Key(apikey),
		bytes.NewReader(cipher),
		int64(len(cipher)),
		minio.PutObjectOptions{
			ContentType: "text/plain",
		}); web.AbortErr(ctx, err) {
		return
	}

	logger.Info("upload user config success")
}

func DownloadUserConfig(ctx *gin.Context) {
	logger := gmw.GetLogger(ctx)
	apikey := strings.TrimSpace(ctx.Request.Header.Get("X-LAISKY-SYNC-KEY"))

	if apikey == "" {
		web.AbortErr(ctx, errors.New("empty apikey"))
		return
	}

	logger = logger.With(zap.String("sync_key_fp", syncKeyFingerprint(apikey)))

	opt := minio.GetObjectOptions{}
	opt.Set("Cache-Control", "no-cache")
	opt.SetReqParam("tt", strconv.Itoa(time.Now().Nanosecond()))

	s3cli, err := s3.GetCli()
	if web.AbortErr(ctx, errors.Wrap(err, "get s3 client")) {
		return
	}

	object, err := s3cli.GetObject(gmw.Ctx(ctx),
		config.Config.S3.Bucket,
		userConfigS3Key(apikey),
		opt,
	)
	if err != nil {
		if isS3NoSuchKey(err) {
			logger.Debug("cloud user config not found; return empty")
			ctx.Header("Cache-Control", "no-cache")
			ctx.Data(200, "application/json", []byte("{}"))
			return
		}
		web.AbortErr(ctx, errors.Wrap(err, "get user config from s3"))
		return
	}
	defer gutils.CloseWithLog(object, logger)

	body, exists, err := readAllOrNotFound(object)
	if err != nil {
		web.AbortErr(ctx, err)
		return
	}
	if !exists {
		logger.Debug("cloud user config not found during read; return empty")
		ctx.Header("Cache-Control", "no-cache")
		ctx.Data(200, "application/json", []byte("{}"))
		return
	}

	encryptKey, err := gcrypto.DeriveKeyByHKDF([]byte(apikey), nil, 32)
	if web.AbortErr(ctx, errors.Wrap(err, "derive key")) {
		return
	}

	plaintext, err := gcrypto.AEADDecrypt(encryptKey, body, nil)
	if web.AbortErr(ctx, errors.Wrap(err, "decrypt body")) {
		return
	}

	var gzout bytes.Buffer
	err = gcompress.GzDecompress(bytes.NewReader(plaintext), &gzout)
	if web.AbortErr(ctx, errors.Wrap(err, "decompress body")) {
		return
	}

	logger.Info("download user config success")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Data(200, "application/json", gzout.Bytes())
}

// IsModelAllowed check if model is allowed
func IsModelAllowed(ctx context.Context,
	user *config.UserConfig,
	req *FrontendReq) error {
	onceLimiter.Do(setupRateLimiter)
	logger := gmw.GetLogger(ctx)

	nPromptTokens := req.PromptTokens()

	switch {
	case user.BYOK: // bypass if user bring their own token
		logger.Debug("bypass rate limit for BYOK user")
		return nil
	case user.NoLimitExpensiveModels:
		logger.Debug("bypass rate limit for no_limit_expensive_models user")
		return nil
	default:
	}

	if len(user.AllowedModels) == 0 {
		return errors.Errorf("no allowed models for current user %q", user.UserName)
	}

	var allowed bool
	for _, m := range user.AllowedModels {
		if m == "*" {
			allowed = true
			break
		}

		if m == req.Model {
			allowed = true
			break
		}
	}
	if !allowed {
		return errors.Errorf("model %q is not allowed for user %q", req.Model, user.UserName)
	}

	// if !globalRatelimiter.Allow() { // check rate limit
	// 	return errors.Errorf("too many requests, please try again later")
	// }

	// rate limit only support limit by second,
	// so we consume 60 tokens once to make it limit by minute
	var (
		ratelimitCost int
		ratelimiter   = expensiveModelRateLimiter
	)
	switch req.Model {
	case "gpt-3.5-turbo", // free models
		// "gpt-3.5-turbo-1106",
		// "gpt-3.5-turbo-0125",
		"gpt-4o-mini",
		"openai/gpt-oss-20b",
		"openai/gpt-oss-120b",
		// "llama2-70b-4096",
		"deepseek-chat",
		// "deepseek-coder",
		"gemma2-9b-it",
		"gemma-3-27b-it",
		"llama3-8b-8192",
		"llama3-70b-8192",
		"llama-3.1-8b-instant",
		"llama-3.1-405b-instruct",
		"llama-3.3-70b-versatile",
		"qwen-qwq-32b",
		"qwen/qwen3-32b",
		// "moonshotai/kimi-k2-instruct",
		// "mixtral-8x7b-32768",
		// "img-to-img",
		// "sdxl-turbo",
		"tts",
		"gemini-2.0-flash":
		ratelimiter = freeModelRateLimiter
		ratelimitCost = freeModelRateLimitCost
	default: // expensive model
		if user.NoLimitExpensiveModels {
			return nil
		}

		ratelimitCost = gconfig.Shared.GetInt("openai.rate_limit_expensive_models_interval_secs")
	}

	if !user.NoLimitExpensiveModels {
		if user.LimitPromptTokenLength > 0 && nPromptTokens > user.LimitPromptTokenLength {
			return errors.Errorf(
				"The length of the prompt you submitted is %d, exceeds the limit for free users %d, "+
					"you need upgrade to a paid membership to use longer prompt tokens, "+
					"more info at https://wiki.laisky.com/projects/gpt/pay/",
				nPromptTokens, user.LimitPromptTokenLength)
		}

		if req.MaxTokens > 4500 {
			return errors.New("max_tokens is limited to 4500 for free users, " +
				"you need upgrade to a paid membership to use larger max_tokens, " +
				"more info at https://wiki.laisky.com/projects/gpt/pay/")
		}

		if req.N > 1 {
			return errors.New("free users are limited to 1 response per request, " +
				"you need upgrade to a paid membership to use larger n, " +
				"more info at https://wiki.laisky.com/projects/gpt/pay/")
		}
	}

	// if price less than 0, means no limit
	logger.Debug("check rate limit",
		zap.String("model", req.Model), zap.Int("price", ratelimitCost))
	if ratelimitCost > 0 && !ratelimiter.AllowN(ratelimitCost) { // check rate limit
		return errors.Errorf("This model(%s) restricts usage for free users. "+
			"Please hold on for %d seconds before trying again, "+
			"alternatively, you may opt to switch to the free gpt-4o-mini, "+
			"or upgrade to a paid membership by https://wiki.laisky.com/projects/gpt/pay/",
			req.Model, (ratelimitCost - ratelimiter.Len()))
	}

	return nil
}

// setupRateLimiter setup ratelimiter depends on loaded config
func setupRateLimiter() {
	const burstRatio = 1.3
	var err error
	logger := log.Logger.Named("gptchat.ratelimiter")

	// {
	// 	if globalRatelimiter, err = gutils.NewRateLimiter(context.Background(),
	// 		gutils.RateLimiterArgs{
	// 			Max:     10,
	// 			NPerSec: 1,
	// 		}); err != nil {
	// 		log.Logger.Panic("new ratelimiter", zap.Error(err))
	// 	}
	// 	logger.Info("set overall ratelimiter", zap.Int("burst", 10))
	// }

	burst := int(math.Ceil(float64(config.Config.RateLimitExpensiveModelsIntervalSeconds) * burstRatio))
	if expensiveModelRateLimiter, err = rlimiter.New(context.Background(),
		"gptchat:expensive",
		rlimiter.Args{Max: burst, NPerSec: 1}); err != nil {
		log.Logger.Panic("new expensiveModelRateLimiter", zap.Error(err))
	}
	logger.Info("set ratelimiter for expensive models", zap.Int("burst", burst))

	freeBurst := int(math.Ceil(float64(freeModelRateLimitCost) * burstRatio))
	if freeModelRateLimiter, err = rlimiter.New(context.Background(),
		"gptchat:free",
		rlimiter.Args{Max: freeBurst, NPerSec: 1}); err != nil {
		log.Logger.Panic("new freeModelRateLimiter", zap.Error(err))
	}
	logger.Info("set ratelimiter for free models", zap.Int("burst", freeBurst))
}
