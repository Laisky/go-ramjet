package ario

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"golang.org/x/sync/errgroup"
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

// first sends the request to every target and returns the first acceptable
// response (see fetch).
//
// As soon as one response wins, every other request is canceled and a later
// acceptable answer is closed, so losing gateways hold no connection while the
// winner is relayed. The winner's request lives until ctx ends, so the caller
// cancels ctx after it has relayed and closed the body. Without a winner it
// returns the first gateway error once every request has ended; it never waits
// on a result that will not come, even after ctx ends.
func (f gatewayFanOut) first(ctx context.Context, targets []string) (*http.Response, error) {
	contexts := make([]context.Context, len(targets))
	cancels := make([]context.CancelFunc, len(targets))
	for i := range targets {
		contexts[i], cancels[i] = context.WithCancel(ctx)
	}

	won := make(chan *http.Response, 1)
	var claimed atomic.Bool
	var pool errgroup.Group
	for i, target := range targets {
		pool.Go(func() error {
			resp, err := f.fetch(contexts[i], target)
			if err != nil {
				return errors.Wrapf(err, "gateway %q", target)
			}
			if !claimed.CompareAndSwap(false, true) {
				return errors.Wrapf(resp.Body.Close(), "close losing gateway %q", target)
			}
			for j, cancel := range cancels {
				if j != i {
					cancel()
				}
			}
			won <- resp
			return nil
		})
	}

	finished := make(chan error, 1)
	go func() { finished <- pool.Wait() }()

	select {
	case resp := <-won:
		return resp, nil
	case err := <-finished:
		// A winner is sent before its goroutine ends, so it is visible here.
		select {
		case resp := <-won:
			return resp, nil
		default:
		}
		if err != nil {
			return nil, errors.Wrap(err, "no gateway answered acceptably")
		}
		return nil, errors.New("no gateway answered acceptably")
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
