package blog_test

import (
	"context"
	"os"
	"testing"

	"github.com/Laisky/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/Laisky/go-ramjet/internal/tasks/blog"
)

var (
	b *blog.Blog
)

func getBlog(t *testing.T) *blog.Blog {
	t.Helper()
	if b != nil {
		return b
	}

	addr := os.Getenv("BLOG_MONGO_ADDR")
	if addr == "" {
		t.Skip("integration test disabled: set BLOG_MONGO_ADDR to run")
	}
	db := os.Getenv("BLOG_MONGO_DB")
	if db == "" {
		db = "blog"
	}
	user := os.Getenv("BLOG_MONGO_USER")
	pass := os.Getenv("BLOG_MONGO_PASS")
	postCol := os.Getenv("BLOG_MONGO_POSTCOL")
	if postCol == "" {
		postCol = "posts"
	}
	keywordCol := os.Getenv("BLOG_MONGO_KEYCOL")
	if keywordCol == "" {
		keywordCol = "statistics"
	}

	var err error
	b, err = blog.NewBlogDB(context.Background(), addr, db, user, pass, postCol, keywordCol)
	require.NoError(t, err)
	return b
}

func TestMongo(t *testing.T) {
	var (
		err error
		cnt string
	)

	cnt, err = getBlog(t).LoadAllPostsCnt(context.Background())
	if err != nil {
		t.Errorf("%+v", err)
	}
	if len(cnt) < 1000 {
		t.Error("can not load content")
	}
}

func TestIter(t *testing.T) {
	ctx := context.Background()
	oid, err := primitive.ObjectIDFromHex("4db1fed00000000000000000")
	require.NoError(t, err)

	err = getBlog(t).UpdatePostTagsByID(ctx, oid, []string{"1", "2"})
	require.NoError(t, err)
}

// func init() {
// 	var err error
// 	b, err = blog.NewBlogDB("localhost:27017", "blog", "posts", "statistics")
// 	if err != nil {
// 		panic(err)
// 	}
// }
