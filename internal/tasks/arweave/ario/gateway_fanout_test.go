package ario

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestGatewayHandlerReturnsWhenTheCallerLeaves guards against a handler that
// never returns after its caller disconnects: once the request context ends,
// every gateway request fails (or answers too late), and the handler must still
// learn the outcome instead of waiting forever on a result nobody sends. The
// race is random per request, so the test repeats it.
func TestGatewayHandlerReturnsWhenTheCallerLeaves(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer func(*http.Request) (*http.Response, error)
	}{
		{name: "gateways fail", answer: func(request *http.Request) (*http.Response, error) {
			return nil, errors.WithStack(request.Context().Err())
		}},
		{name: "gateways answer late", answer: func(request *http.Request) (*http.Response, error) {
			return gatewayOK(request), nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started := make(chan struct{}, 2)
			blocking := gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
				started <- struct{}{}
				<-request.Context().Done()
				return tc.answer(request)
			})
			useGateways(t, &http.Client{Transport: blocking}, "https://gateway.example/", "https://other.example/")
			useGatewayEgress(t, blocking)

			for attempt := range 40 {
				ctx, cancel := context.WithCancel(context.Background())
				request := httptest.NewRequest(http.MethodGet, "/gateway/"+testFileKey, nil).WithContext(ctx)
				done := make(chan struct{})
				go func() {
					defer close(done)
					serveGateway(request)
				}()
				<-started
				<-started
				cancel()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					require.FailNowf(t, "handler hung", "attempt %d: the handler did not return after the caller left", attempt)
				}
			}
		})
	}
}

// stallingGatewayBody yields one byte, then blocks until release is closed.
type stallingGatewayBody struct {
	reading  chan struct{}
	release  chan struct{}
	once     sync.Once
	consumed bool
}

// Read returns one byte on the first call, signalling reading, and then waits
// for release before reporting the end of the body.
func (b *stallingGatewayBody) Read(p []byte) (int, error) {
	if !b.consumed && len(p) > 0 {
		b.consumed = true
		b.once.Do(func() { close(b.reading) })
		p[0] = 'c'
		return 1, nil
	}
	<-b.release
	return 0, io.EOF
}

// Close always succeeds.
func (b *stallingGatewayBody) Close() error { return nil }

// closeRecordingBody is an empty body that records whether it was closed.
type closeRecordingBody struct{ closed *atomic.Bool }

// Read reports the end of the body.
func (closeRecordingBody) Read([]byte) (int, error) { return 0, io.EOF }

// Close records the close.
func (b closeRecordingBody) Close() error {
	b.closed.Store(true)
	return nil
}

// TestGatewayHandlerReleasesLosingGatewaysDuringTheRelay guards against
// losing gateways holding their connections for the whole relay of the
// winner, which can last minutes: a late acceptable answer must be closed at
// once, and a gateway still working must be canceled as soon as another wins.
func TestGatewayHandlerReleasesLosingGatewaysDuringTheRelay(t *testing.T) {
	winner := &stallingGatewayBody{reading: make(chan struct{}), release: make(chan struct{})}
	var lateClosed, slowCanceled atomic.Bool
	transport := gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case "winner.example":
			response := gatewayOK(request)
			response.Body = winner
			return response, nil
		case "late.example":
			// Answers acceptably only after the winner was chosen, as an
			// answer already on its way when the request is canceled.
			select {
			case <-winner.reading:
			case <-request.Context().Done():
			}
			response := gatewayOK(request)
			response.Body = closeRecordingBody{closed: &lateClosed}
			return response, nil
		default:
			<-request.Context().Done()
			slowCanceled.Store(true)
			return nil, errors.WithStack(request.Context().Err())
		}
	})
	useGateways(t, &http.Client{Transport: transport},
		"https://winner.example/", "https://late.example/", "https://slow.example/")

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- serveGateway(httptest.NewRequest(http.MethodGet, "/gateway/"+testFileKey, nil))
	}()
	<-winner.reading
	require.Eventually(t, lateClosed.Load, 2*time.Second, 5*time.Millisecond,
		"a late acceptable answer must be closed while the winner relays")
	require.Eventually(t, slowCanceled.Load, 2*time.Second, 5*time.Millisecond,
		"a gateway still working must be canceled once another wins")
	close(winner.release)

	recorder := <-done
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "c", recorder.Body.String())
}
