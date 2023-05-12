package twitter

import (
	"context"
	"math/rand"
	"strconv"
	"testing"

	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/testify/require"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/Laisky/go-ramjet/library/config"
)

func TestSearchDao_GetLargestID(t *testing.T) {
	config.LoadTest()
	d, err := NewSearchDao(
		gconfig.Shared.GetString("tasks.twitter.clickhouse.dsn"),
	)
	require.NoError(t, err)

	v, err := d.GetLargestID()
	require.NoError(t, err)
	t.Logf("got larget id %v", v)
}

func TestSearchDao_SaveTweets(t *testing.T) {
	config.LoadTest()
	d, err := NewSearchDao(
		gconfig.Shared.GetString("tasks.twitter.clickhouse.dsn"),
	)
	require.NoError(t, err)

	tweets := []SearchTweet{
		{TweetID: strconv.Itoa(rand.Int()), Text: gutils.RandomStringWithLength(10), UserID: strconv.Itoa(rand.Int())},
		{TweetID: strconv.Itoa(rand.Int()), Text: gutils.RandomStringWithLength(10), UserID: strconv.Itoa(rand.Int())},
	}

	for i := range tweets {
		err = d.SaveTweet(tweets[i])
		require.NoError(t, err)
	}
}

func TestTwitterDao_GetTweetsIter(t *testing.T) {
	ctx := context.Background()
	config.LoadTest()
	d, err := NewDao(context.Background(),
		gconfig.Shared.GetString("tasks.twitter.mongodb.addr"),
		gconfig.Shared.GetString("tasks.twitter.mongodb.dbName"),
		gconfig.Shared.GetString("tasks.twitter.mongodb.user"),
		gconfig.Shared.GetString("tasks.twitter.mongodb.passwd"),
	)
	require.NoError(t, err)

	iter, err := d.GetTweetsIter(ctx, bson.M{})
	require.NoError(t, err)
	defer iter.Close(ctx)

	docu := new(Tweet)
	require.True(t, iter.Next(ctx))
	require.True(t, len(docu.Text) > 0)
}
