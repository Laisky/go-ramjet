package prompt

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var benchmarkRenderedPrompt string

// TestRender_ByteForByteCompatibility verifies that ReactRenderer.Render remains byte-for-byte
// compatible with the pre-optimization output. The t parameter records assertion failures; the
// function returns no value.
func TestRender_ByteForByteCompatibility(t *testing.T) {
	t.Parallel()

	const wantDigest = "edf576235a6c44d9f27040f36603dbdae4d10abc2823433e9da26042a7c9f2d0"
	got := NewReactRenderer(20).Render(7, 13)
	gotDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(got)))
	require.Equalf(t, wantDigest, gotDigest, "rendered prompt changed: len=%d", len(got))
}

// BenchmarkReactRendererRender measures the runtime and allocations of ReactRenderer.Render. The b
// parameter controls benchmark iterations and records metrics; the function returns no value.
func BenchmarkReactRendererRender(b *testing.B) {
	renderer := NewReactRenderer(20)
	b.ReportAllocs()
	b.SetBytes(int64(len(renderer.Render(7, 13))))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchmarkRenderedPrompt = renderer.Render(7, 13)
	}
}
