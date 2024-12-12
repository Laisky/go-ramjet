package twitter

import (
	"context"
	"math/rand"
	"strconv"
	"testing"

	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/testify/require"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/Laisky/go-ramjet/library/config"
	"github.com/Laisky/go-ramjet/library/log"
)

func TestSearchDao_GetLargestID(t *testing.T) {
	config.LoadTest(t)
	d, err := NewSearchDao(
		gconfig.Shared.GetString("tasks.twitter.clickhouse.dsn"),
	)
	require.NoError(t, err)

	v, err := d.GetLargestID()
	require.NoError(t, err)
	t.Logf("got larget id %v", v)
}

func TestSearchDao_SaveTweets(t *testing.T) {
	config.LoadTest(t)
	d, err := NewSearchDao(
		gconfig.Shared.GetString("tasks.twitter.clickhouse.dsn"),
	)
	require.NoError(t, err)

	tweets := []ClickhouseTweet{
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
	config.LoadTest(t)
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

func TestElasticsearchDao_GetLargestID(t *testing.T) {
	ctx := context.Background()
	logger := log.Logger.Named("test")
	config.LoadTest(t)

	d, err := newElasticsearchDao(logger,
		gconfig.Shared.GetString("tasks.twitter.elasticsearch.addr"))
	require.NoError(t, err)

	t.Run("test get largest id", func(t *testing.T) {
		largestID, err := d.GetLargestID(ctx)
		require.NoError(t, err)
		t.Logf("got largest id %v", largestID)
	})

	t.Run("save tweet", func(t *testing.T) {
		tweet := &Tweet{
			ID:   strconv.Itoa(rand.Int()),
			Text: gutils.RandomStringWithLength(10),
			// User: &User{
			// 	ID: strconv.Itoa(rand.Int()),
			// },
		}
		err = d.SaveTweet(ctx, tweet)
		require.NoError(t, err)
	})
}
