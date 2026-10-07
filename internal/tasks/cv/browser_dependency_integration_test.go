//go:build browser_integration

package cv

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/stretchr/testify/require"
)

// TestBrowserPDFWaitsForFontsAndKeepsPagination tests real Chrome, typed CDP commands, and PDF decoding against a self-contained document.
func TestBrowserPDFWaitsForFontsAndKeepsPagination(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	const document = `<!doctype html><html><head><meta charset="utf-8">
<style>@page {size: A4} .second {break-before: page}</style></head>
<body><h1 id="ready">Not ready</h1><p class="second">Second page</p>
<script>Object.defineProperty(document.fonts, 'ready', {value: new Promise(resolve => {
setTimeout(() => { document.getElementById('ready').textContent = 'Font-ready migration'; resolve(document.fonts); }, 500);
})});</script></body></html>`
	payload, err := renderHTMLToPDF(ctx, document)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(payload, []byte("%PDF-")))
	pages, err := api.PageCount(ctx, bytes.NewReader(payload), model.NewDefaultConfiguration())
	require.NoError(t, err)
	require.Equal(t, 2, pages)
	command := exec.CommandContext(ctx, "pdftotext", "-", "-")
	command.Stdin = bytes.NewReader(payload)
	text, err := command.CombinedOutput()
	require.NoError(t, err, string(text))
	require.Contains(t, string(text), "Font-ready migration")
	require.Contains(t, string(text), "Second page")
	require.NotContains(t, string(text), "Not ready")
}

// TestBrowserPDFRejectsFontPromiseFailure verifies JavaScript exceptions still fail the rendering request.
func TestBrowserPDFRejectsFontPromiseFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	payload, err := renderHTMLToPDF(ctx, `<html><body><script>
Object.defineProperty(document.fonts, 'ready', {value: Promise.reject(new Error('font load failed'))});
</script></body></html>`)
	require.Error(t, err)
	require.Nil(t, payload)
	require.Contains(t, err.Error(), "font load failed")
}
