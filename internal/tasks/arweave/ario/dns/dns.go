package dns

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	"github.com/Laisky/go-ramjet/internal/tasks/arweave/config"
	"github.com/Laisky/go-ramjet/library/web"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
)

const S3Prefix = "dns/records/"

func dnsNameToS3Path(name string) string {
	return S3Prefix + gutils.FileHashSharding(name)
}

func CreateRecord(ctx *gin.Context) {
	logger := gmw.GetLogger(ctx)
	req := new(CreateRecordRequest)
	if err := ctx.ShouldBindJSON(req); web.AbortErr(ctx, err) {
		return
	}

	opt := minio.GetObjectOptions{}
	opt.Set("Cache-Control", "no-cache")

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
	}

	// update
	if obj != nil {
		record := new(Record)
		if err = json.NewDecoder(obj).Decode(record); err != nil {
			web.AbortErr(ctx, errors.Wrap(err, "decode record"))
			return
		}

		var matched = false
		for _, item := range record.Records {
			if item.Name == req.Name {
				item.FileID = req.FileID
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

	// create
	record := new(Record)
	record.Records = append(record.Records, recordItem{
		Name:   req.Name,
		FileID: req.FileID,
	})

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

	logger.Info("create record",
		zap.String("name", req.Name),
		zap.String("file_id", req.FileID),
	)
	ctx.JSON(200, gin.H{
		"msg": "ok",
	})
}
