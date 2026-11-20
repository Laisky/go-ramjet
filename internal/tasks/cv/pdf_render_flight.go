package cv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"golang.org/x/sync/semaphore"
)

const (
	// pdfMaxConcurrentRenders bounds concurrent CV PDF renders across every PDFService in this process. The crawler
	// limits its own browsers separately with chromedpSema (CHROMEDP_SEMA_LIMIT, default 2), so the process runs at
	// most pdfMaxConcurrentRenders plus that limit headless browsers at once: four with the defaults.
	pdfMaxConcurrentRenders = 2
	// pdfSharedRenderTimeout bounds one piece of shared PDF work that no single caller can cancel. The in-render
	// budgets add up to 79 seconds (30s browser startup, 25s page rendering and two sequential 12s recommendation
	// letter fetches); the rest leaves room for waiting on a render slot and for the S3 upload.
	pdfSharedRenderTimeout = 2 * time.Minute
)

// pdfRenderSlots is the process-wide limit on concurrent CV PDF renders, shared by every PDFService.
var pdfRenderSlots = semaphore.NewWeighted(pdfMaxConcurrentRenders)

// pdfFlightMode selects what happens to shared work once no caller waits for it any more.
type pdfFlightMode int

const (
	// pdfReadOnlyFlight work, such as a preview render, is canceled as soon as every waiting caller has left.
	pdfReadOnlyFlight pdfFlightMode = iota
	// pdfWriteFlight work, such as storing a saved CV, runs to completion within the flight timeout even when no
	// caller waits, and a failure that no caller receives is logged exactly once.
	pdfWriteFlight
)

// pdfFlightValue is the result of one piece of shared PDF work: rendered bytes for a render, an outcome for a write.
type pdfFlightValue struct {
	pdf     []byte
	outcome PDFStoreOutcome
}

// pdfFlightWork is shared PDF work; it receives the detached flight context.
type pdfFlightWork func(context.Context) (pdfFlightValue, error)

// pdfFlight is one piece of shared PDF work and its outcome.
type pdfFlight struct {
	done   chan struct{}
	cancel context.CancelFunc
	mode   pdfFlightMode
	// The fields below are guarded by the owning pdfFlights mutex; value and err are final once done is closed.
	waiters  int
	finished bool
	value    pdfFlightValue
	err      error
}

// pdfFlights runs at most one piece of work per key for one PDFService and tracks which callers still wait for it.
type pdfFlights struct {
	mu      sync.Mutex
	flights map[string]*pdfFlight
	timeout time.Duration
}

// newPDFFlights returns an empty flight group whose work is bounded by timeout.
func newPDFFlights(timeout time.Duration) *pdfFlights {
	return &pdfFlights{flights: map[string]*pdfFlight{}, timeout: timeout}
}

// pdfDocumentKey identifies one kind of PDF work for a document by the SHA-256 digest of its markdown content.
// It takes the work kind and content and returns the deduplication key.
func pdfDocumentKey(kind, content string) string {
	digest := sha256.Sum256([]byte(content))
	return kind + ":" + hex.EncodeToString(digest[:])
}

// join returns the in-flight work for key, starting work when none is running.
//
// New work runs on a context that keeps only the caller's logger, so it never inherits a caller's cancellation or
// holds request-scoped values (such as the pooled gin context) past the request, and it is bounded by the group
// timeout. When wait is true the caller counts as a waiter and must call wait; otherwise the caller only starts or
// joins the work. It returns the flight.
func (g *pdfFlights) join(ctx context.Context, key string, mode pdfFlightMode, wait bool, work pdfFlightWork) *pdfFlight {
	g.mu.Lock()
	defer g.mu.Unlock()
	flight, ok := g.flights[key]
	if !ok {
		flightCtx, cancel := context.WithTimeout(gmw.SetLogger(context.Background(), gmw.GetLogger(ctx)), g.timeout)
		flight = &pdfFlight{done: make(chan struct{}), cancel: cancel, mode: mode}
		g.flights[key] = flight
		go g.run(flightCtx, key, flight, work)
	}
	if wait {
		flight.waiters++
	}
	return flight
}

// run executes work for flight and always publishes an outcome, even when work ends its goroutine with
// runtime.Goexit, so joined callers are never stranded.
func (g *pdfFlights) run(ctx context.Context, key string, flight *pdfFlight, work pdfFlightWork) {
	var value pdfFlightValue
	var err error
	completed := false
	defer func() {
		if !completed {
			err = errors.New("shared PDF work exited without a result")
		}
		g.finish(ctx, key, flight, value, err)
	}()
	value, err = runPDFWork(ctx, work)
	completed = true
}

// finish publishes the outcome of flight to its waiters and logs a failure exactly once when nobody waits for it.
func (g *pdfFlights) finish(ctx context.Context, key string, flight *pdfFlight, value pdfFlightValue, err error) {
	flight.cancel()
	g.mu.Lock()
	flight.value, flight.err, flight.finished = value, err, true
	if g.flights[key] == flight {
		delete(g.flights, key)
	}
	abandoned := flight.waiters == 0
	g.mu.Unlock()
	close(flight.done)

	// Waiting callers receive the error; only a failure nobody can receive any more is logged, and only here.
	// Read-only work stopped by its context after every caller left is expected: no caller wants it, and a write
	// flight that was waiting for it logs its own failure.
	expectedStop := flight.mode == pdfReadOnlyFlight &&
		(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
	if err != nil && abandoned && !expectedStop {
		gmw.GetLogger(ctx).Error("shared cv pdf work failed after every caller left",
			zap.String("flight", key), zap.Error(err))
	}
}

// runPDFWork runs work and converts a panic into an error that carries the panic value and the panicking stack.
func runPDFWork(ctx context.Context, work pdfFlightWork) (value pdfFlightValue, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			value, err = pdfFlightValue{}, errors.Errorf("shared PDF work panicked: %v", recovered)
		}
	}()
	return work(ctx)
}

// wait blocks until flight finishes or ctx ends and returns a caller-owned copy of the value or the shared error.
//
// A caller that leaves stops waiting only for itself; read-only work is canceled when its last waiter leaves.
// A caller whose context ends after the work finished still receives the outcome, so no result is lost.
func (g *pdfFlights) wait(ctx context.Context, key string, flight *pdfFlight) (pdfFlightValue, error) {
	select {
	case <-flight.done:
		return flight.result()
	case <-ctx.Done():
	}

	g.mu.Lock()
	if flight.finished {
		g.mu.Unlock()
		<-flight.done
		return flight.result()
	}
	flight.waiters--
	if flight.waiters == 0 && flight.mode == pdfReadOnlyFlight {
		flight.cancel()
		if g.flights[key] == flight {
			delete(g.flights, key)
		}
	}
	g.mu.Unlock()
	return pdfFlightValue{}, errors.Wrap(ctx.Err(), "wait for shared PDF work")
}

// result returns a caller-owned copy of the finished value or the wrapped shared error.
func (f *pdfFlight) result() (pdfFlightValue, error) {
	if f.err != nil {
		return pdfFlightValue{}, errors.Wrap(f.err, "shared PDF work")
	}
	// Joined callers must not alias one mutable slice.
	return pdfFlightValue{pdf: bytes.Clone(f.value.pdf), outcome: f.value.outcome}, nil
}
