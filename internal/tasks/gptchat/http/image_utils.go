package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/zap"
	"github.com/minio/minio-go/v7"
	"golang.org/x/image/webp"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
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

// isImageModel check if model is an image generation model
func isImageModel(model string) bool {
	switch {
	case strings.Contains(model, "flux-"),
		strings.Contains(model, "dall-e-"),
		strings.Contains(model, "sdxl-"),
		strings.Contains(model, "gpt-image-"),
		strings.Contains(model, "imagen-"):
		return true
	default:
		return false
	}
}

// GetImageModelPrice get price for image model
func GetImageModelPrice(model string) db.Price {
	switch model {
	case "flux-dev", "black-forest-labs/flux-dev":
		return db.PriceTxt2ImageFluxDev
	case "flux-schnell", "black-forest-labs/flux-schnell":
		return db.PriceTxt2ImageSchnell
	case "flux-pro", "black-forest-labs/flux-pro":
		return db.PriceTxt2ImageFluxPro
	case "flux-fill-pro", "black-forest-labs/flux-fill-pro":
		return db.PriceTxt2ImageFluxFillPro
	case "flux-1.1-pro", "black-forest-labs/flux-1.1-pro":
		return db.PriceTxt2ImageFluxPro11
	case "flux-kontext-pro", "black-forest-labs/flux-kontext-pro":
		return db.PriceTxt2ImageFluxKontextPro
	case "flux-1.1-pro-ultra", "black-forest-labs/flux-1.1-pro-ultra":
		return db.PriceTxt2ImageFluxProUltra11
	default:
		return db.PriceTxt2Image
	}
}

// DecodeBase64 decode base64 string, handle data:...;base64, prefix
func DecodeBase64(input string) ([]byte, error) {
	if i := strings.Index(input, ","); i != -1 {
		input = input[i+1:]
	}

	return base64.StdEncoding.DecodeString(input)
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

	// also upload prompt as a separate text file
	promptObjkey := objkeyPrefix + ".prompt.txt"
	if _, err := s3cli.PutObject(ctx,
		config.Config.S3.Bucket,
		promptObjkey,
		strings.NewReader(prompt),
		int64(len(prompt)),
		minio.PutObjectOptions{
			ContentType: "text/plain",
		}); err != nil {
		logger.Error("upload prompt text file", zap.Error(err))
		// don't return error here, as the image is already uploaded
	}

	logger.Debug("succeed upload image to s3", zap.String("objkey", objkey))
	return nil
}
