package dns

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	"github.com/Laisky/go-ramjet/internal/tasks/arweave/config"
	"github.com/Laisky/go-ramjet/library/web"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
)

const S3Prefix = "arweave/dns/records/"

func dnsNameToS3Path(name string) string {
	sum := sha1.Sum([]byte(name))
	hashed := hex.EncodeToString(sum[:])

	return fmt.Sprintf("%s%s/%s/%s", S3Prefix, hashed[:2], hashed[2:4], hashed)
}

// CreateRecord create record
func CreateRecord(ctx *gin.Context) {
	req := new(CreateRecordRequest)
	if err := ctx.ShouldBindJSON(req); web.AbortErr(ctx, err) {
		return
	}

	logger := gmw.GetLogger(ctx).With(
		zap.String("name", req.Name),
		zap.String("file_id", req.FileID),
	)
	gmw.SetLogger(ctx, logger)

	opt := minio.GetObjectOptions{}
	opt.Set("Cache-Control", "no-cache")

	record := new(Record)
	objpath := dnsNameToS3Path(req.Name)
	obj, err := config.Instance.S3Cli.GetObject(gmw.Ctx(ctx),
		config.Instance.S3.Bucket,
		objpath,
		opt,
	)
	if err != nil {
		if minio.ToErrorResponse(err).Code != "NoSuchKey" {
			web.AbortErr(ctx, errors.Wrapf(err, "get record %q", objpath))
			return
		}

		// notfound, create
		if ctx.Request.Method != http.MethodPost {
			web.AbortErr(ctx, errors.Errorf("record not exists, %s", objpath))
			return
		}

		logger = logger.With(zap.String("op", "create"))
		record.Records = append(record.Records, recordItem{
			Name:   req.Name,
			FileID: req.FileID,
		})

	} else {
		// update
		if ctx.Request.Method != http.MethodPut {
			web.AbortErr(ctx, errors.Errorf("record already exists, %s", objpath))
			return
		}

		logger = logger.With(zap.String("op", "update"))
		if err = json.NewDecoder(obj).Decode(record); err != nil {
			web.AbortErr(ctx, errors.Wrap(err, "decode record"))
			return
		}

		var matched = false
		for idx, item := range record.Records {
			if item.Name == req.Name {
				record.Records[idx].FileID = req.FileID
				matched = true
				break
			}
		}

		if !matched {
			record.Records = append(record.Records, recordItem{
				Name:   req.Name,
				FileID: req.FileID,
			})
		}
	}

	// save
	body, err := json.Marshal(record)
	if web.AbortErr(ctx, errors.Wrap(err, "marshal record")) {
		return
	}

	_, err = config.Instance.S3Cli.PutObject(gmw.Ctx(ctx),
		config.Instance.S3.Bucket,
		objpath,
		bytes.NewReader(body),
		int64(len(body)),
		minio.PutObjectOptions{
			ContentType: "application/json",
		},
	)
	if web.AbortErr(ctx, errors.Wrap(err, "put record")) {
		return
	}

	logger.Info("record saved")
	ctx.JSON(http.StatusOK, gin.H{
		"msg":      "done",
		"bucket":   config.Instance.S3.Bucket,
		"obj_path": objpath,
	})
}

// GetRecord get record by name
func GetRecord(ctx *gin.Context) {
	logger := gmw.GetLogger(ctx)
	name := ctx.Param("name")

	opt := minio.GetObjectOptions{}
	opt.Set("Cache-Control", "no-cache")

	objpath := dnsNameToS3Path(name)
	obj, err := config.Instance.S3Cli.GetObject(gmw.Ctx(ctx),
		config.Instance.S3.Bucket,
		objpath,
		opt,
	)
	if err != nil {
		if minio.ToErrorResponse(err).Code != "NoSuchKey" {
			web.AbortErr(ctx, errors.Wrapf(err, "get record %q", objpath))
			return
		}

		ctx.JSON(http.StatusNotFound, gin.H{
			"msg": "record not found",
		})
		return
	}

	record := new(Record)
	if err = json.NewDecoder(obj).Decode(record); web.AbortErr(ctx, errors.Wrap(err, "decode record")) {
		return
	}

	for _, item := range record.Records {
		if item.Name == name {
			logger.Debug("get record",
				zap.String("name", name),
				zap.Any("record", record),
			)
			ctx.JSON(http.StatusOK, item)
			return
		}
	}

	ctx.JSON(http.StatusNotFound, gin.H{
		"msg": "record not found",
	})
}
