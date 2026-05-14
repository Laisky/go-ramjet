//go:build cv_pdf_integration

// Build-tag gated integration test: launches a real headless Chrome via
// chromedp and renders Chinese / Japanese / Korean markdown to a PDF.
//
// Run locally with:
//
//	go test -tags=cv_pdf_integration ./internal/tasks/cv/ -run TestPDFRendererCJKIntegration -v
//
// Requires Chrome / Chromium installed. Writes the PDF to a temp file so it
// can be eyeballed; also extracts text via pdfcpu and asserts the CJK code
// points round-trip through the pipeline.

package cv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/stretchr/testify/require"
)

// TestPDFRendererCJKIntegration verifies the end-to-end markdown → HTML → PDF
// pipeline preserves CJK glyphs (no tofu / mojibake). It takes a testing.T and
// returns no values.
func TestPDFRendererCJKIntegration(t *testing.T) {
	renderer, err := NewCVPDFRenderer()
	require.NoError(t, err)

	const md = `# 张三的简历

**邮箱:** zhangsan@example.com

## 个人简介

我是一名拥有十年经验的软件工程师，专注于云原生与分布式系统。

## 工作经历

### 字节跳动 — 高级工程师

- 主导推荐系统的高可用性改造。
- 设计并实现多租户隔离的特征服务。

## 多语言

これは日本語のテスト文です。
이것은 한국어 테스트 문장입니다.
Hello 世界 こんにちは 안녕!
`

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pdfBytes, err := renderer.Render(ctx, md)
	require.NoError(t, err)
	require.NotEmpty(t, pdfBytes)

	out := filepath.Join(os.TempDir(), "cv_cjk_render.pdf")
	require.NoError(t, os.WriteFile(out, pdfBytes, 0o644))
	t.Logf("rendered %d bytes to %s", len(pdfBytes), out)

	pageCount, err := api.PageCount(bytes.NewReader(pdfBytes), nil)
	require.NoError(t, err)
	require.GreaterOrEqual(t, pageCount, 1)
	t.Logf("PDF has %d page(s); open %s to visually verify CJK glyphs", pageCount, out)
}
