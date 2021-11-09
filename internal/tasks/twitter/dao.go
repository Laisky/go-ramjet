package twitter

import (
	"time"

	"github.com/Laisky/zap"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gorm.io/gorm"

	"github.com/Laisky/go-ramjet/library/db/clickhouse"
	"github.com/Laisky/go-ramjet/library/db/mongo"
	"github.com/Laisky/go-ramjet/library/log"
)

type TwitterDao struct {
	mongo.DB
	db     *mgo.Database
	tweets *mgo.Collection
}

func NewTwitterDao(addr, dbName, user, pwd string) (d *TwitterDao, err error) {
	log.Logger.Info("connect to db",
		zap.String("addr", addr),
		zap.String("dbName", dbName),
	)

	d = new(TwitterDao)
	dialInfo := &mgo.DialInfo{
		Addrs:     []string{addr},
		Direct:    true,
		Timeout:   10 * time.Second,
		Database:  dbName,
		Username:  user,
		Password:  pwd,
		PoolLimit: 1000,
	}
	err = d.Dial(dialInfo)
	if err != nil {
		return nil, err
	}

	d.db = d.DB.S.DB(dbName)
	d.tweets = d.db.C("tweets")
	return d, nil
}

func (d *TwitterDao) GetTweetsIter(cond bson.M) *mgo.Iter {
	log.Logger.Debug("load tweets", zap.Any("condition", cond))
	return d.tweets.Find(cond).Sort("created_at").Iter()
}

func (d *TwitterDao) GetLargestID() (largestID bson.ObjectId, err error) {
	tweet := new(Tweet)
	if err = d.tweets.Find(bson.M{}).
		Select(bson.M{"_id": 1}).
		Sort("-id").
		Limit(1).
		One(tweet); err != nil {
		return "", errors.Wrapf(err, "load largest id")
	}

	if tweet.MongoID == nil {
		return "", errors.New("no id found")
	}

	return *tweet.MongoID, nil
}

func (d *TwitterDao) Upsert(cond, docu bson.M) (*mgo.ChangeInfo, error) {
	log.Logger.Info("upsert tweet", zap.Any("condition", cond))
	return d.tweets.Upsert(cond, docu)
}

type SearchDao struct {
	db *gorm.DB
}

func NewSearchDao(dsn string) (*SearchDao, error) {
	db, err := clickhouse.New(dsn)
	if err != nil {
		return nil, err
	}

	return &SearchDao{db: db}, nil
}

// GetLargestID returns the largest id of tweets
//
// do not use this API, twitter's id is not monotonical
func (d *SearchDao) GetLargestID() (string, error) {
	var id string
	if err := d.db.Model(SearchTweet{}).
		Order("id desc").
		Limit(1).
		Pluck("id", &id).Error; err != nil {
		return "", errors.Wrapf(err, "load largest id")
	}

	return id, nil
}

func (d *SearchDao) GetLatestCreatedAt() (time.Time, error) {
	var t time.Time
	if err := d.db.Model(SearchTweet{}).
		Order("created_at DESC").
		Limit(1).
		Pluck("created_at", &t).Error; err != nil {
		return t, errors.Wrapf(err, "load latest created_at")
	}

	return t, nil
}

func (d *SearchDao) SaveTweet(tweet SearchTweet) error {
	return d.db.FirstOrCreate(&tweet, SearchTweet{
		TweetID: tweet.TweetID,
	}).Error
}
