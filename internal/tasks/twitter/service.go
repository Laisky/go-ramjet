package twitter

import (
	"sync"

	gutils "github.com/Laisky/go-utils"
)

var (
	svc   *Service
	svcMu sync.Mutex
)

func initSvc() error {
	if svc != nil {
		return nil
	}

	svcMu.Lock()
	defer svcMu.Unlock()

	searchDao, err := NewSearchDao(gutils.Settings.GetString("tasks.twitter.clickhouse.dsn"))
	if err != nil {
		return err
	}

	twitterDao, err := NewTwitterDao(
		gutils.Settings.GetString("tasks.twitter.mongodb.addr"),
		gutils.Settings.GetString("tasks.twitter.mongodb.dbName"),
		gutils.Settings.GetString("tasks.twitter.mongodb.user"),
		gutils.Settings.GetString("tasks.twitter.mongodb.passwd"),
	)
	if err != nil {
		return err
	}

	svc = &Service{
		searchDao:  searchDao,
		twitterDao: twitterDao,
	}

	return nil
}

type Service struct {
	searchDao  *SearchDao
	twitterDao *TwitterDao
}

func getTweetUserID(tweet *Tweet) string {
	if tweet.User != nil {
		return tweet.User.ID
	}

	return ""
}

func (s *Service) SyncSearchTweets() error {
	latestT, err := s.searchDao.GetLatestCreatedAt()
	if err != nil {
		return err
	}

	iter := s.twitterDao.GetTweetsIter(latestT)
	defer iter.Close()

	tweet := new(Tweet)
	for iter.Next(tweet) {
		tweet := SearchTweet{
			TweetID:   tweet.ID,
			UserID:    getTweetUserID(tweet),
			Text:      tweet.Text,
			CreatedAt: tweet.CreatedAt,
		}

		if err := s.searchDao.SaveTweet(tweet); err != nil {
			return err
		}
	}

	return nil
}
