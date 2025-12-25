package http

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/zap"
	"github.com/minio/minio-go/v7"
	"golang.org/x/image/webp"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/s3"
)

// ConvertImageToPNG convert image to png
func ConvertImageToPNG(imgContent []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(imgContent))
	if err != nil {
		// try webp
		img, err = webp.Decode(bytes.NewReader(imgContent))
		if err != nil {
			return nil, errors.Wrap(err, "decode image")
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, errors.Wrap(err, "encode png")
	}

	return buf.Bytes(), nil
}

func drawImageByTxtObjkeyPrefix(taskid string) string {
	year := time.Now().Format("2006")
	month := time.Now().Format("01")
	return fmt.Sprintf("create-images/%s/%s/%s", year, month, taskid)
}

func drawImageByImageObjkeyPrefix(taskid string) string {
	year := time.Now().Format("2006")
	month := time.Now().Format("01")
	return fmt.Sprintf("image-by-image/%s/%s/%s", year, month, taskid)
}

func uploadImage2Minio(ctx context.Context,
	objkeyPrefix,
	prompt string,
	imgContent []byte,
	imgExt string,
) (err error) {
	logger := gmw.GetLogger(ctx)
	s3cli, err := s3.GetCli()
	if err != nil {
		return errors.Wrap(err, "get s3 client")
	}

	objkey := objkeyPrefix + imgExt
	if _, err := s3cli.PutObject(ctx,
		config.Config.S3.Bucket,
		objkey,
		bytes.NewReader(imgContent),
		int64(len(imgContent)),
		minio.PutObjectOptions{
			ContentType: "image/png",
			UserMetadata: map[string]string{
				"prompt": prompt,
			},
		}); err != nil {
		return errors.Wrap(err, "upload image")
	}

	logger.Debug("succeed upload image to s3", zap.String("objkey", objkey))
	return nil
}
