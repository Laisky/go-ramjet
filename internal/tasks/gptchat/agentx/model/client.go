package model

import (
	"context"

	glog "github.com/Laisky/go-utils/v6/log"

	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
)

// Client abstracts an upstream LLM behind a typed streaming API. The agent
// loop holds a Client; it has zero direct knowledge of the wire format used
// by any particular provider.
//
// Implementations must:
//
//   - Push StreamChunks onto the returned channel in upstream-order. For
//     streaming, that means deltas as they arrive; for non-streaming, the
//     adapter synthesizes a logical sequence from the final response (one
//     ChunkFunction per function_call, one ChunkText for the final
//     assistant text, then ChunkDone).
//
//   - Close the channel exactly once when the round terminates (after
//     ChunkDone or ChunkError). Closing without a terminal chunk is a bug.
//
//   - Respect ctx cancellation: when ctx is done the implementation should
//     stop reading from the upstream as soon as possible, emit a
//     ChunkError carrying ctx.Err(), and close the channel.
//
//   - Validate Request.Input shapes before issuing any upstream call.
//     Returning an error from Stream means no upstream call was made.
type Client interface {
	// Stream invokes the model and returns a typed event stream.
	// Returns an error before issuing the upstream call when the request
	// is malformed (unknown InputItem shapes, empty Model, etc).
	Stream(ctx context.Context, req Request) (<-chan StreamChunk, error)
	// Capabilities returns the static feature flags for this client.
	// Callers should consult these before setting request flags that
	// require upstream support (notably ParallelToolCalls).
	Capabilities() Capabilities
}

// OneAPIDeps captures the per-request inputs the OneAPI adapter needs to
// reach the upstream. The agent handler constructs one of these from the
// inbound request; tests use synthetic deps.
//
// The fields mirror the existing http.UpstreamDeps surface so behavior
// (headers, freetier handling, request-id echo) stays byte-identical to
// the proxy path. Logger is duplicated here so the model package can log
// its own translation/parsing steps without reaching into UpstreamDeps.
type OneAPIDeps struct {
	// UpstreamDeps supplies the user config, request headers, raw query,
	// and (optionally) a StreamSink. The adapter overwrites StreamSink
	// when Request.Stream is true so it can intercept the typed chunks
	// without re-parsing the wire bytes.
	UpstreamDeps httppkg.UpstreamDeps
	// Logger is the structured logger used for translation diagnostics.
	// When nil, logging is silently dropped.
	Logger glog.Logger
}
