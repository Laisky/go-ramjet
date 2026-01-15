package twitter

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// resetMongoDaoForTest clears cached dao state for tests.
func resetMongoDaoForTest() {
	twitterDaoMu.Lock()
	defer twitterDaoMu.Unlock()

	twitterDao = nil
	twitterDaoCfg = mongoDialConfig{}
	newMongoDao = NewDao
	pingMongoDao = defaultPingMongoDao
}

// Test_getMongoDao_CachesByConfig ensures the dao is cached per connection settings.
func Test_getMongoDao_CachesByConfig(t *testing.T) {
	t.Cleanup(resetMongoDaoForTest)
	pingMongoDao = func(ctx context.Context, dao *mongoDao) error {
		return nil
	}

	var calls int32
	newMongoDao = func(ctx context.Context, addr, dbName, user, pwd string) (*mongoDao, error) {
		atomic.AddInt32(&calls, 1)
		return &mongoDao{dbName: dbName, tweetsColName: "tweets"}, nil
	}

	ctx := context.Background()
	first, err := getMongoDao(ctx, "addr-1", "db-1", "user-1", "pwd-1")
	require.NoError(t, err)

	second, err := getMongoDao(ctx, "addr-1", "db-1", "user-1", "pwd-1")
	require.NoError(t, err)

	require.Same(t, first, second)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))

	third, err := getMongoDao(ctx, "addr-2", "db-2", "user-1", "pwd-1")
	require.NoError(t, err)
	require.NotSame(t, first, third)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

// Test_getMongoDao_RetryOnFailure ensures failed creation does not poison the cache.
func Test_getMongoDao_RetryOnFailure(t *testing.T) {
	t.Cleanup(resetMongoDaoForTest)
	pingMongoDao = func(ctx context.Context, dao *mongoDao) error {
		return nil
	}

	var calls int32
	newMongoDao = func(ctx context.Context, addr, dbName, user, pwd string) (*mongoDao, error) {
		cnt := atomic.AddInt32(&calls, 1)
		if cnt == 1 {
			return nil, errors.New("dial failed")
		}
		return &mongoDao{dbName: dbName, tweetsColName: "tweets"}, nil
	}

	ctx := context.Background()
	_, err := getMongoDao(ctx, "addr-1", "db-1", "user-1", "pwd-1")
	require.Error(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))

	dao, err := getMongoDao(ctx, "addr-1", "db-1", "user-1", "pwd-1")
	require.NoError(t, err)
	require.NotNil(t, dao)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

// Test_getMongoDao_ReconnectOnPingFailure ensures cache is refreshed when ping fails.
func Test_getMongoDao_ReconnectOnPingFailure(t *testing.T) {
	t.Cleanup(resetMongoDaoForTest)

	var calls int32
	newMongoDao = func(ctx context.Context, addr, dbName, user, pwd string) (*mongoDao, error) {
		atomic.AddInt32(&calls, 1)
		return &mongoDao{dbName: dbName, tweetsColName: "tweets"}, nil
	}

	var pings int32
	pingMongoDao = func(ctx context.Context, dao *mongoDao) error {
		cnt := atomic.AddInt32(&pings, 1)
		if cnt == 1 {
			return errors.New("ping failed")
		}
		return nil
	}

	ctx := context.Background()
	first, err := getMongoDao(ctx, "addr-1", "db-1", "user-1", "pwd-1")
	require.NoError(t, err)

	second, err := getMongoDao(ctx, "addr-1", "db-1", "user-1", "pwd-1")
	require.NoError(t, err)
	require.NotSame(t, first, second)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls))
}
