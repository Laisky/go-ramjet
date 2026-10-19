package tasks

import (
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/Laisky/zap"
)

// rendererFailureResponse contains selected observed facts from one failed upstream response.
// DecodedBody is the complete available prefix, not missing wire bytes or arbitrary headers.
type rendererFailureResponse struct {
	RequestID, ParentRequestID, Kind, Method, URL, PeerAddress, Protocol string
	UpstreamStatus                                                       int
	DeclaredContentLength                                                int64
	ContentEncoding                                                      string
	TransferEncoding                                                     []string
	EncodedBodyBytes                                                     int64
	DecodedBody                                                          []byte
	DecodedBodyObserved, DecodedBodyComplete                             bool
	FailurePhase, DecodeError, DecodeCloseError, ReadError, CloseError   string
}

// fields emits selected response facts through ordinary structured logging without a separate schema or store.
func (r *rendererFailureResponse) fields() []zap.Field {
	fields := []zap.Field{
		zap.String("renderer_request_id", r.RequestID), zap.String("renderer_parent_request_id", r.ParentRequestID),
		zap.String("renderer_request_kind", r.Kind), zap.String("renderer_request_method", r.Method),
		zap.String("renderer_peer_address", r.PeerAddress), zap.String("renderer_protocol", r.Protocol),
		zap.Int("renderer_response_status", r.UpstreamStatus), zap.Int64("renderer_declared_content_length", r.DeclaredContentLength),
		zap.String("renderer_content_encoding", r.ContentEncoding), zap.Strings("renderer_transfer_encoding", r.TransferEncoding),
		zap.Int64("renderer_encoded_body_bytes", r.EncodedBodyBytes), zap.Bool("renderer_decoded_body_complete", r.DecodedBodyComplete),
		zap.String("renderer_failure_phase", r.FailurePhase), zap.String("renderer_decode_error", r.DecodeError),
		zap.String("renderer_decode_close_error", r.DecodeCloseError), zap.String("renderer_read_error", r.ReadError),
		zap.String("renderer_close_error", r.CloseError),
	}
	fields = append(fields, crawlerChosenInputFields("renderer_request_url", r.URL, true)...)
	return append(fields, crawlerChosenInputFields("renderer_decoded_body", string(r.DecodedBody), r.DecodedBodyObserved)...)
}

// rendererFailureObservation owns the first failed upstream exchange for one task.
// It never contains request headers, cookies, application credentials, or later exchanges.
type rendererFailureObservation struct {
	mu    sync.Mutex
	first *rendererFailureResponse
}

// rendererFailureObservationKey isolates the response holder from other context values.
type rendererFailureObservationKey struct{}

// withRendererFailureObservation returns a task-local context and its empty response holder.
func withRendererFailureObservation(ctx context.Context) (context.Context, *rendererFailureObservation) {
	observation := &rendererFailureObservation{}
	return context.WithValue(ctx, rendererFailureObservationKey{}, observation), observation
}

// Snapshot returns an independent copy of the first failed exchange, or nil if none failed.
func (o *rendererFailureObservation) Snapshot() *rendererFailureResponse {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return cloneRendererFailure(o.first)
}

// cloneRendererFailure copies selected metadata and independently owns captured bytes.
func cloneRendererFailure(value *rendererFailureResponse) *rendererFailureResponse {
	if value == nil {
		return nil
	}
	result := *value
	result.TransferEncoding = append([]string(nil), value.TransferEncoding...)
	result.URL = value.URL
	result.DecodedBody = bytes.Clone(value.DecodedBody)
	return &result
}

// observeRendererFailure records only the first failed exchange observed by the task.
// The response body is the available decoded prefix, never a fabricated complete response.
func observeRendererFailure(ctx context.Context, value *rendererFailureResponse) {
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
