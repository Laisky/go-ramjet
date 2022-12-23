package twitter

import (
	"context"
	"sync"

	gconfig "github.com/Laisky/go-config/v2"
)

var (
	svc   *Service
	svcMu sync.Mutex
)

func initSvc(ctx context.Context) error {
	if svc != nil {
		return nil
	}

	svcMu.Lock()
	defer svcMu.Unlock()

	// searchDao, err := NewSearchDao(gconfig.Shared.GetString("db.clickhouse.dsn"))
	// if err != nil {
	// 	return err
	// }

	twitterDao, err := NewDao(ctx,
		gconfig.Shared.GetString("db.twitter.addr"),
		gconfig.Shared.GetString("db.twitter.db"),
		gconfig.Shared.GetString("db.twitter.user"),
		gconfig.Shared.GetString("db.twitter.passwd"),
	)
	if err != nil {
		return err
	}

	// twitterHome, err := NewDao(ctx,
	// 	gconfig.Shared.GetString("db.twitter-home.addr"),
	// 	gconfig.Shared.GetString("db.twitter-home.db"),
	// 	gconfig.Shared.GetString("db.twitter-home.user"),
	// 	gconfig.Shared.GetString("db.twitter-home.passwd"),
	// )
	// if err != nil {
	// 	return err
	// }

	svc = &Service{
		// searchDao:     searchDao,
		twitterDao: twitterDao,
		// twitterRepDao: twitterHome,
	}

	return nil
}

type Service struct {
	// searchDao  *SearchDao
	twitterDao *Dao
	// twitterRepDao replica twitter db
	// twitterRepDao *Dao
}

func getTweetUserID(tweet *Tweet) string {
	if tweet.User != nil {
		return tweet.User.ID
	}

	return ""
}

// SyncSearchTweets sync tweets to search db(clickhouse)
// func (s *Service) SyncSearchTweets() error {
// 	latestT, err := s.searchDao.GetLatestCreatedAt()
// 	if err != nil {
// 		return err
// 	}

// 	iter := s.twitterDao.GetTweetsIter(bson.M{
// 		"created_at": bson.M{"$gte": latestT},
// 	})
// 	defer gutils.SilentClose(iter)

// 	tweet := new(Tweet)
// 	for iter.Next(tweet) {
// 		tweet := SearchTweet{
// 			TweetID:   tweet.ID,
// 			UserID:    getTweetUserID(tweet),
// 			Text:      tweet.Text,
// 			CreatedAt: tweet.CreatedAt,
// 		}

// 		if err := s.searchDao.SaveTweet(tweet); err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }

// SyncReplicaTweets sync tweets to replica db
// func (s *Service) SyncReplicaTweets() error {
// 	latestT, err := s.twitterRepDao.GetLargestID()
// 	if err != nil {
// 		return err
// 	}

// 	iter := s.twitterDao.GetTweetsIter(bson.M{
// 		"_id": bson.M{"$gte": latestT},
// 	})
// 	defer gutils.SilentClose(iter)

// 	tweet := new(Tweet)
// 	for iter.Next(tweet) {
// 		if _, err = s.twitterRepDao.Upsert(
// 			bson.M{"_id": tweet.MongoID},
// 			bson.M{"$set": tweet},
// 		); err != nil {
// 			return errors.Wrapf(err, "upsert docu %v", tweet)
// 		}
// 	}

// 	return nil
// }
