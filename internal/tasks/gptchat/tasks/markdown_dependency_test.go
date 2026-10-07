package tasks

import (
	"bytes"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"

	"github.com/Laisky/go-ramjet/library/log"
)

// TestStableHTMLMarkdownConversion verifies local conversion preserves headings, links, lists, and code without an external model request.
func TestStableHTMLMarkdownConversion(t *testing.T) {
	t.Parallel()
	ctx := gmw.SetLogger(t.Context(), log.Logger.Named("markdown_dependency_test"))
	_, markdown, err := ExtractHTMLBody(ctx, "", []byte(`<html><body><h1>Migration</h1>
<p><strong>Important</strong> <a href="https://example.com/docs">Docs</a></p>
<ul><li>First item</li><li>Second item</li></ul><pre><code>fmt.Println("safe")</code></pre>
</body></html>`), "", true)
	require.NoError(t, err)
	source := []byte(markdown)
	document := parser.New().Parse(source)
	var body bytes.Buffer
	require.NoError(t, html.New().Render(&body, source, document))
	for _, fragment := range []string{"<h1>Migration</h1>", "<strong>Important</strong>", `<a href="https://example.com/docs">Docs</a>`, "<li>First item</li>", "<li>Second item</li>", "<pre><code>fmt.Println"} {
		require.Contains(t, body.String(), fragment)
	}
}
