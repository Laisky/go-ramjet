package ario

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// verifyNoFanOutLeak fails t if a goroutine started during the test is still
// running at its end. Gateway goroutines may outlive the handler briefly, so
// goleak retries before reporting.
func verifyNoFanOutLeak(t *testing.T) {
	t.Helper()
	ignore := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, ignore) })
}

// TestGatewayHandlerReturnsWhenTheCallerLeaves guards against a handler that
// never returns after its caller disconnects: once the request context ends,
// every gateway request fails (or answers too late), and the handler must still
// learn the outcome instead of waiting forever on a result nobody sends. The
// race is random per request, so the test repeats it.
func TestGatewayHandlerReturnsWhenTheCallerLeaves(t *testing.T) {
	verifyNoFanOutLeak(t)
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
	verifyNoFanOutLeak(t)
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

// countingCloseBody serves content and counts how often it is closed.
type countingCloseBody struct {
	*strings.Reader
	closes *atomic.Int32
	err    error
}

// Close counts the close and returns the configured error.
func (b countingCloseBody) Close() error {
	b.closes.Add(1)
	return b.err
}

// TestGatewayHandlerKeepsAWinnerThatFinishesWithEveryLoser guards the end of
// the fan-out: when the winner and every losing gateway finish at the same
// moment, the caller must still get the winner, never the losers' error, the
// winner's body must be closed exactly once, and no goroutine may be left
// waiting to report the outcome.
func TestGatewayHandlerKeepsAWinnerThatFinishesWithEveryLoser(t *testing.T) {
	verifyNoFanOutLeak(t)
	const gateways = 8
	targets := make([]string, 0, gateways)
	for i := range gateways {
		targets = append(targets, fmt.Sprintf("https://gateway%d.example/", i))
	}
	for attempt := range 50 {
		var entered sync.WaitGroup
		entered.Add(gateways)
		gate := make(chan struct{})
		go func() {
			entered.Wait()
			close(gate)
		}()
		var winnerCloses atomic.Int32
		useGateways(t, &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
			entered.Done()
			<-gate
			if request.URL.Host != "gateway0.example" {
				return gatewayMiss(request), nil
			}
			response := gatewayOK(request)
			response.Body = countingCloseBody{Reader: strings.NewReader("content"), closes: &winnerCloses}
			return response, nil
		})}, targets...)

		recorder := serveGateway(httptest.NewRequest(http.MethodGet, "/gateway/"+testFileKey, nil))
		require.Equal(t, http.StatusOK, recorder.Code, "attempt %d: %s", attempt, recorder.Body.String())
		require.Equal(t, "content", recorder.Body.String())
		require.EqualValues(t, 1, winnerCloses.Load(), "attempt %d: the winner body is closed exactly once", attempt)
	}
}

// observedLogger returns a debug-level request logger whose entries are recorded.
func observedLogger(t *testing.T) (glog.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	logger, err := glog.NewWithName("fanout", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	return logger, logs
}

// TestGatewayFanOutLogsEveryLosingOutcomeOnce guards against swallowed
// gateway errors: once a winner exists, every losing gateway's failure,
// including a failure to close a late answer, is logged once at debug level
// with its target; without a winner the first failure is returned and every
// other one is logged once.
func TestGatewayFanOutLogsEveryLosingOutcomeOnce(t *testing.T) {
	verifyNoFanOutLeak(t)
	errClose := errors.New("synthetic close failure")
	t.Run("winner", func(t *testing.T) {
		running := goleak.IgnoreCurrent()
		logger, logs := observedLogger(t)
		var winnerCloses, lateCloses atomic.Int32
		claimed := make(chan struct{})
		var claimOnce sync.Once
		client := &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
			switch request.URL.Host {
			case "winner.example":
				response := gatewayOK(request)
				response.Body = countingCloseBody{Reader: strings.NewReader("content"), closes: &winnerCloses}
				return response, nil
			case "late.example":
				<-request.Context().Done()
				response := gatewayOK(request)
				response.Body = countingCloseBody{Reader: strings.NewReader("late"), closes: &lateCloses, err: errClose}
				return response, nil
			default:
				<-request.Context().Done()
				claimOnce.Do(func() { close(claimed) })
				return gatewayMiss(request), nil
			}
		})}
		fanOut := gatewayFanOut{client: client, method: http.MethodGet, header: http.Header{}, logger: logger}
		response, err := fanOut.first(t.Context(), []string{
			"https://winner.example/" + testFileKey,
			"https://late.example/" + testFileKey,
			"https://miss.example/" + testFileKey,
		})
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		<-claimed

		require.Eventually(t, func() bool {
			return logs.FilterMessage("losing gateway outcome").Len() == 2
		}, 2*time.Second, 5*time.Millisecond, "each losing outcome is logged")
		// Every gateway goroutine has ended, so no later entry can follow.
		goleak.VerifyNone(t, running)
		entries := logs.FilterMessage("losing gateway outcome").All()
		require.Len(t, entries, 2, "each losing outcome is logged exactly once")
		var messages []string
		for _, entry := range entries {
			require.Equal(t, zapcore.DebugLevel, entry.Level)
			messages = append(messages, fmt.Sprint(entry.ContextMap()["error"]))
		}
		require.Contains(t, strings.Join(messages, "\n"), errClose.Error())
		require.Contains(t, strings.Join(messages, "\n"), "miss.example")
		require.EqualValues(t, 1, lateCloses.Load())
	})
	t.Run("no winner", func(t *testing.T) {
		logger, logs := observedLogger(t)
		client := &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
			return gatewayMiss(request), nil
		})}
		fanOut := gatewayFanOut{client: client, method: http.MethodGet, header: http.Header{}, logger: logger}
		response, err := fanOut.first(t.Context(), []string{
			"https://one.example/" + testFileKey,
			"https://two.example/" + testFileKey,
			"https://three.example/" + testFileKey,
		})
		require.Error(t, err)
		require.Nil(t, response)
		entries := logs.FilterMessage("losing gateway outcome").All()
		require.Len(t, entries, 2, "every failure but the returned one is logged once")
		for _, entry := range entries {
			require.NotContains(t, err.Error(), fmt.Sprint(entry.ContextMap()["error"]),
				"a logged failure is not also returned")
		}
	})
}
