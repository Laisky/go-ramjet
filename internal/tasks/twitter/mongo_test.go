package twitter

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

func TestMongo(t *testing.T) {
	if os.Getenv("BLOG_MONGO_ADDR") == "" {
		t.Skip("integration test disabled: set BLOG_MONGO_ADDR to run")
	}
	ctx := context.Background()
	cnt, err := b.LoadAllPostsCnt(ctx)
	require.NoError(t, err)

	if len(cnt) < 1000 {
		t.Error("can not load content")
	}
}

func TestIter(t *testing.T) {
	if os.Getenv("BLOG_MONGO_ADDR") == "" {
		t.Skip("integration test disabled: set BLOG_MONGO_ADDR to run")
	}
	ctx := context.Background()
	bid, err := primitive.ObjectIDFromHex("4db1fed00000000000000000")
	require.NoError(t, err)

	err = b.UpdatePostTagsByID(ctx, bid, []string{"1", "2"})
	require.NoError(t, err)
}

// func init() {
// 	var err error
// 	b, err = blog.NewBlogDB("localhost:27017", "blog", "posts", "statistics")
// 	if err != nil {
// 		panic(err)
// 	}
// }
