package blog_test

import (
	"context"
	"testing"

	"github.com/Laisky/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/Laisky/go-ramjet/internal/tasks/blog"
)

var (
	b *blog.Blog
)

func TestMongo(t *testing.T) {
	var (
		err error
		cnt string
	)

	cnt, err = b.LoadAllPostsCnt(context.Background())
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

	err = b.UpdatePostTagsByID(ctx, oid, []string{"1", "2"})
	require.NoError(t, err)
}

// func init() {
// 	var err error
// 	b, err = blog.NewBlogDB("localhost:27017", "blog", "posts", "statistics")
// 	if err != nil {
// 		panic(err)
// 	}
// }
