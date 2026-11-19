package cv

import (
	"context"
	"strconv"
	"sync"
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

	// saveLock, a one-slot channel, makes committing a save to the content store and recording it as the latest
	// save one critical section, so the record follows the store's commit order.
	saveLock chan struct{}
	// writeMu guards the key and generation of the content that SaveContent committed last in this process. Only
	// SaveContent changes them, and they are the fallback authority when the latest content cannot be read.
	writeMu        sync.Mutex
	latestWriteKey string
	latestWriteGen uint64

	// refreshMu guards refreshRetryAt, the monotonic time before which public downloads start no new refresh.
	refreshMu      sync.Mutex
	refreshRetryAt time.Time

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
		saveLock:   make(chan struct{}, 1),
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

// SaveContent saves content as the latest CV and records it as the latest save in this process.
//
// Committing to the content store and recording the save happen in one critical section, so when the latest content
// cannot be read before an upload, the save the store committed last is the one that may still upload its PDF.
// Saves meant to be followed by RenderAndStore go through here. It returns the saved payload, or the content
// store's error.
func (s *PDFService) SaveContent(ctx context.Context, content string) (ContentPayload, error) {
	// A free lock is taken even when ctx has ended, so the content store decides, as before, whether a save from a
	// departing client still commits; only waiting behind another save honors ctx.
	select {
	case s.saveLock <- struct{}{}:
	default:
		select {
		case s.saveLock <- struct{}{}:
		case <-ctx.Done():
			return ContentPayload{}, errors.Wrap(ctx.Err(), "wait for an earlier cv content save")
		}
	}
	defer func() { <-s.saveLock }()

	payload, err := s.content.Save(ctx, content)
	if err != nil {
		return ContentPayload{}, errors.Wrap(err, "save cv content")
	}
	s.recordSave(payload.Content)
	return payload, nil
}

// RenderAndStore renders markdown into a PDF and persists it to object storage while it is the latest content.
//
// content is normally what SaveContent just saved. Callers storing the same content concurrently share one render
// and upload, which completes even when every caller leaves; a failure that no caller receives is logged once. A
// caller whose context has already ended still starts the write in the background, because its content was saved
// and the PDF must follow it. It returns the outcome, or an error.
func (s *PDFService) RenderAndStore(ctx context.Context, content string) (PDFStoreOutcome, error) {
	key := s.storeKey(content)
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
// no new refresh for pdfRefreshFailureBackoff, so a lasting outage costs one read and one warning per window. The
// refresh logs a failure to read the content, or a failed or timed-out render or upload, once and at warning level.
func (s *PDFService) refreshLatest(ctx context.Context) {
	if s.refreshBackingOff() {
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

		key := s.storeKey(payload.Content)
		store := s.flights.join(flightCtx, key, pdfWriteFlight, true, s.storeWork(payload.Content))
		// The refresh waits for the store work as a caller, past the refresh's own deadline, which started before the
		// content read: the store work ends within its own timeout, and holding the refresh until then keeps later
		// downloads from reading the content again. As a caller the refresh receives the failure and reports it as
		// the warning background refreshes have always logged, so an anonymous download never raises an
		// error-level alert.
		if _, err := s.flights.wait(context.WithoutCancel(flightCtx), key, store); err != nil {
			s.backOffRefresh()
			logger.Warn("cv pdf async refresh render failed",
				zap.Duration("retry_after", pdfRefreshFailureBackoff), zap.Error(err))
		}
		return pdfFlightValue{}, nil
	})
}

// backOffRefresh stops public downloads from starting a new refresh for pdfRefreshFailureBackoff. The deadline
// keeps time.Now's monotonic reading, so a wall-clock step does not stretch or shorten the window.
func (s *PDFService) backOffRefresh() {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.refreshRetryAt = time.Now().Add(pdfRefreshFailureBackoff)
}

// refreshBackingOff reports whether a failed refresh's backoff window is still open.
func (s *PDFService) refreshBackingOff() bool {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	return time.Now().Before(s.refreshRetryAt)
}

// recordSave records content, which the content store has just committed, as the latest save in this process.
//
// A save of other content than the previous one starts a newer generation, so a write for that content never joins
// a flight that an intervening save made stale.
func (s *PDFService) recordSave(content string) {
	key := pdfDocumentKey("store", content)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if key != s.latestWriteKey {
		s.latestWriteGen++
		s.latestWriteKey = key
	}
}

// storeKey returns the flight key for storing content, without recording anything.
//
// A write of the content saved last joins that save's flight. A write of other content, such as a refresh of
// content read just before a newer save, or an older save that reaches its PDF late, gets a key that no later save
// joins, because such a save starts a newer generation. It returns the flight key.
func (s *PDFService) storeKey(content string) string {
	key := pdfDocumentKey("store", content)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return key + "#" + strconv.FormatUint(s.latestWriteGen, 10)
}

// isLatestSave reports whether content is what SaveContent committed last in this process.
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
// When the latest content cannot be read, it falls back to the content SaveContent committed last in this process,
// which follows the content store's commit order, and logs that failure once per write. The save the store
// committed last still stores its PDF, so a transient read failure does not turn a successful save into an error.
// A write for any other content, such as an older save that reaches its PDF late or a refresh of content this
// process did not save last, does not store its PDF, so it cannot overwrite a newer PDF that would then stay wrong
// until the next save. Saves made by other processes are not known here.
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
