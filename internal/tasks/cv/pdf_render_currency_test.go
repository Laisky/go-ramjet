package cv

import (
	"context"
	"net/http"
	"runtime"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// failingLoadContentStore saves content normally but cannot read it back, like an S3 read outage.
type failingLoadContentStore struct {
	versionedContentStore
}

// Load always fails with a transient read error.
func (s *failingLoadContentStore) Load(context.Context) (ContentPayload, error) {
	return ContentPayload{}, errors.New("transient content read failure")
}

// exitingPDFRenderer ends its goroutine with runtime.Goexit, as a misused t.FailNow in a renderer would.
type exitingPDFRenderer struct{}

// Render exits the calling goroutine without returning.
func (exitingPDFRenderer) Render(context.Context, string) ([]byte, error) {
	runtime.Goexit()
	return nil, nil
}

// TestSaveContentAfterClientLeftStillStoresPDF verifies a save whose client disconnected right after the content
// was written still renders and stores the matching PDF in the background.
func TestSaveContentAfterClientLeftStillStoresPDF(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		defer renderer.openAll()
		content := &versionedContentStore{current: "# Old"}
		service, pdfStore, _ := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

		goneCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := saveCVContent(goneCtx, h, nil, "# New")
		require.NoError(t, err)
		require.Equal(t, "# New", content.current)

		synctest.Wait()
		renderer.open(0)
		synctest.Wait()
		require.Equal(t, "%PDF-1.4 # New", storedPDF(t, pdfStore), "the PDF must follow content saved before the client left")
	})
}

// TestPDFServiceOutdatedSavesDoNotRender verifies saves superseded while waiting for a render slot give the slot
// back without rendering, so the latest save renders as soon as a slot frees up.
func TestPDFServiceOutdatedSavesDoNotRender(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		defer renderer.openAll()
		content := &versionedContentStore{}
		service, pdfStore, _ := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

		var wg sync.WaitGroup
		previewErrs := make([]error, 2)
		for i, text := range []string{"# P1", "# P2"} {
			wg.Go(func() { _, previewErrs[i] = service.Render(context.Background(), text) })
			synctest.Wait()
		}
		codes := make([]int, 3)
		errs := make([]error, 3)
		for i, text := range []string{"# A", "# B", "# C"} {
			wg.Go(func() { codes[i], errs[i] = saveCVContent(context.Background(), h, nil, text) })
			synctest.Wait()
		}
		renderer.open(0)
		renderer.open(1)
		synctest.Wait()
		require.Equal(t, []string{"# P1", "# P2", "# C"}, renderer.startedContents(), "outdated saves must not render")
		renderer.open(2)
		wg.Wait()

		for i := range codes {
			require.NoError(t, errs[i])
			require.Equal(t, http.StatusOK, codes[i])
		}
		require.NoError(t, previewErrs[0])
		require.NoError(t, previewErrs[1])
		require.Equal(t, "%PDF-1.4 # C", storedPDF(t, pdfStore))
	})
}

// TestPDFServiceUploadsWhenCurrencyCheckFails verifies a transient failure to read the latest content does not
// fail an otherwise successful save: the PDF for the content this save wrote is uploaded and the failure is
// logged once.
func TestPDFServiceUploadsWhenCurrencyCheckFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		defer renderer.openAll()
		logger, entries := newObservedLogger(t)
		content := &failingLoadContentStore{}
		service, pdfStore, _ := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

		var code int
		var saveErr error
		var wg sync.WaitGroup
		wg.Go(func() { code, saveErr = saveCVContent(context.Background(), h, logger, "# Saved") })
		synctest.Wait()
		renderer.open(0)
		wg.Wait()

		require.NoError(t, saveErr)
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, "%PDF-1.4 # Saved", storedPDF(t, pdfStore))
		require.Equal(t, 1, entriesMentioning(entries, "transient content read failure"), "the check failure is logged once")
	})
}

// TestPDFServiceUploadLockHonorsContext verifies an upload waiting behind another upload gives up when its shared
// work reaches the timeout instead of blocking without bound.
func TestPDFServiceUploadLockHonorsContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		defer renderer.openAll()
		service, pdfStore, _ := newFlightTestPDFService(t, renderer, &versionedContentStore{current: "# Locked"})
		release := holdUploadLock(t, service)
		defer release()

		started := time.Now()
		var saveErr error
		var wg sync.WaitGroup
		wg.Go(func() { saveErr = renderAndStoreErr(service, "# Locked") })
		synctest.Wait()
		renderer.open(0)
		wg.Wait()

		require.ErrorIs(t, saveErr, context.DeadlineExceeded)
		require.ErrorContains(t, saveErr, "upload")
		require.Equal(t, pdfSharedRenderTimeout, time.Since(started))
		require.Empty(t, storedPDF(t, pdfStore))
	})
}

// TestPDFServiceRenderGoexitDoesNotStrandCallers verifies work that ends its goroutine with runtime.Goexit still
// finishes its flight, so joined callers get an error instead of waiting forever.
func TestPDFServiceRenderGoexitDoesNotStrandCallers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		service, _, _ := newFlightTestPDFService(t, exitingPDFRenderer{}, &versionedContentStore{current: "# Exit"})

		errs := make([]error, 3)
		var wg sync.WaitGroup
		for i := range 2 {
			wg.Go(func() { _, errs[i] = service.Render(context.Background(), "# Exit") })
		}
		wg.Go(func() { errs[2] = renderAndStoreErr(service, "# Exit") })
		wg.Wait()
		for _, err := range errs {
			require.ErrorContains(t, err, "without a result")
		}
	})
}

// TestSaveContentLogsSkippedUpload verifies the save handler logs the real upload outcome: a save whose content
// was superseded before its upload reports a skipped upload, not an uploaded PDF.
func TestSaveContentLogsSkippedUpload(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		defer renderer.openAll()
		logger, entries := newObservedLogger(t)
		content := &versionedContentStore{}
		service, pdfStore, _ := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

		var code int
		var saveErr error
		var wg sync.WaitGroup
		wg.Go(func() { code, saveErr = saveCVContent(context.Background(), h, logger, "# A") })
		synctest.Wait()
		_, err := content.Save(context.Background(), "# B")
		require.NoError(t, err)
		renderer.open(0)
		wg.Wait()

		require.NoError(t, saveErr)
		require.Equal(t, http.StatusOK, code)
		require.Zero(t, entriesMentioning(entries, "cv pdf uploaded"))
		require.Equal(t, 1, entriesMentioning(entries, "cv pdf upload skipped"))
	})
}

// holdUploadLock takes the service's upload lock as another upload would and returns the function releasing it.
func holdUploadLock(t *testing.T, service *PDFService) func() {
	t.Helper()
	require.True(t, service.uploadLock.TryAcquire(1))
	return func() { service.uploadLock.Release(1) }
}

// renderAndStoreErr stores content through service for a caller that waits, and returns only the error.
func renderAndStoreErr(service *PDFService, content string) error {
	_, err := service.RenderAndStore(context.Background(), content)
	return err
}

// TestPDFServiceSupersededSaveSkipsBeforeWaitingForSlot verifies a write whose content was superseded before it
// started returns at once without queueing for a render slot, while a current write uploads.
func TestPDFServiceSupersededSaveSkipsBeforeWaitingForSlot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		defer renderer.openAll()
		content := &versionedContentStore{current: "# B"}
		service, pdfStore, _ := newFlightTestPDFService(t, renderer, content)

		var wg sync.WaitGroup
		for _, text := range []string{"# P1", "# P2"} {
			wg.Go(func() {
				_, err := service.Render(context.Background(), text)
				if err != nil {
					t.Errorf("preview %s: %v", text, err)
				}
			})
			synctest.Wait()
		}
		outcome, err := service.RenderAndStore(context.Background(), "# A")
		require.NoError(t, err)
		require.Equal(t, PDFStoreSkippedSuperseded, outcome)
		require.Equal(t, []string{"# P1", "# P2"}, renderer.startedContents())

		renderer.open(0)
		renderer.open(1)
		wg.Wait()
		var current PDFStoreOutcome
		wg.Go(func() { current, err = service.RenderAndStore(context.Background(), "# B") })
		synctest.Wait()
		renderer.open(2)
		wg.Wait()
		require.NoError(t, err)
		require.Equal(t, PDFStoreUploaded, current)
		require.Equal(t, "%PDF-1.4 # B", storedPDF(t, pdfStore))
	})
}
