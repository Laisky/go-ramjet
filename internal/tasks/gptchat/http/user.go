package http

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/s3"
	"github.com/Laisky/go-ramjet/library/log"
	gcrypto "github.com/Laisky/go-utils/v4/crypto"
	"github.com/Laisky/go-utils/v4/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
)

// GetCurrentUser get current user
func GetCurrentUser(ctx *gin.Context) {
	user, err := getUserByAuthHeader(ctx)
	if AbortErr(ctx, err) {
		return
	}

	payload, err := json.Marshal(user)
	if AbortErr(ctx, err) {
		return
	}

	ctx.Data(200, "application/json", payload)
}

func GetCurrentUserQuota(ctx *gin.Context) {
	usertoken := ctx.Query("apikey")
	user, err := getUserByToken(ctx, usertoken)
	if AbortErr(ctx, err) {
		return
	}

	externalBill, err := GetUserExternalBillingQuota(ctx.Request.Context(), user)
	if err != nil {
		log.Logger.Error("get user external billing quota", zap.Error(err))
	}

	// internalBill, err := GetUserInternalBill(ctx.Request.Context(), user, db.BillTypeTxt2Image)
	// if err != nil {
	// 	log.Logger.Error("get user internal billing quota", zap.Error(err))
	// }

	ctx.JSON(http.StatusOK, map[string]any{
		"external": externalBill,
	})
}

func userConfigS3Key(apikey string) string {
	hashed := sha256.Sum256([]byte(apikey))
	return "user-configs/" + hex.EncodeToString(hashed[:])
}

func UploadUserConfig(ctx *gin.Context) {
	logger := gmw.GetLogger(ctx)
	apikey := strings.TrimSpace(ctx.Request.Header.Get("X-LAISKY-SYNC-KEY"))
	if apikey == "" {
		AbortErr(ctx, errors.New("empty apikey"))
		return
	}

	logger = logger.With(zap.String("user", apikey[:15]))

	body, err := ctx.GetRawData()
	if AbortErr(ctx, errors.Wrap(err, "get raw data")) {
		return
	}

	if len(body) > 100*1024*1024 {
		AbortErr(ctx, errors.New("body too large"))
		return
	}

	encryptKey, err := gcrypto.DeriveKeyByHKDF([]byte(apikey), nil, 32)
	if AbortErr(ctx, errors.Wrap(err, "derive key")) {
		return
	}

	cipher, err := gcrypto.AEADEncrypt(encryptKey, body, nil)
	if AbortErr(ctx, errors.Wrap(err, "encrypt body")) {
		return
	}

	// upload cipher to s3
	if _, err := s3.GetCli().PutObject(ctx.Request.Context(),
		config.Config.S3.Bucket,
		userConfigS3Key(apikey),
		bytes.NewReader(cipher),
		int64(len(cipher)),
		minio.PutObjectOptions{
			ContentType: "text/plain",
		}); AbortErr(ctx, err) {
		return
	}

	logger.Info("upload user config success")
}

func DownloadUserConfig(ctx *gin.Context) {
	logger := gmw.GetLogger(ctx)
	apikey := strings.TrimSpace(ctx.Request.Header.Get("X-LAISKY-SYNC-KEY"))

	if apikey == "" {
		AbortErr(ctx, errors.New("empty apikey"))
		return
	}

	logger = logger.With(zap.String("user", apikey[:15]))

	opt := minio.GetObjectOptions{}
	opt.Set("Cache-Control", "no-cache")
	opt.SetReqParam("tt", strconv.Itoa(time.Now().Nanosecond()))

	object, err := s3.GetCli().GetObject(ctx.Request.Context(),
		config.Config.S3.Bucket,
		userConfigS3Key(apikey),
		opt,
	)
	if AbortErr(ctx, errors.Wrap(err, "get user config from s3")) {
		return
	}
	defer object.Close()

	cipher, err := io.ReadAll(object)
	if AbortErr(ctx, errors.Wrap(err, "read cipher from s3")) {
		return
	}

	encryptKey, err := gcrypto.DeriveKeyByHKDF([]byte(apikey), nil, 32)
	if AbortErr(ctx, errors.Wrap(err, "derive key")) {
		return
	}

	plaintext, err := gcrypto.AEADDecrypt(encryptKey, cipher, nil)
	if AbortErr(ctx, errors.Wrap(err, "decrypt body")) {
		return
	}

	logger.Info("download user config success")
	ctx.Data(200, "application/json", plaintext)
}
