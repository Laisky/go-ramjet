package tasks

import (
	"context"
	"io"
	"sync"

	"github.com/Laisky/go-ramjet/library/crawlercapture"
)

// rendererFailureObservation owns the first failed upstream exchange for one task.
// It never contains request headers, cookies, application credentials, or later exchanges.
type rendererFailureObservation struct {
	mu    sync.Mutex
	first *crawlercapture.Renderer
}

// rendererFailureObservationKey isolates the response holder from other context values.
type rendererFailureObservationKey struct{}

// withRendererFailureObservation returns a task-local context and its empty response holder.
func withRendererFailureObservation(ctx context.Context) (context.Context, *rendererFailureObservation) {
	observation := &rendererFailureObservation{}
	return context.WithValue(ctx, rendererFailureObservationKey{}, observation), observation
}

// Snapshot returns an independent copy of the first failed exchange, or nil if none failed.
func (o *rendererFailureObservation) Snapshot() *crawlercapture.Renderer {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return cloneRendererFailure(o.first)
}

// cloneRendererFailure copies selected metadata and independently owns captured bytes.
func cloneRendererFailure(value *crawlercapture.Renderer) *crawlercapture.Renderer {
	if value == nil {
		return nil
	}
	result := *value
	result.TransferEncoding = append([]string(nil), value.TransferEncoding...)
	result.URL = value.URL.Clone()
	result.DecodedBody = value.DecodedBody.Clone()
	return &result
}

// observeRendererFailure records only the first failed exchange observed by the task.
// The response body is the available decoded prefix, never a fabricated complete response.
func observeRendererFailure(ctx context.Context, value *crawlercapture.Renderer) {
	observation, _ := ctx.Value(rendererFailureObservationKey{}).(*rendererFailureObservation)
	if observation == nil || value == nil {
		return
	}
	observation.mu.Lock()
	defer observation.mu.Unlock()
	if observation.first == nil {
		observation.first = cloneRendererFailure(value)
	}
}

// rendererCountedBody counts content-encoded bytes read from the HTTP response body.
// net/http already removed HTTP chunk framing, so this is neither TCP nor full wire size.
type rendererCountedBody struct {
	io.ReadCloser
	bytes int64
}

// Read forwards body reads unchanged and counts all bytes actually returned, including partial reads.
func (b *rendererCountedBody) Read(buffer []byte) (int, error) {
	n, err := b.ReadCloser.Read(buffer)
	b.bytes += int64(n)
	return n, err
}
