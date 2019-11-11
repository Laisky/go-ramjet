package keyword_test

import (
	"testing"

	"github.com/Laisky/go-ramjet/tasks/keyword"
)

var (
	b *keyword.Blog
)

func TestMongo(t *testing.T) {
	var (
		err error
		cnt string
	)
	cnt, err = b.LoadAllPostsCnt()
	if err != nil {
		t.Errorf("%+v", err)
	}
	if len(cnt) < 1000 {
		t.Error("can not load content")
	}
}

func TestIter(t *testing.T) {
	err := b.UpdatePostTagsById("4db1fed00000000000000000", []string{"1", "2"})
	if err != nil {
		t.Errorf("got error: %+v", err)
	}
}

// func init() {
// 	var err error
// 	b, err = keyword.NewBlogDB("localhost:27017", "blog", "posts", "statistics")
// 	if err != nil {
// 		panic(err)
// 	}
// }
