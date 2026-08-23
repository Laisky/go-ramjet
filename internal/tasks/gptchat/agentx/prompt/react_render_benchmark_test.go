package prompt

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

var benchmarkRenderedPrompt string

// TestRender_ByteForByteCompatibility pins the prompt emitted by the
// pre-optimization implementation so allocation changes cannot alter model input.
func TestRender_ByteForByteCompatibility(t *testing.T) {
	t.Parallel()

	const wantDigest = "edf576235a6c44d9f27040f36603dbdae4d10abc2823433e9da26042a7c9f2d0"
	got := NewReactRenderer(20).Render(7, 13)
	gotDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(got)))
	if gotDigest != wantDigest {
		t.Fatalf("rendered prompt changed: len=%d sha256=%s", len(got), gotDigest)
	}
}

func BenchmarkReactRendererRender(b *testing.B) {
	renderer := NewReactRenderer(20)
	b.ReportAllocs()
	b.SetBytes(int64(len(renderer.Render(7, 13))))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchmarkRenderedPrompt = renderer.Render(7, 13)
	}
}
