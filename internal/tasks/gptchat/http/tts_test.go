package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/Microsoft/cognitive-services-speech-sdk-go/common"
	"github.com/Microsoft/cognitive-services-speech-sdk-go/speech"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// asyncOutcomeTestGuard bounds how long a test waits before declaring a stuck sender.
const asyncOutcomeTestGuard = 5 * time.Second

// startSDKStyleSender mimics the Azure Speech SDK async calls, which create an
// unbuffered channel and send exactly one outcome from a goroutine after delay.
// The returned channel is closed once that send has completed.
func startSDKStyleSender(delay time.Duration, value int) (task chan int, sent chan struct{}) {
	task = make(chan int)
	sent = make(chan struct{})
	go func() {
		defer close(sent)
		time.Sleep(delay)
		task <- value
	}()
	return task, sent
}

// TestAwaitAsyncOutcomeReturnsValue verifies an in-time outcome is returned to the
// caller and is not released by the helper.
func TestAwaitAsyncOutcomeReturnsValue(t *testing.T) {
	t.Parallel()
	task, sent := startSDKStyleSender(0, 42)

	released := make(chan int, 1)
	got, err := awaitAsyncOutcome(context.Background(), task, asyncOutcomeTestGuard, func(v int) { released <- v })
	require.NoError(t, err)
	require.Equal(t, 42, got)

	<-sent
	select {
	case v := <-released:
		require.FailNowf(t, "unexpected release", "in-time outcome %d must stay owned by the caller", v)
	default:
	}
}

// TestAwaitAsyncOutcomeTimeoutReleasesLateOutcome verifies that after a timeout the
// SDK sender is not stuck on its unbuffered channel and the late outcome is released.
func TestAwaitAsyncOutcomeTimeoutReleasesLateOutcome(t *testing.T) {
	t.Parallel()
	task, sent := startSDKStyleSender(100*time.Millisecond, 7)

	released := make(chan int, 1)
	_, err := awaitAsyncOutcome(context.Background(), task, 10*time.Millisecond, func(v int) { released <- v })
	require.Error(t, err)
	require.Contains(t, err.Error(), "timeout")

	select {
	case <-sent:
	case <-time.After(asyncOutcomeTestGuard):
		require.FailNow(t, "SDK sender is blocked forever on its unbuffered outcome channel")
	}
	select {
	case v := <-released:
		require.Equal(t, 7, v)
	case <-time.After(asyncOutcomeTestGuard):
		require.FailNow(t, "late outcome was never released")
	}
}

// TestAwaitAsyncOutcomeLeavesNoGoroutine verifies that the goroutine which keeps
// receiving an abandoned outcome exits once the late value has been released, so
// abandoned syntheses do not accumulate goroutines when the SDK does answer.
func TestAwaitAsyncOutcomeLeavesNoGoroutine(t *testing.T) {
	before := liveGoroutines("awaitAsyncOutcome[")
	task := make(chan int)
	released := make(chan int, 1)
	_, err := awaitAsyncOutcome(context.Background(), task, time.Millisecond, func(v int) { released <- v })
	require.Error(t, err)
	require.Greater(t, liveGoroutines("awaitAsyncOutcome["), before, "nothing receives the abandoned outcome")

	task <- 11
	require.Equal(t, 11, <-released)
	require.Eventually(t, func() bool { return liveGoroutines("awaitAsyncOutcome[") <= before },
		5*time.Second, 10*time.Millisecond, "the release goroutine outlived the late outcome")
}

// TestAwaitAsyncOutcomeContextCanceled verifies that a canceled request stops the
// wait promptly and still releases the outcome that arrives afterwards.
func TestAwaitAsyncOutcomeContextCanceled(t *testing.T) {
	t.Parallel()
	task, sent := startSDKStyleSender(200*time.Millisecond, 9)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	released := make(chan int, 1)
	start := time.Now()
	_, err := awaitAsyncOutcome(ctx, task, asyncOutcomeTestGuard, func(v int) { released <- v })
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(start), asyncOutcomeTestGuard/2, "wait must not outlive the request")

	select {
	case <-sent:
	case <-time.After(asyncOutcomeTestGuard):
		require.FailNow(t, "SDK sender is blocked forever on its unbuffered outcome channel")
	}
	select {
	case v := <-released:
		require.Equal(t, 9, v)
	case <-time.After(asyncOutcomeTestGuard):
		require.FailNow(t, "late outcome was never released")
	}
}

// fakeTTSSynthesizer is a ttsSynthesizer whose outcome channel is driven by the test.
// Like the SDK, its async calls return unbuffered channels fed by goroutines.
type fakeTTSSynthesizer struct {
	task      chan speech.SpeechSynthesisOutcome
	stopCalls atomic.Int32
	onStart   func()
}

// StartSpeakingSsmlAsync runs the optional onStart hook and returns the test-driven
// outcome channel.
func (f *fakeTTSSynthesizer) StartSpeakingSsmlAsync(string) chan speech.SpeechSynthesisOutcome {
	if f.onStart != nil {
		f.onStart()
	}
	return f.task
}

// StopSpeakingAsync records the call and delivers a nil result on an unbuffered channel.
func (f *fakeTTSSynthesizer) StopSpeakingAsync() chan error {
	f.stopCalls.Add(1)
	ch := make(chan error)
	go func() { ch <- nil }()
	return ch
}

// SynthesisStarted ignores the handler.
func (f *fakeTTSSynthesizer) SynthesisStarted(speech.SpeechSynthesisEventHandler) {}

// Synthesizing ignores the handler.
func (f *fakeTTSSynthesizer) Synthesizing(speech.SpeechSynthesisEventHandler) {}

// SynthesisCompleted ignores the handler.
func (f *fakeTTSSynthesizer) SynthesisCompleted(speech.SpeechSynthesisEventHandler) {}

// SynthesisCanceled ignores the handler.
func (f *fakeTTSSynthesizer) SynthesisCanceled(speech.SpeechSynthesisEventHandler) {}

// ttsHandlerHarness runs the real TTSHanler against a fake synthesizer and counts how
// often the synthesizer's native resources are released.
type ttsHandlerHarness struct {
	synth    *fakeTTSSynthesizer
	releases atomic.Int32
	logs     *observer.ObservedLogs
	gctx     *gin.Context
	rec      *httptest.ResponseRecorder
}

// newTTSHandlerHarness isolates configuration, HTTP and the synthesizer factory so
// TTSHanler runs without Azure: billing succeeds and SSML generation falls back to
// the default template. parent becomes the request context.
func newTTSHandlerHarness(t *testing.T, parent context.Context, timeout time.Duration) *ttsHandlerHarness {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/token/consume" {
			_, _ = w.Write([]byte("{}"))
			return
		}
		http.Error(w, "llm disabled in tts test", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	requestLogger, requestEntries, _ := embeddingTestSetup(t, server)
	config.Config.ExternalBillingAPI = server.URL
	config.Config.Azure.TTSKey = "synthetic-tts-key"
	config.Config.Azure.TTSRegion = "synthetic-region"

	h := &ttsHandlerHarness{
		synth: &fakeTTSSynthesizer{task: make(chan speech.SpeechSynthesisOutcome)},
		logs:  requestEntries,
	}
	origFactory, origTimeout := newTTSSynthesizer, ttsSynthesisTimeout
	t.Cleanup(func() { newTTSSynthesizer, ttsSynthesisTimeout = origFactory, origTimeout })
	ttsSynthesisTimeout = timeout
	newTTSSynthesizer = func() (ttsSynthesizer, func(), error) {
		return h.synth, func() { h.releases.Add(1) }, nil
	}

	gin.SetMode(gin.TestMode)
	h.rec = httptest.NewRecorder()
	h.gctx, _ = gin.CreateTestContext(h.rec)
	h.gctx.Request = httptest.NewRequest(http.MethodGet, "/gptchat/audio/tts?apikey=synthetic&text=hello", nil).WithContext(parent)
	h.gctx.Set(ctxKeyUser, &config.UserConfig{
		Token: "sk-synthetic-tts", OpenaiToken: "sk-synthetic-tts", UserName: "tts-tester",
		APIBase: server.URL, AllowedModels: []string{"*"}, NoLimitExpensiveModels: true,
	})
	gmw.SetLogger(h.gctx, requestLogger)
	return h
}

// run calls TTSHanler and fails the test if it does not return promptly.
func (h *ttsHandlerHarness) run(t *testing.T) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		TTSHanler(h.gctx)
	}()
	select {
	case <-done:
	case <-time.After(asyncOutcomeTestGuard):
		require.FailNow(t, "TTSHanler did not return")
	}
}

// deliverLate sends a late outcome as the SDK goroutine would and fails the test if
// nobody receives it.
func (h *ttsHandlerHarness) deliverLate(t *testing.T, outcome speech.SpeechSynthesisOutcome) {
	t.Helper()
	select {
	case h.synth.task <- outcome:
	case <-time.After(asyncOutcomeTestGuard):
		require.FailNow(t, "late synthesis outcome was never received")
	}
}

// TestTTSHanlerAbandonedSynthesisKeepsResourcesUntilLateOutcome verifies the handler
// path on timeout: it stops the synthesis, does not free the native synthesizer on
// return while the SDK may still use it, and frees it exactly once after the late
// outcome arrives, logging that outcome's error.
func TestTTSHanlerAbandonedSynthesisKeepsResourcesUntilLateOutcome(t *testing.T) {
	h := newTTSHandlerHarness(t, context.Background(), 20*time.Millisecond)

	h.run(t)
	require.GreaterOrEqual(t, h.rec.Code, http.StatusBadRequest)
	require.Zero(t, h.releases.Load(), "synthesizer released while the abandoned native call may still use it")
	require.Eventually(t, func() bool { return h.synth.stopCalls.Load() == 1 },
		asyncOutcomeTestGuard, 5*time.Millisecond, "abandoned synthesis was not stopped")

	h.deliverLate(t, speech.SpeechSynthesisOutcome{OperationOutcome: common.OperationOutcome{Error: errors.New("late native failure")}})
	require.Eventually(t, func() bool { return h.releases.Load() == 1 },
		asyncOutcomeTestGuard, 5*time.Millisecond, "synthesizer was not released after the late outcome")
	require.Eventually(t, func() bool {
		return h.logs.FilterMessage("released abandoned speech synthesis").Len() == 1
	}, asyncOutcomeTestGuard, 5*time.Millisecond, "late outcome was not logged")
	entry := h.logs.FilterMessage("released abandoned speech synthesis").All()[0]
	require.Equal(t, "late native failure", entry.ContextMap()["outcome_error"])
	require.Equal(t, int32(1), h.releases.Load())
}

// TestTTSHanlerCanceledRequestStopsWaiting verifies a request canceled while waiting
// for synthesis returns without waiting for the timeout and hands cleanup to the
// late outcome.
func TestTTSHanlerCanceledRequestStopsWaiting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := newTTSHandlerHarness(t, ctx, time.Hour)
	h.synth.onStart = cancel // the client disconnects once synthesis has started

	h.run(t)
	require.Zero(t, h.releases.Load())
	require.Eventually(t, func() bool { return h.synth.stopCalls.Load() == 1 },
		asyncOutcomeTestGuard, 5*time.Millisecond, "abandoned synthesis was not stopped")

	h.deliverLate(t, speech.SpeechSynthesisOutcome{})
	require.Eventually(t, func() bool { return h.releases.Load() == 1 },
		asyncOutcomeTestGuard, 5*time.Millisecond)
}

// TestTTSHanlerInTimeOutcomeReleasesOnReturn verifies that when the outcome arrives
// in time the handler itself frees the synthesizer once, without stopping it.
func TestTTSHanlerInTimeOutcomeReleasesOnReturn(t *testing.T) {
	h := newTTSHandlerHarness(t, context.Background(), asyncOutcomeTestGuard)
	go func() {
		h.synth.task <- speech.SpeechSynthesisOutcome{OperationOutcome: common.OperationOutcome{Error: errors.New("synthesis rejected")}}
	}()

	h.run(t)
	require.GreaterOrEqual(t, h.rec.Code, http.StatusBadRequest)
	require.Contains(t, h.rec.Body.String(), "synthesis rejected")
	require.Equal(t, int32(1), h.releases.Load())
	require.Zero(t, h.synth.stopCalls.Load())
}
