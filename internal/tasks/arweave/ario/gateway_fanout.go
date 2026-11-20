package ario

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
)

// gatewayFanOut sends one caller request to every gateway target. It holds
// only copies of the caller's request, so its goroutines never read the gin
// context, which is recycled once the handler returns.
type gatewayFanOut struct {
	client *http.Client
	method string
	header http.Header
	body   []byte
	logger glog.Logger
}

// gatewayOutcome is the single result of a fan-out: the winning response, or
// the error reported when no gateway answered acceptably.
type gatewayOutcome struct {
	resp *http.Response
	err  error
}

// first sends the request to every target and returns the first acceptable
// response (see fetch).
//
// As soon as one response wins, every other request is canceled and a later
// acceptable answer is closed, so losing gateways hold no connection while the
// winner is relayed. The winner's request lives until ctx ends, so the caller
// cancels ctx after it has relayed and closed the body. Without a winner it
// returns the first gateway failure once every request has ended.
//
// Exactly one outcome is ever sent: the winner, by the goroutine that claims
// the win, or the failure, by the waiter once every request has ended without
// a winner. So first never waits on a result that will not come, even after
// ctx ends, and never mistakes a winner for a failure. Every failure it does
// not return, including a losing answer that fails to close, is logged once
// at debug level.
func (f gatewayFanOut) first(ctx context.Context, targets []string) (*http.Response, error) {
	contexts := make([]context.Context, len(targets))
	cancels := make([]context.CancelFunc, len(targets))
	for i := range targets {
		contexts[i], cancels[i] = context.WithCancel(ctx)
	}

	outcome := make(chan gatewayOutcome, 1)
	var claimed atomic.Bool
	var mu sync.Mutex
	var failures []error
	var pool sync.WaitGroup
	for i, target := range targets {
		pool.Go(func() {
			resp, err := f.fetch(contexts[i], target)
			switch {
			case err != nil:
			case claimed.CompareAndSwap(false, true):
				for j, cancel := range cancels {
					if j != i {
						cancel()
					}
				}
				outcome <- gatewayOutcome{resp: resp}
				return
			default:
				err = errors.Wrap(resp.Body.Close(), "close losing answer")
			}
			if err != nil {
				mu.Lock()
				failures = append(failures, errors.Wrapf(err, "gateway %q", target))
				mu.Unlock()
			}
		})
	}

	go func() {
		pool.Wait()
		// Every gateway goroutine has ended, so failures is complete and the
		// claim is final.
		if claimed.Load() {
			f.logLosers(failures)
			return
		}
		if len(failures) == 0 {
			outcome <- gatewayOutcome{err: errors.New("no gateway answered acceptably")}
			return
		}
		f.logLosers(failures[1:])
		outcome <- gatewayOutcome{err: errors.Wrap(failures[0], "no gateway answered acceptably")}
	}()

	result := <-outcome
	return result.resp, result.err
}

// logLosers logs each failure that first does not return once, at debug
// level. The failures name public gateway URLs and carry no caller data.
func (f gatewayFanOut) logLosers(failures []error) {
	for _, err := range failures {
		f.logger.Debug("losing gateway outcome", zap.Error(err))
	}
}

// fetch sends the request to target and returns the response when it is
// acceptable: a 2xx or 3xx answer whose final hop is the operator cache or
// carries an ar.io hop header. Any other response is closed and reported as an
// error.
func (f gatewayFanOut) fetch(ctx context.Context, target string) (*http.Response, error) {
	f.logger.Debug("fetching file", zap.String("target_url", target))

	var reqBody io.Reader
	if len(f.body) > 0 {
		reqBody = bytes.NewReader(f.body)
	}
	req, err := http.NewRequestWithContext(ctx, f.method, target, reqBody)
	if err != nil {
		return nil, errors.Wrapf(err, "new request %q", target)
	}
	req.Header = f.header.Clone()
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "do request %q", target)
	}

	// Trust is decided on the final hop: a cache URL that redirects elsewhere
	// must not lend its trust to the destination.
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest &&
		(isOperatorCache(finalGatewayURL(resp, target)) || resp.Header.Get("X-Ar-Io-Hops") != "") {
		return resp, nil
	}
	return nil, errors.Join(
		errors.Errorf("request %q, got %d", target, resp.StatusCode),
		errors.Wrap(resp.Body.Close(), "close rejected gateway response"),
	)
}
