package tasks

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// fakeTimeoutErr is a net.Error whose Timeout() reports true, mirroring the
// "read tcp ...->...:6379: i/o timeout" surfaced by a blocking BLPOP during a
// Redis restart.
type fakeTimeoutErr struct{}

func (fakeTimeoutErr) Error() string   { return "read tcp 1.2.3.4:50320->5.6.7.8:6379: i/o timeout" }
func (fakeTimeoutErr) Timeout() bool   { return true }
func (fakeTimeoutErr) Temporary() bool { return false }

var _ net.Error = fakeTimeoutErr{}

// fakeRedisReplyErr implements the redis.Error interface, mirroring the typed
// proto.RedisError values go-redis returns for server error replies (which
// cannot be constructed outside the go-redis module). redis.HasErrorPrefix and
// the typed redis.Is*Error helpers only recognise values that implement this
// interface, which is exactly what guards against false positives on plain
// application errors.
type fakeRedisReplyErr string

func (e fakeRedisReplyErr) Error() string { return string(e) }
func (e fakeRedisReplyErr) RedisError()   {}

var _ redis.Error = fakeRedisReplyErr("")

func withInjectedPop(t *testing.T, fn func(context.Context) (*rlibs.HTMLCrawlerTask, error)) {
	t.Helper()
	original := popHTMLCrawlerTask
	t.Cleanup(func() { popHTMLCrawlerTask = original })
	popHTMLCrawlerTask = fn
}

// Test_runDynamicWebCrawler_transientRedisErrorIsNotFatal reproduces the
// production log storm: during a Redis restart, popHTMLCrawlerTask returns a
// transient "LOADING Redis is loading the dataset in memory" error, and the
// worker treats it as a fatal "get html crawler task" error which the loop
// logs at ERROR with a full stack trace and retries on a flat 1s sleep.
//
// The correct contract: a transient, self-healing infrastructure error must NOT
// be wrapped onto the fatal ERROR path.
func Test_runDynamicWebCrawler_transientRedisErrorIsNotFatal(t *testing.T) {
	withInjectedPop(t, func(context.Context) (*rlibs.HTMLCrawlerTask, error) {
		return nil, fakeRedisReplyErr("LOADING Redis is loading the dataset in memory")
	})

	err := runDynamicWebCrawler()
	if err != nil {
		require.NotContains(t, err.Error(), "get html crawler task",
			"transient Redis LOADING error must not be routed onto the fatal ERROR path")
	}
}

func Test_isTransientRedisErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"redis_nil_empty_queue", redis.Nil, false},
		{"context_canceled_shutdown", context.Canceled, false},
		{"context_deadline_poll_timeout", context.DeadlineExceeded, false},
		{"loading", fakeRedisReplyErr("LOADING Redis is loading the dataset in memory"), true},
		{"readonly", fakeRedisReplyErr("READONLY You can't write against a read only replica."), true},
		{"clusterdown", fakeRedisReplyErr("CLUSTERDOWN The cluster is down"), true},
		{"masterdown", fakeRedisReplyErr("MASTERDOWN Link with MASTER is down"), true},
		{"loading_wrapped", errors.Wrap(fakeRedisReplyErr("LOADING Redis is loading the dataset in memory"), "set task result"), true},
		{"eof", io.EOF, true},
		{"unexpected_eof", io.ErrUnexpectedEOF, true},
		{"net_timeout", fakeTimeoutErr{}, true},
		{"dial_refused", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}, true},
		{"fatal_wrongtype", errors.New("WRONGTYPE Operation against a key holding the wrong kind of value"), false},
		{"fatal_parse", errors.New("parse task: invalid character"), false},
		// Guard against the strings.Contains false-positive of the previous
		// implementation: a plain (non-redis.Error) application error that merely
		// mentions a server-reply word mid-message must stay FATAL, never be
		// silently swallowed as transient. (The old Contains-based check matched
		// these; the prefix/interface-based check correctly rejects them.)
		{"fatal_word_loading_midstring", errors.New("render failed: asset was LOADING when the worker reset"), false},
		{"fatal_word_readonly_midstring", errors.New("config rejected: field READONLY is not allowed here"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isTransientRedisErr(tc.err))
		})
	}
}

func Test_runDynamicWebCrawler_swallowsTransientPopErrors(t *testing.T) {
	transient := []struct {
		name string
		err  error
	}{
		{"loading", fakeRedisReplyErr("LOADING Redis is loading the dataset in memory")},
		{"eof", io.EOF},
		{"io_timeout", fakeTimeoutErr{}},
		{"connection_refused", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}},
	}

	for _, tc := range transient {
		t.Run(tc.name, func(t *testing.T) {
			withInjectedPop(t, func(context.Context) (*rlibs.HTMLCrawlerTask, error) {
				return nil, tc.err
			})

			err := runDynamicWebCrawler()
			require.Error(t, err)
			require.True(t, isTransientRedisErr(err),
				"transient pop error must remain classifiable as transient by the worker loop")
			require.NotContains(t, err.Error(), "get html crawler task",
				"transient pop error must not be wrapped onto the fatal ERROR path")
		})
	}
}

func Test_runDynamicWebCrawler_returnsNilForEmptyQueue(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"redis_nil", redis.Nil},
		{"deadline_exceeded", context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withInjectedPop(t, func(context.Context) (*rlibs.HTMLCrawlerTask, error) {
				return nil, tc.err
			})
			require.NoError(t, runDynamicWebCrawler())
		})
	}
}

func Test_runDynamicWebCrawler_surfacesFatalPopError(t *testing.T) {
	withInjectedPop(t, func(context.Context) (*rlibs.HTMLCrawlerTask, error) {
		return nil, errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")
	})

	err := runDynamicWebCrawler()
	require.Error(t, err)
	require.False(t, isTransientRedisErr(err),
		"genuinely unexpected errors must stay on the fatal ERROR path")
	require.Contains(t, err.Error(), "get html crawler task")
}

func Test_crawlerPollBackoffWithRand_boundsAndGrowth(t *testing.T) {
	zero := func(int64) int64 { return 0 }
	maxJitter := func(n int64) int64 { return n - 1 }

	var prevCeil time.Duration
	for attempt := 0; attempt <= 20; attempt++ {
		lo := crawlerPollBackoffWithRand(attempt, zero)
		hi := crawlerPollBackoffWithRand(attempt, maxJitter)

		require.GreaterOrEqual(t, lo, crawlerPollBaseBackoff, "attempt %d must respect the floor", attempt)
		require.LessOrEqual(t, hi, crawlerPollMaxBackoff, "attempt %d must respect the cap", attempt)
		require.LessOrEqual(t, lo, hi, "attempt %d: lower bound must not exceed upper bound", attempt)
		if attempt > 0 {
			require.GreaterOrEqual(t, hi, prevCeil, "ceiling must grow monotonically")
		}
		prevCeil = hi
	}

	// Deep into the backoff the ceiling is pinned at the cap.
	require.Equal(t, crawlerPollMaxBackoff, crawlerPollBackoffWithRand(20, maxJitter))
	// attempt 0 is deterministic at the floor (jitter span is zero).
	require.Equal(t, crawlerPollBaseBackoff, crawlerPollBackoffWithRand(0, maxJitter))
	// negative attempts are clamped to the floor.
	require.Equal(t, crawlerPollBaseBackoff, crawlerPollBackoffWithRand(-3, maxJitter))
}

func Test_crawlerPollBackoff_isBounded(t *testing.T) {
	for attempt := 0; attempt <= 12; attempt++ {
		d := crawlerPollBackoff(attempt)
		require.GreaterOrEqual(t, d, crawlerPollBaseBackoff)
		require.LessOrEqual(t, d, crawlerPollMaxBackoff)
	}
}
