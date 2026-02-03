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
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/internal/tasks/arweave/config"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/web"
)

var (
	httpcli *http.Client
)

func init() {
	var err error
	httpcli, err = gutils.NewHTTPClient(
		gutils.WithHTTPClientTimeout(3 * time.Minute),
	)
	if err != nil {
		log.Logger.Panic("new http client", zap.Error(err))
	}
}

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
	if err != nil {
		if minio.ToErrorResponse(err).Code != "NoSuchKey" {
			web.AbortErr(ctx, errors.Wrapf(err, "get record %q", objpath))
			return
		}

		notfound = true
	} else {
		obj.Seek(0, io.SeekStart)
	}

	// sometines, even if object is not found,
	// GetObject will return a valid object without error.
	//
	// Warning: obj.Stat() will erase the object's content,
	// so we should read the object's content before calling obj.Stat().
	// if _, err := obj.Stat(); err != nil {
	// 	if minio.ToErrorResponse(err).Code != "NoSuchKey" {
	// 		web.AbortErr(ctx, errors.Wrapf(err, "get record %q", objpath))
	// 		return
	// 	}

	// 	notfound = true
	// }

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
			Owner:  req.Owner,
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
				// check owner
				switch {
				case item.Owner == nil && (req.Owner == nil || req.Owner.TelegramUID == 861999008):
					// owner == nil means it is owned by super admin
				case item.Owner != nil && req.Owner != nil && item.Owner.TelegramUID == req.Owner.TelegramUID:
					// owner is the same
				default:
					ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
						"msg": "you are not the owner of this record",
					})
					return
				}

				if item.FileID == req.FileID {
					ctx.JSON(http.StatusOK, gin.H{
						"msg": "file_id is the same",
					})
					return
				}

				// update record
				record.Records[idx].History = append(record.Records[idx].History, historyItem{
					Time:   time.Now(),
					FileID: item.FileID,
				})
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
		return recordItem, errors.Wrapf(err, "decode record %q", objpath)
	}

	for _, recordItem = range record.Records {
		if recordItem.Name == name {
			return recordItem, nil
		}
	}

	return recordItem, errors.Errorf("record not found")
}

// Query query record by name
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

	// ctx.Redirect(http.StatusFound, fmt.Sprintf("https://ario.laisky.com/%s", record.FileID))

	// proxy to ario
	upstreamReq, err := http.NewRequest(ctx.Request.Method, fmt.Sprintf("https://ario.laisky.com/%s", record.FileID), nil)
	if web.AbortErr(ctx, err) {
		return
	}

	upstreamReq.Header = ctx.Request.Header
	upstreamReq.Header.Del("Host")
	upstreamReq.Header.Del("Referer")
	upstreamReq.Header.Del("Origin")
	upstreamReq.Header.Del("User-Agent")
	upstreamReq.Header.Del("Accept-Encoding")

	resp, err := httpcli.Do(upstreamReq)
	if web.AbortErr(ctx, err) {
		return
	}

	defer resp.Body.Close()
	for k, v := range resp.Header {
		if len(v) == 0 {
			continue
		}

		ctx.Header(k, v[0])
	}

	ctx.Header("X-Ar-File-Id", record.FileID)
	if resp.StatusCode/100 != 2 {
		ctx.AbortWithError(resp.StatusCode, errors.Errorf("fetch %q, got %d", record.FileID, resp.StatusCode))
		return
	}

	_, err = io.Copy(ctx.Writer, resp.Body)
	if web.AbortErr(ctx, errors.Wrap(err, "copy response")) {
		return
	}

	ctx.Status(resp.StatusCode)
}

var listRecordsCache = gutils.NewSingleItemExpCache[[]recordItem](10 * time.Minute)

// ListReocrds list all records
func ListReocrds(ctx *gin.Context) {
	logger := gmw.GetLogger(ctx)
	logger.Debug("list records")

	if records, ok := listRecordsCache.Get(); ok {
		ctx.JSON(http.StatusOK, records)
		return
	}

	records := make([]recordItem, 0)
	var mu sync.Mutex
	var pool errgroup.Group
	pool.SetLimit(10)
	opt := minio.ListObjectsOptions{
		Prefix:    S3Prefix,
		Recursive: true,
		MaxKeys:   1000,
	}
	for listObj := range config.Instance.S3Cli.ListObjects(gmw.Ctx(ctx),
		config.Instance.S3.Bucket, opt) {
		if listObj.Err != nil {
			web.AbortErr(ctx, errors.Wrap(listObj.Err, "list records"))
			return
		}

		listObj := listObj
		pool.Go(func() error {
			logger.Debug("get record", zap.String("key", listObj.Key))
			getopt := minio.GetObjectOptions{}
			getopt.Set("Cache-Control", "no-cache")
			getopt.SetReqParam("tt", strconv.Itoa(time.Now().Nanosecond()))
			getObj, err := config.Instance.S3Cli.GetObject(gmw.Ctx(ctx),
				config.Instance.S3.Bucket,
				listObj.Key,
				getopt,
			)
			if err != nil {
				return errors.Wrapf(err, "get record %q", listObj.Key)
			}

			record := new(Record)
			if err = json.NewDecoder(getObj).Decode(record); err != nil {
				return errors.Wrap(err, "decode record")
			}

			mu.Lock()
			for _, recordItem := range record.Records {
				records = append(records, recordItem)
			}
			mu.Unlock()

			return nil
		})
	}

	if err := pool.Wait(); err != nil {
		logger.Error("list records", zap.Error(err))
	}

	ctx.JSON(http.StatusOK, records)
	listRecordsCache.Set(records)
}
