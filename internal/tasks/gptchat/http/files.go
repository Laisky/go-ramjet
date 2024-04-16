package http

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/s3"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
)

// UploadFiles upload files
func UploadFiles(ctx *gin.Context) {
	logger := gmw.GetLogger(ctx)

	user, err := getUserByAuthHeader(ctx)
	if AbortErr(ctx, err) {
		return
	}

	err = checkUserExternalBilling(ctx, user, db.PriceUploadFile, "upload file")
	if AbortErr(ctx, errors.Wrap(err, "check user external billing")) {
		return
	}

	file, err := ctx.FormFile("file")
	if AbortErr(ctx, errors.Wrap(err, "get file from form")) {
		return
	}

	if file.Size > int64(config.Config.LimitUploadFileBytes) {
		AbortErr(ctx, errors.Errorf("file size should not exceed %d bytes",
			config.Config.LimitUploadFileBytes))
		return
	}

	ext := ctx.PostForm("file_ext")
	if ext == "" {
		AbortErr(ctx, errors.New("should set file extension by `ext`"))
		return
	} else if !strings.HasPrefix(ext, ".") {
		AbortErr(ctx, errors.New("file extension should start with dot"))
		return
	}

	fileContent, err := file.Open()
	if AbortErr(ctx, errors.Wrap(err, "open file")) {
		return
	}

	var buf bytes.Buffer
	_, err = buf.ReadFrom(fileContent)
	if AbortErr(ctx, errors.Wrap(err, "read file content")) {
		return
	}
	fileBytes := buf.Bytes()

	fileHashBytes := sha1.Sum(fileBytes)
	fileHash := hex.EncodeToString(fileHashBytes[:])
	objkeyPrefix := fmt.Sprintf("user-files/%s/%s/%s",
		fileHash[:2], fileHash[2:4], fileHash)

	s3cli := s3.GetCli()
	_, err = s3cli.PutObject(ctx,
		config.Config.S3.Bucket,
		objkeyPrefix+ext,
		bytes.NewReader(fileBytes),
		int64(len(fileBytes)),
		minio.PutObjectOptions{
			ContentType: "application/octet-stream",
		},
	)
	if AbortErr(ctx, errors.Wrap(err, "upload file")) {
		return
	}

	logger.Info("upload file success",
		zap.String("user", user.UserName),
		zap.String("file", file.Filename),
		zap.String("ext", ext),
		zap.String("objkey", objkeyPrefix+ext),
	)
	ctx.JSON(200, gin.H{
		"url": fmt.Sprintf("https://s3.laisky.com/%s/%s", config.Config.S3.Bucket, objkeyPrefix+ext),
	})
}
