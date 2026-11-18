package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v6"
	gcompress "github.com/Laisky/go-utils/v6/compress"
	gcrypto "github.com/Laisky/go-utils/v6/crypto"
	"github.com/Laisky/go-utils/v6/json"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/s3"
	"github.com/Laisky/go-ramjet/library/log"
	rlimiter "github.com/Laisky/go-ramjet/library/ratelimit"
	s3lib "github.com/Laisky/go-ramjet/library/s3"
	"github.com/Laisky/go-ramjet/library/web"
)

var (
	onceLimiter                                     sync.Once
	freeModelRateLimiter, expensiveModelRateLimiter rlimiter.Limiter
)

const (
	freeModelRateLimitCost = 3
	freeUserMaxTokens      = 4500
	freeUserMaxResponses   = 1
)

// GetCurrentUser get current user
func GetCurrentUser(ctx *gin.Context) {
	setNoStoreHeaders(ctx)

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

// setNoStoreHeaders disables browser and intermediary caching for user-specific responses.
//
// Parameters:
//   - ctx: Current gin request context whose response headers will be updated.
//
// Returns:
//   - Nothing.
func setNoStoreHeaders(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store, no-cache, max-age=0, must-revalidate")
	ctx.Header("Pragma", "no-cache")
	ctx.Header("Expires", "0")
	ctx.Header("Surrogate-Control", "no-store")
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
		if resp.Code == "NoSuchKey" || resp.StatusCode == http.StatusNotFound {
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
	if _, err := s3lib.PutObjectCappingVersions(gmw.Ctx(ctx),
		logger,
		s3cli,
		config.Config.S3.Bucket,
		userConfigS3Key(apikey),
		bytes.NewReader(cipher),
		int64(len(cipher)),
		minio.PutObjectOptions{ContentType: "text/plain"},
		s3lib.DefaultVersionsToKeep,
	); web.AbortErr(ctx, errors.Wrap(err, "upload user config")) {
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

// deleteUserConfigResponse is the JSON body returned by DeleteUserConfig.
type deleteUserConfigResponse struct {
	// Deleted is the number of stored object versions (including delete
	// markers) that were permanently removed; 0 means no backup existed.
	Deleted int `json:"deleted"`
}

// DeleteUserConfig permanently deletes the encrypted cloud backup bound to the
// sync key sent in the X-LAISKY-SYNC-KEY header, so that key can no longer
// restore any configuration after the user rotates it.
//
// Parameters:
//   - ctx: Current gin request context carrying the sync key header.
//
// Returns:
//   - Nothing. It responds 200 with {"deleted": <n>}; deleting a backup that
//     does not exist succeeds with {"deleted": 0}. A missing sync key or an
//     object store failure aborts the request with an error response.
func DeleteUserConfig(ctx *gin.Context) {
	setNoStoreHeaders(ctx)
	logger := gmw.GetLogger(ctx)
	syncKey := strings.TrimSpace(ctx.Request.Header.Get("X-LAISKY-SYNC-KEY"))
	if syncKey == "" {
		web.AbortErr(ctx, errors.New("empty apikey"))
		return
	}

	logger = logger.With(zap.String("sync_key_fp", syncKeyFingerprint(syncKey)))

	s3cli, err := s3.GetCli()
	if web.AbortErr(ctx, errors.Wrap(err, "get s3 client")) {
		return
	}

	deleted, err := deleteUserConfigObjects(gmw.Ctx(ctx), logger, s3cli, config.Config.S3.Bucket, syncKey)
	if web.AbortErr(ctx, err) {
		return
	}

	logger.Info("delete user config success", zap.Int("deleted", deleted))
	ctx.JSON(http.StatusOK, deleteUserConfigResponse{Deleted: deleted})
}

// deleteUserConfigObjects removes every stored version of the cloud backup
// object derived from syncKey, including delete markers, so no historical
// ciphertext stays restorable.
//
// Parameters:
//   - ctx: Context bounding the object store calls.
//   - logger: Request-scoped logger; it must never carry the raw sync key.
//   - cli: Object store client used to list and remove versions.
//   - bucket: Bucket that stores user config backups.
//   - syncKey: Trimmed, non-empty sync key whose backup should be deleted.
//
// Returns:
//   - The number of object versions removed; 0 when no backup exists.
//   - An error when syncKey is empty or listing/removing fails. On a partial
//     failure the count removed before the failure is returned as well.
func deleteUserConfigObjects(
	ctx context.Context,
	logger glog.Logger,
	cli s3lib.ObjectVersionClient,
	bucket, syncKey string,
) (int, error) {
	if syncKey == "" {
		return 0, errors.New("empty sync key")
	}

	deleted, err := s3lib.RemoveAllObjectVersions(ctx, logger, cli, bucket, userConfigS3Key(syncKey))
	if err != nil {
		return deleted, errors.Wrapf(err, "delete user config (removed %d versions before failure)", deleted)
	}

	return deleted, nil
}

// findLastUserMessageIdx returns the index of the last user message in messages.
//
// findLastUserMessageIdx returns -1 when no user message exists.
func findLastUserMessageIdx(messages []FrontendReqMessage) int {
	for idx := len(messages) - 1; idx >= 0; idx-- {
		if messages[idx].Role == OpenaiMessageRoleUser {
			return idx
		}
	}

	return -1
}

// findOldestRemovableMessageIdx returns the oldest message index that can be dropped.
//
// findOldestRemovableMessageIdx keeps firstSystemIdx and lastUserIdx untouched and
// returns -1 when no removable message remains.
func findOldestRemovableMessageIdx(messages []FrontendReqMessage, firstSystemIdx, lastUserIdx int) int {
	for idx := range messages {
		if idx == firstSystemIdx || idx == lastUserIdx {
			continue
		}

		return idx
	}

	return -1
}

// applyFreeUserQuotaLimits adjusts req in place to satisfy free-user quotas.
//
// applyFreeUserQuotaLimits caps req.MaxTokens/req.N and drops oldest req.Messages
// while preserving the first system message and the last user message. It returns
// dropped message count, final prompt token count, whether req was changed, and an
// error when prompt tokens still exceed promptTokenLimit with no removable messages.
func applyFreeUserQuotaLimits(req *FrontendReq, promptTokenLimit int) (droppedMessages int, finalPromptTokens int, changed bool, err error) {
	if req.MaxTokens > freeUserMaxTokens {
		req.MaxTokens = freeUserMaxTokens
		changed = true
	}

	if req.N > freeUserMaxResponses {
		req.N = freeUserMaxResponses
		changed = true
	}

	firstSystemIdx := -1
	if len(req.Messages) > 0 && req.Messages[0].Role == OpenaiMessageRoleSystem {
		firstSystemIdx = 0
	}
	lastUserIdx := findLastUserMessageIdx(req.Messages)

	finalPromptTokens = req.PromptTokens()
	for promptTokenLimit > 0 && finalPromptTokens > promptTokenLimit && len(req.Messages) > 0 {
		oldestRemovableIdx := findOldestRemovableMessageIdx(req.Messages, firstSystemIdx, lastUserIdx)
		if oldestRemovableIdx < 0 {
			return droppedMessages, finalPromptTokens, changed,
				errors.Errorf("prompt tokens %d exceeds free-user limit %d even after preserving first system and last user messages",
					finalPromptTokens, promptTokenLimit)
		}

		req.Messages = append(req.Messages[:oldestRemovableIdx], req.Messages[oldestRemovableIdx+1:]...)
		droppedMessages++
		changed = true

		if oldestRemovableIdx < lastUserIdx {
			lastUserIdx--
		}
		finalPromptTokens = req.PromptTokens()
	}

	return droppedMessages, finalPromptTokens, changed, nil
}

// IsModelAllowed check if model is allowed
func IsModelAllowed(ctx context.Context,
	user *config.UserConfig,
	req *FrontendReq) error {
	onceLimiter.Do(setupRateLimiter)
	logger := gmw.GetLogger(ctx)

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
	// Free models. Keep this in sync with FreeModels in web/src/pages/gptchat/models.ts;
	// an id missing here silently falls through to the expensive rate limiter.
	switch req.Model {
	case "gpt-4o-mini",
		"gpt-5.4-nano",
		"openai/gpt-oss-20b",
		"openai/gpt-oss-120b",
		"deepseek-v4-flash",
		"@cf/meta/llama-3.3-70b-instruct-fp8-fast",
		"meta-llama/llama-prompt-guard-2-86m",
		"qwen/qwen3.6-27b",
		"tts",
		"gemini-3.1-flash-lite-preview",
		"gemini-3.5-flash-lite",
		"gemini-3.8-flash":
		ratelimiter = freeModelRateLimiter
		ratelimitCost = freeModelRateLimitCost
	default: // expensive model
		if user.NoLimitExpensiveModels {
			return nil
		}

		ratelimitCost = gconfig.Shared.GetInt("openai.rate_limit_expensive_models_interval_secs")
	}

	if !user.NoLimitExpensiveModels {
		droppedMessages, finalPromptTokens, changed, err := applyFreeUserQuotaLimits(req, user.LimitPromptTokenLength)
		if err != nil {
			return errors.WithStack(err)
		}

		if changed {
			logger.Debug("auto adjusted free user request to satisfy quota",
				zap.String("model", req.Model),
				zap.Int("dropped_messages", droppedMessages),
				zap.Int("prompt_tokens", finalPromptTokens),
				zap.Uint("max_tokens", req.MaxTokens),
				zap.Int("n", req.N),
			)
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
