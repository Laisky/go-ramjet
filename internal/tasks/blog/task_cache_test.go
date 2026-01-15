package blog

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	"github.com/stretchr/testify/require"
)

// resetBlogDBForTest clears cached blog db state for tests.
func resetBlogDBForTest() {
	blogDBMu.Lock()
	defer blogDBMu.Unlock()

	blogDB = nil
	blogDBCfg = blogDBConfig{}
	newBlogDB = NewBlogDB
	pingBlogDB = defaultPingBlogDB
}

// setBlogConfigForTest sets minimal blog DB config values for tests.
func setBlogConfigForTest(addr string) {
	gconfig.Shared.Set("db.blog.addr", addr)
	gconfig.Shared.Set("db.blog.db", "blog-db")
	gconfig.Shared.Set("db.blog.user", "blog-user")
	gconfig.Shared.Set("db.blog.passwd", "blog-pass")
	gconfig.Shared.Set("db.blog.collections.posts", "posts")
	gconfig.Shared.Set("db.blog.collections.stats", "stats")
}

// Test_prepareDB_CachesByConfig ensures the DB is cached per configuration.
func Test_prepareDB_CachesByConfig(t *testing.T) {
	t.Cleanup(resetBlogDBForTest)
	setBlogConfigForTest("addr-1")
	pingBlogDB = func(ctx context.Context, db *Blog) error {
		return nil
	}

	var calls int32
	newBlogDB = func(ctx context.Context, addr, dbName, user, pwd, postColName, keywordColName string) (*Blog, error) {
		atomic.AddInt32(&calls, 1)
		return &Blog{dbName: dbName, postColName: postColName, keywordColName: keywordColName}, nil
	}

	ctx := context.Background()
	first, err := prepareDB(ctx)
	require.NoError(t, err)

	second, err := prepareDB(ctx)
	require.NoError(t, err)

	require.Same(t, first, second)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))

	setBlogConfigForTest("addr-2")
	third, err := prepareDB(ctx)
	require.NoError(t, err)
	require.NotSame(t, first, third)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

// Test_prepareDB_RetryOnFailure ensures failures do not cache a bad DB instance.
func Test_prepareDB_RetryOnFailure(t *testing.T) {
	t.Cleanup(resetBlogDBForTest)
	setBlogConfigForTest("addr-1")
	pingBlogDB = func(ctx context.Context, db *Blog) error {
		return nil
	}

	var calls int32
	newBlogDB = func(ctx context.Context, addr, dbName, user, pwd, postColName, keywordColName string) (*Blog, error) {
		cnt := atomic.AddInt32(&calls, 1)
		if cnt == 1 {
			return nil, errors.New("dial failed")
		}
		return &Blog{dbName: dbName, postColName: postColName, keywordColName: keywordColName}, nil
	}

	ctx := context.Background()
	_, err := prepareDB(ctx)
	require.Error(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))

	setBlogConfigForTest("addr-1")
	db, err := prepareDB(ctx)
	require.NoError(t, err)
	require.NotNil(t, db)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

// Test_prepareDB_ReconnectOnPingFailure ensures cache is refreshed when ping fails.
func Test_prepareDB_ReconnectOnPingFailure(t *testing.T) {
	t.Cleanup(resetBlogDBForTest)
	setBlogConfigForTest("addr-1")

	var calls int32
	newBlogDB = func(ctx context.Context, addr, dbName, user, pwd, postColName, keywordColName string) (*Blog, error) {
		atomic.AddInt32(&calls, 1)
		return &Blog{dbName: dbName, postColName: postColName, keywordColName: keywordColName}, nil
	}

	var pings int32
	pingBlogDB = func(ctx context.Context, db *Blog) error {
		cnt := atomic.AddInt32(&pings, 1)
		if cnt == 1 {
			return errors.New("ping failed")
		}
		return nil
	}

	ctx := context.Background()
	first, err := prepareDB(ctx)
	require.NoError(t, err)

	second, err := prepareDB(ctx)
	require.NoError(t, err)
	require.NotSame(t, first, second)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls))
}
