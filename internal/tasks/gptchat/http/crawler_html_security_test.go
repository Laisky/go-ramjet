package http

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExtractHTMLBodyForeignContentEscaping verifies that rendered foreign
// content cannot turn style text into an active image while ordinary HTML
// remains unchanged.
func TestExtractHTMLBodyForeignContentEscaping(t *testing.T) {
	t.Run("foreign content", func(t *testing.T) {
		input := `<math><mtext><table><mglyph><svg><desc><style><img src=x onerror=alert(1)>`
		body, err := _extractHtmlBody(strings.NewReader(input))
		require.NoError(t, err)
		require.Contains(t, string(body), `&lt;img src=x onerror=alert(1)&gt;`)
		require.NotContains(t, string(body), `<img src=x onerror=alert(1)>`)
	})

	t.Run("ordinary content", func(t *testing.T) {
		body, err := _extractHtmlBody(strings.NewReader(`<p>Hello <strong>world</strong></p>`))
		require.NoError(t, err)
		require.Equal(t, `<body><p>Hello <strong>world</strong></p></body>`, string(body))
	})
}
