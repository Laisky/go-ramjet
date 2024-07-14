package dns

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

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

	objpath := dnsNameToS3Path(req.Name)
	logger.Debug("get record",
		zap.String("bucket", config.Instance.S3.Bucket),
		zap.String("objpath", objpath))

	record := new(Record)
	opt := minio.GetObjectOptions{}
	opt.Set("Cache-Control", "no-cache")
	opt.SetReqParam("tt", strconv.Itoa(time.Now().Nanosecond()))
	obj, err := config.Instance.S3Cli.GetObject(gmw.Ctx(ctx),
		config.Instance.S3.Bucket,
		objpath,
		opt,
	)
	var notfound bool
	if err != nil {
		if minio.ToErrorResponse(err).Code != "NoSuchKey" {
			web.AbortErr(ctx, errors.Wrapf(err, "get record %q", objpath))
			return
		}

		notfound = true
	}

	objCnt, err := io.ReadAll(obj)
	if web.AbortErr(ctx, errors.Wrap(err, "read record")) {
		return
	}
	obj.Seek(0, io.SeekStart)

	// sometines, even if object is not found,
	// GetObject will return a valid object without error.
	//
	// Warning: obj.Stat() will erase the object's content,
	// so we should read the object's content before calling obj.Stat().
	if _, err := obj.Stat(); err != nil {
		if minio.ToErrorResponse(err).Code != "NoSuchKey" {
			web.AbortErr(ctx, errors.Wrapf(err, "get record %q", objpath))
			return
		}

		notfound = true
	}

	// notfound, create
	if notfound {
		if ctx.Request.Method != http.MethodPost {
			web.AbortErr(ctx,
				errors.Errorf("method is not POST, and record not exists, %s", objpath))
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
		if err = json.Unmarshal(objCnt, record); web.AbortErr(ctx, errors.Wrap(err, "decode record")) {
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
	name := ctx.Param("name")

	logger := gmw.GetLogger(ctx).With(
		zap.String("name", name),
	)
	gmw.SetLogger(ctx, logger)

	record, err := getRecord(gmw.Ctx(ctx), name)
	if web.AbortErr(ctx, err) {
		return
	}

	ctx.JSON(http.StatusOK, record)
	return
}

func getRecord(ctx context.Context, name string) (recordItem recordItem, err error) {
	logger := gmw.GetLogger(ctx)
	logger.Debug("get record", zap.String("name", name))

	opt := minio.GetObjectOptions{}
	opt.Set("Cache-Control", "no-cache")

	objpath := dnsNameToS3Path(name)
	obj, err := config.Instance.S3Cli.GetObject(ctx,
		config.Instance.S3.Bucket,
		objpath,
		opt,
	)
	if err != nil {
		if minio.ToErrorResponse(err).Code != "NoSuchKey" {
			return recordItem, errors.Wrapf(err, "get record %q", objpath)
		}

		return recordItem, errors.Errorf("record not found")
	}

	record := new(Record)
	if err = json.NewDecoder(obj).Decode(record); err != nil {
		return recordItem, errors.Wrap(err, "decode record")
	}

	for _, recordItem = range record.Records {
		if recordItem.Name == name {
			return recordItem, nil
		}
	}

	return recordItem, errors.Errorf("record not found")
}

func Query(ctx *gin.Context) {
	name := ctx.Param("name")

	logger := gmw.GetLogger(ctx).With(
		zap.String("name", name),
	)
	gmw.SetLogger(ctx, logger)

	record, err := getRecord(gmw.Ctx(ctx), name)
	if web.AbortErr(ctx, err) {
		return
	}

	ctx.Redirect(http.StatusFound, fmt.Sprintf("https://ario.laisky.com/%s", record.FileID))
}
