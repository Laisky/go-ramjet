package twitter

import (
	"context"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"go.mongodb.org/mongo-driver/bson"
)

func newSvc(ctx context.Context,
	logger glog.Logger,
	twitterDao *mongoDao,
	esDao *ElasticsearchDao,
) (svc *Service, err error) {
	svc = &Service{
		logger:     logger,
		twitterDao: twitterDao,
		esDao:      esDao,
	}

	return svc, nil
}

type Service struct {
	logger     glog.Logger
	twitterDao *mongoDao
	esDao      *ElasticsearchDao
}

// func getTweetUserID(tweet *Tweet) string {
// 	if tweet.User != nil {
// 		return tweet.User.ID
// 	}

// 	return ""
// }

// syncTweets sync tweets to elasticsearch
func (s *Service) syncTweets(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	latestTweetID, err := s.esDao.GetLargestID(ctx)
	if err != nil {
		return errors.Wrap(err, "get latest tweet id")
	}

	iter, err := s.twitterDao.GetTweetsIter(ctx, bson.M{
		"id": bson.M{"$gt": latestTweetID},
	})
	if err != nil {
		return errors.Wrap(err, "get tweets iter")
	}

	var i int
	for iter.Next(ctx) {
		tweet := new(Tweet)
		if err = iter.Decode(tweet); err != nil {
			s.logger.Warn("decode tweet",
				zap.ByteString("tweet", iter.Current),
				zap.Error(err))
		}

		if err := s.esDao.SaveTweet(ctx, tweet); err != nil {
			return errors.Wrap(err, "save tweet")
		}

		i++
		if i%10 == 0 {
			s.logger.Info("sync tweets",
				zap.String("latestTweetID", tweet.ID),
				zap.Int("count", i))
		}
	}

	return nil
}

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
