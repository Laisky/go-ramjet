package cv

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/phpdave11/gofpdf"
	"github.com/stretchr/testify/require"
)

// TestRenderRecommendationLettersPDFWithFetcher verifies recommendation letters render into a multi-page PDF.
// It takes a testing.T and returns no values.
func TestRenderRecommendationLettersPDFWithFetcher(t *testing.T) {
	t.Parallel()

	imagePayload := buildTestPNG(t, 320, 640)
	fetcher := func(_ context.Context, _ string) (recommendationImage, error) {
		return recommendationImage{
			Payload:   imagePayload,
			ImageType: "PNG",
			WidthPx:   320,
			HeightPx:  640,
		}, nil
	}

	letters := []pdfRecommendationLetter{
		{Label: "A", ImageURL: "mock://a"},
		{Label: "B", ImageURL: "mock://b"},
	}

	pdfBytes, err := renderRecommendationLettersPDFWithFetcher(context.Background(), letters, fetcher)
	require.NoError(t, err)
	require.NotEmpty(t, pdfBytes)

	pageCount, err := api.PageCount(bytes.NewReader(pdfBytes), nil)
	require.NoError(t, err)
	require.Equal(t, 2, pageCount)
}

// TestMergePDFBytes verifies PDF byte slices are merged into a single PDF.
// It takes a testing.T and returns no values.
func TestMergePDFBytes(t *testing.T) {
	t.Parallel()

	first := buildTestPDF(t, "First")
	second := buildTestPDF(t, "Second")

	merged, err := mergePDFBytes(context.Background(), first, second)
	require.NoError(t, err)
	require.NotEmpty(t, merged)

	pageCount, err := api.PageCount(bytes.NewReader(merged), nil)
	require.NoError(t, err)
	require.Equal(t, 2, pageCount)
}

// TestCVPDFTemplateStyling verifies the PDF template includes layout rules for consistent pagination.
// It takes a testing.T and returns no values.
func TestCVPDFTemplateStyling(t *testing.T) {
	t.Parallel()

	require.NotContains(t, cvPDFTemplate, "--cv-paper")
	require.Contains(t, cvPDFTemplate, "background: #ffffff;")
	require.Contains(t, cvPDFTemplate, "break-after: auto;")
	require.Contains(t, cvPDFTemplate, "page-break-after: auto;")
	require.Contains(t, cvPDFTemplate, "break-inside: auto;")
}

// TestCVPDFTemplateCJKFontStack verifies the PDF HTML template ships a font
// stack that covers CJK scripts. Without these fallbacks headless Chrome
// renders Chinese / Japanese / Korean characters as tofu in the output PDF.
// It takes a testing.T and returns no values.
func TestCVPDFTemplateCJKFontStack(t *testing.T) {
	t.Parallel()

	// Web font families served from Google Fonts CDN — required for dev or any
	// environment that lacks system CJK fonts.
	require.Contains(t, cvPDFTemplate, "Noto+Sans+SC")
	require.Contains(t, cvPDFTemplate, "Noto+Sans+TC")
	require.Contains(t, cvPDFTemplate, "Noto+Sans+JP")
	require.Contains(t, cvPDFTemplate, "Noto+Sans+KR")
	require.Contains(t, cvPDFTemplate, "Noto+Serif+SC")

	// CSS family names — what Chromium matches against once the font is
	// available (either from CDN above or from fonts-noto-cjk in the image).
	require.Contains(t, cvPDFTemplate, `"Noto Sans SC"`)
	require.Contains(t, cvPDFTemplate, `"Noto Serif SC"`)
	require.Contains(t, cvPDFTemplate, `"Noto Sans CJK SC"`)
	require.Contains(t, cvPDFTemplate, `"Noto Serif CJK SC"`)
}

// TestCVPDFRendererPreservesCJK verifies the markdown → HTML stage emits raw
// CJK code points unchanged, ruling out byte mangling upstream of Chromium.
// It takes a testing.T and returns no values.
func TestCVPDFRendererPreservesCJK(t *testing.T) {
	t.Parallel()

	renderer, err := NewCVPDFRenderer()
	require.NoError(t, err)

	const cjk = "# 张三的简历\n\n你好世界。これは履歴書です。안녕하세요。"
	htmlBody, err := renderer.renderMarkdown(cjk)
	require.NoError(t, err)
	require.Contains(t, htmlBody, "张三的简历")
	require.Contains(t, htmlBody, "你好世界")
	require.Contains(t, htmlBody, "これは履歴書です")
	require.Contains(t, htmlBody, "안녕하세요")

	doc, err := renderer.buildHTML(extractMarkdownTitle(cjk), htmlBody)
	require.NoError(t, err)
	require.Contains(t, doc, "<title>张三的简历</title>")
	require.Contains(t, doc, "你好世界")
}

// buildTestPNG creates a solid PNG image for tests.
// It takes a testing.T plus width/height and returns the PNG bytes.
func buildTestPNG(t *testing.T, width int, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 180, B: 120, A: 255})
		}
	}

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// buildTestPDF creates a one-page PDF with a text label.
// It takes a testing.T and label text and returns the PDF bytes.
func buildTestPDF(t *testing.T, label string) []byte {
	t.Helper()

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 16)
	pdf.Cell(40, 10, label)
	require.NoError(t, pdf.Error())

	var buf bytes.Buffer
	require.NoError(t, pdf.Output(&buf))
	return buf.Bytes()
}
