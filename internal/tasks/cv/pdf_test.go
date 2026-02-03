package cv

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExtractMarkdownTitle verifies the first H1 is used as the PDF title.
func TestExtractMarkdownTitle(t *testing.T) {
	t.Parallel()

	title := extractMarkdownTitle("# My CV\n\n## Experience\nContent")
	require.Equal(t, "My CV", title)

	fallback := extractMarkdownTitle("## Experience\nContent")
	require.Equal(t, "CV", fallback)
}

// TestCVPDFRendererBuildHTML verifies the template wraps rendered HTML.
func TestCVPDFRendererBuildHTML(t *testing.T) {
	t.Parallel()

	renderer, err := NewCVPDFRenderer()
	require.NoError(t, err)

	htmlDoc, err := renderer.buildHTML("Sample CV", "<h1>Sample</h1><p>Body</p>")
	require.NoError(t, err)
	require.True(t, strings.Contains(htmlDoc, "<title>Sample CV</title>"))
	require.True(t, strings.Contains(htmlDoc, "<h1>Sample</h1>"))
	require.True(t, strings.Contains(htmlDoc, "<p>Body</p>"))
}
