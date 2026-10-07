package cv

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStableMarkdownRendering verifies GFM, heading IDs, raw HTML, and per-document isolation after the v2 migration.
func TestStableMarkdownRendering(t *testing.T) {
	t.Parallel()
	renderer, err := NewCVPDFRenderer()
	require.NoError(t, err)
	const source = "# Stable migration\n\n~~old~~ **new**\n\n- [x] Tested\n\n| Skill | Level |\n| --- | --- |\n| Go | Senior |\n\n<span class=\"marker\">raw</span>\n"
	for range 8 {
		t.Run("shared renderer", func(t *testing.T) {
			t.Parallel()
			body, renderErr := renderer.renderMarkdown(source)
			require.NoError(t, renderErr)
			for _, fragment := range []string{`id="stable-migration"`, "<del>old</del>", "<strong>new</strong>", "<table>", "<td>Senior</td>", `type="checkbox"`, `<span class="marker">raw</span>`} {
				require.Contains(t, body, fragment)
			}
			require.NotContains(t, body, `id="stable-migration-1"`)
		})
	}
}

// TestStablePDFMergeRejectsInvalidInputs verifies cancellation and invalid PDF inputs remain errors.
func TestStablePDFMergeRejectsInvalidInputs(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	payload, err := mergePDFBytes(ctx, []byte("not a PDF"))
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, payload)

	for _, payloads := range [][][]byte{nil, {nil}, {[]byte("not a PDF")}} {
		payload, err = mergePDFBytes(t.Context(), payloads...)
		require.Error(t, err)
		require.Nil(t, payload)
	}
}
