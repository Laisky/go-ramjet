package cv

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"golang.org/x/sync/semaphore"
)

// PDFStoreOutcome reports what RenderAndStore did with the rendered PDF.
type PDFStoreOutcome string

const (
	// PDFStoreUploaded means the PDF for the requested content was uploaded.
	PDFStoreUploaded PDFStoreOutcome = "uploaded"
	// PDFStoreSkippedSuperseded means newer content was saved first, so this PDF was not rendered or not uploaded.
	PDFStoreSkippedSuperseded PDFStoreOutcome = "skipped_superseded"
)

// PDFService renders CV markdown into PDF and uploads it to object storage.
//
// Concurrent requests for the same document share one render, renders are bounded process-wide, and a write renders
// and uploads only while its content is still the latest saved CV, so an older save never overwrites a newer PDF.
type PDFService struct {
	renderer PDFRenderer
	store    *S3PDFStore
	content  ContentRepository
	slots    *semaphore.Weighted
	flights  *pdfFlights

	// writeMu guards the key and generation of the latest save requested in this process. Only saves change them,
	// so they name the content this process last saved, the fallback authority when the content cannot be read.
	writeMu        sync.Mutex
	latestWriteKey string
	latestWriteGen uint64

	// refreshRetryAt is the Unix time in nanoseconds before which public downloads start no new refresh.
	refreshRetryAt atomic.Int64

	// uploadLock serializes the check-then-upload step of writes, honoring each write's context while it waits.
	// It serializes uploads within this process only; replicas sharing one bucket would also need a conditional
	// write tied to the content version (for example If-Match on an ETag) to be fully safe.
	uploadLock *semaphore.Weighted
}

// NewPDFService creates a PDFService using the provided renderer, PDF store and CV content source.
// The content source is the authority on the latest saved CV; writes for any other content are skipped.
// It returns the configured service or an error when a dependency is missing.
func NewPDFService(renderer PDFRenderer, store *S3PDFStore, content ContentRepository) (*PDFService, error) {
	if renderer == nil {
		return nil, errors.WithStack(errors.New("pdf renderer is nil"))
	}
	if store == nil {
		return nil, errors.WithStack(errors.New("pdf store is nil"))
	}
	if content == nil {
		return nil, errors.WithStack(errors.New("pdf content source is nil"))
	}

	return &PDFService{
		renderer:   renderer,
		store:      store,
		content:    content,
		slots:      pdfRenderSlots,
		flights:    newPDFFlights(pdfSharedRenderTimeout),
		uploadLock: semaphore.NewWeighted(1),
	}, nil
}

// Render converts markdown content into a PDF document and returns the PDF bytes or an error.
//
// Callers rendering the same content concurrently share one render. A caller that cancels stops waiting only for
// itself, and the render stops once every caller has left.
func (s *PDFService) Render(ctx context.Context, content string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}
	if s.renderer == nil {
		return nil, errors.WithStack(errors.New("pdf renderer is nil"))
	}

	key := pdfDocumentKey("render", content)
	flight := s.flights.join(ctx, key, pdfReadOnlyFlight, true, func(flightCtx context.Context) (pdfFlightValue, error) {
		pdfBytes, _, err := s.renderWithSlot(flightCtx, content, nil)
		if err != nil {
			return pdfFlightValue{}, err
		}
		return pdfFlightValue{pdf: pdfBytes}, nil
	})
	value, err := s.flights.wait(ctx, key, flight)
	if err != nil {
		return nil, err
	}
	return value.pdf, nil
}

// renderWithSlot renders content while holding one process-wide render slot; ctx bounds both the wait and render.
// When proceed is not nil it runs once the slot is held and may decline, which returns ok=false without rendering.
// It returns the PDF bytes and ok=true after rendering, or an error.
func (s *PDFService) renderWithSlot(ctx context.Context, content string, proceed func(context.Context) bool) (pdfBytes []byte, ok bool, err error) {
	if err := s.slots.Acquire(ctx, 1); err != nil {
		return nil, false, errors.Wrap(err, "acquire PDF render slot")
	}
	defer s.slots.Release(1)
	if proceed != nil && !proceed(ctx) {
		return nil, false, nil
	}

	pdfBytes, err = s.renderer.Render(ctx, content)
	if err != nil {
		return nil, false, errors.Wrap(err, "render pdf")
	}
	return pdfBytes, true, nil
}

// RenderAndStore renders markdown into a PDF and persists it to object storage while it is the latest content.
//
// Callers storing the same content concurrently share one render and upload, which completes even when every
// caller leaves; a failure that no caller receives is logged once. A caller whose context has already ended still
// starts the write in the background, because its content was saved and the PDF must follow it. It returns the
// outcome, or an error.
func (s *PDFService) RenderAndStore(ctx context.Context, content string) (PDFStoreOutcome, error) {
	key := s.writeKey(content)
	if err := ctx.Err(); err != nil {
		s.flights.join(ctx, key, pdfWriteFlight, false, s.storeWork(content))
		return "", errors.Wrap(err, "caller left; storing the cv pdf continues in the background")
	}

	flight := s.flights.join(ctx, key, pdfWriteFlight, true, s.storeWork(content))
	value, err := s.flights.wait(ctx, key, flight)
	if err != nil {
		return "", errors.Wrap(err, "render and store cv pdf")
	}
	return value.outcome, nil
}

// pdfRefreshKey identifies the one background refresh of the latest saved CV that may run at a time.
const pdfRefreshKey = "refresh:latest"

// pdfRefreshFailureBackoff is how long public downloads stop starting refreshes after a refresh failed.
const pdfRefreshFailureBackoff = 15 * time.Second

// refreshLatest stores the PDF of the latest saved CV in the background and returns without waiting.
//
// ctx only supplies the logger for the shared work; its cancellation and deadline are ignored, because the refresh
// must outlive the download that triggered it and is bounded by the shared flight timeout instead.
//
// Concurrent refreshes share one read of the latest content and the render and upload that follow, until that work
// ends, so a burst of public downloads of a missing PDF costs one content read. The store work is joined like any
// other write, so a refresh and a save of the same content share one render. After a refresh fails, downloads start
// no new refresh for pdfRefreshFailureBackoff, so a lasting outage costs one read and one warning per window. A
// failure to read the content is logged once by the refresh; a render or upload failure is reported once by the
// store work.
func (s *PDFService) refreshLatest(ctx context.Context) {
	if time.Now().UnixNano() < s.refreshRetryAt.Load() {
		return
	}
	s.flights.join(ctx, pdfRefreshKey, pdfWriteFlight, false, func(flightCtx context.Context) (pdfFlightValue, error) {
		logger := gmw.GetLogger(flightCtx)
		payload, err := s.content.Load(flightCtx)
		if err != nil {
			s.backOffRefresh()
			logger.Warn("cv pdf async refresh load failed",
				zap.Duration("retry_after", pdfRefreshFailureBackoff), zap.Error(err))
			return pdfFlightValue{}, nil
		}
		logger.Debug("cv pdf refresh started",
			zap.Int("content_bytes", len(payload.Content)),
			zap.String("updated_at", formatUpdatedAt(payload.UpdatedAt)))

		key := s.refreshWriteKey(payload.Content)
		store := s.flights.join(flightCtx, key, pdfWriteFlight, false, s.storeWork(payload.Content))
		// Holding the refresh until the store work ends keeps later downloads from reading the content again. The
		// refresh does not wait as a caller, so the store work, bounded by its own timeout, reports its own outcome.
		select {
		case <-store.done:
			// The store work's error is final once done is closed; it was reported by that work or its caller.
			if store.err != nil {
				s.backOffRefresh()
			}
		case <-flightCtx.Done():
		}
		return pdfFlightValue{}, nil
	})
}

// backOffRefresh stops public downloads from starting a new refresh for pdfRefreshFailureBackoff.
func (s *PDFService) backOffRefresh() {
	s.refreshRetryAt.Store(time.Now().Add(pdfRefreshFailureBackoff).UnixNano())
}

// writeKey returns the flight key for storing content.
//
// A write joins in-flight work for the same content only when no write for other content was requested since;
// otherwise it starts a newer generation, so a save never joins a flight that an intervening save made stale.
func (s *PDFService) writeKey(content string) string {
	key := pdfDocumentKey("store", content)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if key != s.latestWriteKey {
		s.latestWriteGen++
		s.latestWriteKey = key
	}
	return key + "#" + strconv.FormatUint(s.latestWriteGen, 10)
}

// refreshWriteKey returns the flight key for a refresh storing content without recording content as a save.
//
// A refresh of the content saved last joins that save's flight. A refresh of other content, for example content
// read just before a newer save, gets a key that no later save joins, because such a save starts a newer generation.
// It returns the flight key.
func (s *PDFService) refreshWriteKey(content string) string {
	key := pdfDocumentKey("store", content)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return key + "#" + strconv.FormatUint(s.latestWriteGen, 10)
}

// isLatestSave reports whether content is the content saved last through this service in this process.
func (s *PDFService) isLatestSave(content string) bool {
	key := pdfDocumentKey("store", content)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return key == s.latestWriteKey
}

// storeWork returns the shared work that renders content and uploads it while it is still the latest saved CV.
//
// Currency is checked before waiting for a render slot, again once the slot is held, and finally right before the
// upload, so superseded saves neither occupy render slots nor overwrite a newer PDF.
func (s *PDFService) storeWork(content string) pdfFlightWork {
	return func(ctx context.Context) (pdfFlightValue, error) {
		check := &pdfCurrencyCheck{source: s.content, want: content, latestSave: s.isLatestSave}
		skipped := pdfFlightValue{outcome: PDFStoreSkippedSuperseded}
		if !check.current(ctx) {
			return skipped, nil
		}
		pdfBytes, rendered, err := s.renderWithSlot(ctx, content, check.current)
		if err != nil {
			return pdfFlightValue{}, errors.Wrap(err, "render cv pdf")
		}
		if !rendered {
			return skipped, nil
		}
		return s.storeIfCurrent(ctx, check, pdfBytes)
	}
}

// storeIfCurrent uploads pdfBytes only while check still reports the latest saved CV content.
//
// Uploads are serialized and each re-checks the content source right before writing, so whichever upload runs
// last belongs to the latest content. It returns the outcome, or an error from waiting or uploading.
func (s *PDFService) storeIfCurrent(ctx context.Context, check *pdfCurrencyCheck, pdfBytes []byte) (pdfFlightValue, error) {
	if err := s.uploadLock.Acquire(ctx, 1); err != nil {
		return pdfFlightValue{}, errors.Wrap(err, "acquire cv pdf upload lock")
	}
	defer s.uploadLock.Release(1)

	if !check.current(ctx) {
		return pdfFlightValue{outcome: PDFStoreSkippedSuperseded}, nil
	}
	if err := s.store.Save(ctx, pdfBytes); err != nil {
		return pdfFlightValue{}, errors.Wrap(err, "save cv pdf")
	}
	return pdfFlightValue{outcome: PDFStoreUploaded}, nil
}

// pdfCurrencyCheck tells whether one write's content is still the latest saved CV. It is used by one flight only.
type pdfCurrencyCheck struct {
	source     ContentRepository
	want       string
	latestSave func(content string) bool
	logged     bool
}

// current reports whether the wanted content is the latest saved CV.
//
// When the latest content cannot be read, it falls back to the content saved last in this process and logs that
// failure once per write. A save that no newer save in this process replaced still stores its PDF, so a transient
// read failure does not turn a successful save into an error. A write for any other content, such as an older save
// or a refresh of content this process did not save last, does not store its PDF, so it cannot overwrite a newer
// PDF that would then stay wrong until the next save.
func (c *pdfCurrencyCheck) current(ctx context.Context) bool {
	latest, err := c.source.Load(ctx)
	if err != nil {
		isLatest := c.latestSave(c.want)
		if !c.logged {
			c.logged = true
			gmw.GetLogger(ctx).Warn("cannot confirm the cv content is current; deciding from the latest save in this process",
				zap.Bool("latest_save", isLatest), zap.Error(err))
		}
		return isLatest
	}
	return latest.Content == c.want
}
