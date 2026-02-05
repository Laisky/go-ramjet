package cv

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/phpdave11/gofpdf"
)

const (
	recommendationFetchTimeout  = 12 * time.Second
	recommendationMaxImageBytes = 20 << 20
	recommendationPageWidthMM   = 210.0
	recommendationPageHeightMM  = 297.0
	recommendationPageMarginMM  = 10.0
)

type pdfRecommendationLetter struct {
	Label    string
	ImageURL string
}

type recommendationImage struct {
	Payload   []byte
	ImageType string
	WidthPx   int
	HeightPx  int
}

type recommendationImageFetcher func(ctx context.Context, imageURL string) (recommendationImage, error)

var cvRecommendationLetters = []pdfRecommendationLetter{
	{
		Label:    "Basebit",
		ImageURL: "https://s3.laisky.com/public/personal/cv/laisky/recommend-letter-bbt.JPG",
	},
	{
		Label:    "Pateo",
		ImageURL: "https://s3.laisky.com/public/personal/cv/laisky/recommend-letter-pateo.JPG",
	},
}

// renderRecommendationLettersPDF renders each recommendation letter image into a dedicated PDF page.
// It takes the request context and letters, and returns the PDF bytes or an error.
func renderRecommendationLettersPDF(ctx context.Context, letters []pdfRecommendationLetter) ([]byte, error) {
	return renderRecommendationLettersPDFWithFetcher(ctx, letters, fetchRecommendationImage)
}

// renderRecommendationLettersPDFWithFetcher renders recommendation letters using a custom image fetcher.
// It takes the context, letters, and fetcher, and returns the PDF bytes or an error.
func renderRecommendationLettersPDFWithFetcher(
	ctx context.Context,
	letters []pdfRecommendationLetter,
	fetcher recommendationImageFetcher,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}
	if len(letters) == 0 {
		return nil, nil
	}
	if fetcher == nil {
		return nil, errors.WithStack(errors.New("recommendation image fetcher is nil"))
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(false, 0)

	for idx, letter := range letters {
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, "context done")
		}
		imageData, err := fetcher(ctx, letter.ImageURL)
		if err != nil {
			return nil, errors.Wrapf(err, "fetch recommendation image %s", letter.Label)
		}

		imageName := buildRecommendationImageName(idx, letter.Label)
		options := gofpdf.ImageOptions{ImageType: imageData.ImageType, ReadDpi: true}
		pdf.RegisterImageOptionsReader(imageName, options, bytes.NewReader(imageData.Payload))
		if err := pdf.Error(); err != nil {
			return nil, errors.Wrap(err, "register recommendation image")
		}

		pdf.AddPage()
		placeRecommendationImage(pdf, imageName, imageData, options)
		if err := pdf.Error(); err != nil {
			return nil, errors.Wrap(err, "place recommendation image")
		}
	}

	if err := pdf.Error(); err != nil {
		return nil, errors.Wrap(err, "render recommendation letters")
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, errors.Wrap(err, "output recommendation letters pdf")
	}

	return buf.Bytes(), nil
}

// fetchRecommendationImage downloads an image and returns its payload plus metadata.
// It takes a context and image URL, and returns the image payload or an error.
func fetchRecommendationImage(ctx context.Context, imageURL string) (recommendationImage, error) {
	if err := ctx.Err(); err != nil {
		return recommendationImage{}, errors.Wrap(err, "context done")
	}
	if strings.TrimSpace(imageURL) == "" {
		return recommendationImage{}, errors.WithStack(errors.New("image url is empty"))
	}

	fetchCtx, cancel := context.WithTimeout(ctx, recommendationFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, imageURL, nil)
	if err != nil {
		return recommendationImage{}, errors.Wrap(err, "create image request")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return recommendationImage{}, errors.Wrap(err, "fetch image")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return recommendationImage{}, errors.WithStack(errors.Errorf("image fetch status %d", resp.StatusCode))
	}

	limited := io.LimitReader(resp.Body, recommendationMaxImageBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return recommendationImage{}, errors.Wrap(err, "read image payload")
	}
	if len(payload) == 0 {
		return recommendationImage{}, errors.WithStack(errors.New("image payload is empty"))
	}
	if len(payload) > recommendationMaxImageBytes {
		return recommendationImage{}, errors.WithStack(errors.New("image payload exceeds limit"))
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(payload))
	if err != nil {
		return recommendationImage{}, errors.Wrap(err, "decode image config")
	}

	imageType, err := normalizeImageType(format)
	if err != nil {
		return recommendationImage{}, errors.Wrap(err, "normalize image type")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return recommendationImage{}, errors.WithStack(errors.New("image dimensions are invalid"))
	}

	return recommendationImage{
		Payload:   payload,
		ImageType: imageType,
		WidthPx:   cfg.Width,
		HeightPx:  cfg.Height,
	}, nil
}

// normalizeImageType maps decoded image formats into gofpdf image type names.
// It takes the decoded format name and returns the gofpdf image type or an error.
func normalizeImageType(format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "jpeg", "jpg":
		return "JPG", nil
	case "png":
		return "PNG", nil
	case "gif":
		return "GIF", nil
	default:
		return "", errors.WithStack(errors.Errorf("unsupported image format %q", format))
	}
}

// buildRecommendationImageName creates a stable image name for gofpdf registration.
// It takes the index and label and returns the generated image name.
func buildRecommendationImageName(index int, label string) string {
	safeLabel := strings.TrimSpace(strings.ToLower(label))
	safeLabel = strings.ReplaceAll(safeLabel, " ", "-")
	if safeLabel == "" {
		safeLabel = "letter"
	}
	return "recommendation-" + safeLabel + "-" + strconv.Itoa(index+1)
}

// placeRecommendationImage computes sizing and adds the image to the PDF page.
// It takes the PDF instance, image name, image metadata, and image options and returns no values.
func placeRecommendationImage(
	pdf *gofpdf.Fpdf,
	imageName string,
	imageData recommendationImage,
	options gofpdf.ImageOptions,
) {
	if pdf == nil {
		return
	}
	availableWidth := recommendationPageWidthMM - 2*recommendationPageMarginMM
	availableHeight := recommendationPageHeightMM - 2*recommendationPageMarginMM

	widthPx := float64(imageData.WidthPx)
	heightPx := float64(imageData.HeightPx)
	scale := math.Min(availableWidth/widthPx, availableHeight/heightPx)
	imageWidth := widthPx * scale
	imageHeight := heightPx * scale
	x := (recommendationPageWidthMM - imageWidth) / 2
	y := (recommendationPageHeightMM - imageHeight) / 2

	pdf.ImageOptions(imageName, x, y, imageWidth, imageHeight, false, options, 0, "")
}

// mergePDFBytes merges multiple PDF byte slices into a single PDF.
// It takes the context and PDF byte slices and returns the merged PDF bytes or an error.
func mergePDFBytes(ctx context.Context, pdfs ...[]byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}
	if len(pdfs) == 0 {
		return nil, errors.WithStack(errors.New("no pdf payloads provided"))
	}

	readers := make([]io.ReadSeeker, 0, len(pdfs))
	for idx, payload := range pdfs {
		if len(payload) == 0 {
			return nil, errors.WithStack(errors.Errorf("pdf payload %d is empty", idx))
		}
		readers = append(readers, bytes.NewReader(payload))
	}

	var merged bytes.Buffer
	if err := api.MergeRaw(readers, &merged, false, nil); err != nil {
		return nil, errors.Wrap(err, "merge pdf payloads")
	}

	return merged.Bytes(), nil
}
