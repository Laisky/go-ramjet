package s3

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// resetSharedClient clears the process-wide client for one test and restores the
// previous state on cleanup.
func resetSharedClient(t *testing.T) {
	t.Helper()
	rw.Lock()
	origCli := cli
	cli = nil
	rw.Unlock()
	once = sync.Once{}
	errInit = nil
	t.Cleanup(func() {
		rw.Lock()
		defer rw.Unlock()
		cli, once, errInit = origCli, sync.Once{}, nil
		if origCli != nil {
			once.Do(func() {})
		}
	})
}

// TestGetCliKeepsReportingFailedInitialization verifies that when the shared client
// cannot be created, every later call reports that failure instead of returning a
// nil client with a nil error, which callers would dereference and panic on.
func TestGetCliKeepsReportingFailedInitialization(t *testing.T) {
	resetSharedClient(t)

	for range 2 {
		got, err := GetCli("", "synthetic-access-id", "synthetic-access-key")
		require.Error(t, err)
		require.Nil(t, got)
	}
}

// TestGetCliCreationIsOfflineAndDeterministic pins why GetCli may keep a creation
// failure instead of retrying: creating the client does no network I/O (an
// unroutable endpoint still yields a client at once), so a failure comes only from
// the arguments, and retrying the same arguments can never succeed.
func TestGetCliCreationIsOfflineAndDeterministic(t *testing.T) {
	resetSharedClient(t)

	start := time.Now()
	got, err := GetCli("192.0.2.1:9", "synthetic-access-id", "synthetic-access-key")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Less(t, time.Since(start), time.Second, "client creation must not dial the endpoint")
}
