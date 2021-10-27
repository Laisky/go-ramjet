package twitter

import (
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/Laisky/go-ramjet/library/config"
	gutils "github.com/Laisky/go-utils"
	"github.com/stretchr/testify/require"
)

func TestSearchDao_GetLargestID(t *testing.T) {
	config.LoadTest()
	d, err := NewSearchDao(
		gutils.Settings.GetString("tasks.twitter.clickhouse.dsn"),
	)
	require.NoError(t, err)

	v, err := d.GetLargestID()
	require.NoError(t, err)
	t.Logf("got larget id %v", v)
}

func TestSearchDao_SaveTweets(t *testing.T) {
	config.LoadTest()
	d, err := NewSearchDao(
		gutils.Settings.GetString("tasks.twitter.clickhouse.dsn"),
	)
	require.NoError(t, err)

	tweets := []SearchTweet{
		SearchTweet{TweetID: strconv.Itoa(rand.Int()), Text: gutils.RandomStringWithLength(10), UserID: strconv.Itoa(rand.Int())},
		SearchTweet{TweetID: strconv.Itoa(rand.Int()), Text: gutils.RandomStringWithLength(10), UserID: strconv.Itoa(rand.Int())},
	}

	for i := range tweets {
		err = d.SaveTweet(tweets[i])
		require.NoError(t, err)
	}
}

func TestTwitterDao_GetTweetsIter(t *testing.T) {
	config.LoadTest()
	d, err := NewTwitterDao(
		gutils.Settings.GetString("tasks.twitter.mongodb.addr"),
		gutils.Settings.GetString("tasks.twitter.mongodb.dbName"),
		gutils.Settings.GetString("tasks.twitter.mongodb.user"),
		gutils.Settings.GetString("tasks.twitter.mongodb.passwd"),
	)
	require.NoError(t, err)

	iter := d.GetTweetsIter(time.Time{})
	defer iter.Close()

	docu := new(Tweet)
	require.True(t, iter.Next(docu))
	require.True(t, len(docu.Text) > 0)
}
