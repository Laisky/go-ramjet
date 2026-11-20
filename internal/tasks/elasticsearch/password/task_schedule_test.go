package password

import (
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRunTaskSkipsOverlappingRotation verifies that a tick during an active run
// makes no request and that a later tick can run even after a failed response.
func TestRunTaskSkipsOverlappingRotation(t *testing.T) {
	configureTask(t, false, "https://es.example.invalid/rotate")
	logs := captureLogs(t)

	var calls atomic.Int32
	firstStarted := make(chan struct{})
	release := make(chan struct{})
	releaseFirst := sync.OnceFunc(func() { close(release) })
	useTransport(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		status := http.StatusOK
		if calls.Add(1) == 1 {
			close(firstStarted)
			<-release
			status = http.StatusServiceUnavailable
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader("fixture response")),
		}, nil
	}))

	firstDone := make(chan struct{})
	secondDone := make(chan struct{})
	secondStarted := false
	t.Cleanup(func() {
		releaseFirst()
		<-firstDone
		if secondStarted {
			<-secondDone
		}
	})
	go func() {
		defer close(firstDone)
		runTask()
	}()
	select {
	case <-firstStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("first scheduled run did not reach the fake transport")
	}

	secondStarted = true
	go func() {
		defer close(secondDone)
		runTask()
	}()
	select {
	case <-secondDone:
	case <-time.After(5 * time.Second):
		t.Fatal("overlapping run did not return promptly")
	}
	require.EqualValues(t, 1, calls.Load())

	releaseFirst()
	<-firstDone
	require.Len(t, logs.FilterMessage("failed to change password on some apis").All(), 1)
	runTask()
	require.EqualValues(t, 2, calls.Load())
}

// TestBindPasswordTaskScheduling verifies when the scheduler is called and the
// interval it receives for legacy settings, defaults, and debug mode.
func TestBindPasswordTaskScheduling(t *testing.T) {
	api := []string{"https://es.example.invalid/rotate"}
	for _, tc := range []struct {
		name     string
		apis     []string
		seconds  int64
		debug    bool
		interval time.Duration
	}{
		{name: "unconfigured", seconds: 10},
		{name: "unconfigured in debug mode", debug: true},
		{name: "configured seconds", apis: api, seconds: 90, interval: 90 * time.Second},
		{name: "missing or zero uses safe default", apis: api, interval: time.Minute},
		{name: "negative uses safe default", apis: api, seconds: -1, interval: time.Minute},
		{name: "overflow uses safe default", apis: api, seconds: math.MaxInt64, interval: time.Minute},
		{name: "debug still uses five seconds", apis: api, seconds: 90, debug: true, interval: 5 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configureTask(t, false, tc.apis...)
			setConfig(t, "tasks.elasticsearch-v2.password.interval", tc.seconds)
			setConfig(t, "debug", tc.debug)

			calls := 0
			bindPasswordTask(func(interval time.Duration, rotate func()) {
				calls++
				require.Equal(t, tc.interval, interval)
				require.NotNil(t, rotate)
			})
			if tc.interval == 0 {
				require.Zero(t, calls)
				return
			}
			require.Equal(t, 1, calls)
		})
	}
}
