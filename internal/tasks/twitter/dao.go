package twitter

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/db/mongo"
	"github.com/Laisky/zap"
	"go.mongodb.org/mongo-driver/bson"
	mongoLib "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/gorm"

	"github.com/Laisky/go-ramjet/library/db/clickhouse"
	"github.com/Laisky/go-ramjet/library/log"
)

type Dao struct {
	db                    mongo.DB
	dbName, tweetsColName string
}

func NewDao(ctx context.Context, addr, dbName, user, pwd string) (d *Dao, err error) {
	log.Logger.Info("connect to db",
		zap.String("addr", addr),
		zap.String("dbName", dbName),
	)

	d = &Dao{
		dbName:        dbName,
		tweetsColName: "tweets",
	}
	d.db, err = mongo.NewDB(ctx,
		mongo.DialInfo{
			Addr:   addr,
			DBName: dbName,
			User:   user,
			Pwd:    pwd,
		},
	)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (db *Dao) tweetsCol() *mongoLib.Collection {
	return db.db.DB(db.dbName).Collection(db.tweetsColName)
}

func (d *Dao) GetTweetsIter(ctx context.Context, cond bson.M) (*mongoLib.Cursor, error) {
	log.Logger.Debug("load tweets", zap.Any("condition", cond))
	return d.tweetsCol().Find(ctx, cond, options.Find().SetSort(bson.M{"created_at": -1}))
}

func (d *Dao) GetLargestID(ctx context.Context) (*Tweet, error) {
	tweet := new(Tweet)
	err := d.tweetsCol().FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.M{"id": -1})).Decode(tweet)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load tweet with largest id")
	}
	return tweet, nil
}

func (d *Dao) Upsert(ctx context.Context, cond, docu bson.M) (*mongoLib.UpdateResult, error) {
	log.Logger.Info("upsert tweet", zap.Any("condition", cond))
	opt := options.Update().SetUpsert(true)
	return d.tweetsCol().UpdateOne(ctx, cond, docu, opt)
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
