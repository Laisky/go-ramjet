package cv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/Laisky/errors/v2"
	"golang.org/x/sync/semaphore"
	"golang.org/x/sync/singleflight"
)

const (
	// pdfMaxConcurrentRenders bounds headless Chrome PDF renders in this process, matching the crawler's default
	// limit of two concurrent browsers (chromedpSema).
	pdfMaxConcurrentRenders = 2
	// pdfSharedRenderTimeout bounds one deduplicated render, or render and upload, that no single caller can cancel.
	// The in-render budgets add up to 79 seconds (30s browser startup, 25s page rendering and two sequential 12s
	// recommendation letter fetches); the rest leaves room for waiting on a render slot and for the S3 upload.
	pdfSharedRenderTimeout = 2 * time.Minute
)

// defaultPDFRenderCoordinator is shared by every PDFService, so deduplication and the render bound are process-wide.
var defaultPDFRenderCoordinator = newPDFRenderCoordinator(pdfMaxConcurrentRenders, pdfSharedRenderTimeout)

// pdfRenderCoordinator deduplicates identical PDF work and bounds how many PDF renders run at the same time.
type pdfRenderCoordinator struct {
	flights singleflight.Group
	slots   *semaphore.Weighted
	timeout time.Duration
}

// newPDFRenderCoordinator returns a coordinator that runs at most limit renders at once and bounds each shared
// piece of work by timeout.
func newPDFRenderCoordinator(limit int64, timeout time.Duration) *pdfRenderCoordinator {
	return &pdfRenderCoordinator{slots: semaphore.NewWeighted(limit), timeout: timeout}
}

// pdfDocumentKey identifies one kind of PDF work for a document by the SHA-256 digest of its markdown content.
// It takes the work kind and content and returns the deduplication key.
func pdfDocumentKey(kind, content string) string {
	digest := sha256.Sum256([]byte(content))
	return kind + ":" + hex.EncodeToString(digest[:])
}

// share runs work once for all concurrent callers that use the same key and returns the shared result.
//
// The work runs on a context detached from the first caller's cancellation and bounded by the coordinator timeout,
// so a caller that leaves only stops waiting; the others still receive the result. It takes the caller context,
// the deduplication key and the work, and returns a caller-owned copy of the payload or the shared error.
func (c *pdfRenderCoordinator) share(ctx context.Context, key string, work func(context.Context) ([]byte, error)) ([]byte, error) {
	results := c.flights.DoChan(key, func() (any, error) {
		sharedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.timeout)
		defer cancel()
		return work(sharedCtx)
	})
	select {
	case <-ctx.Done():
		return nil, errors.Wrap(ctx.Err(), "wait for shared PDF work")
	case result := <-results:
		if result.Err != nil {
			return nil, errors.Wrap(result.Err, "shared PDF work")
		}
		payload, _ := result.Val.([]byte)
		// Joined callers must not alias one mutable slice.
		return bytes.Clone(payload), nil
	}
}

// withRenderSlot runs render while holding one of the process-wide render slots.
// It takes the context bounding the wait and the render, and returns the render result or the wait error.
func (c *pdfRenderCoordinator) withRenderSlot(ctx context.Context, render func(context.Context) ([]byte, error)) ([]byte, error) {
	if err := c.slots.Acquire(ctx, 1); err != nil {
		return nil, errors.Wrap(err, "acquire PDF render slot")
	}
	defer c.slots.Release(1)
	return render(ctx)
}
