package twitter

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/go-utils/v6/json"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/laisky-blog-graphql/library/db/mongo"
	"github.com/Laisky/zap"
	"go.mongodb.org/mongo-driver/bson"
	mongoLib "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/gorm"

	"github.com/Laisky/go-ramjet/library/db/clickhouse"
	"github.com/Laisky/go-ramjet/library/log"
)

type mongoDao struct {
	db                    mongo.DB
	dbName, tweetsColName string
}

func NewDao(ctx context.Context, addr, dbName, user, pwd string) (d *mongoDao, err error) {
	log.Logger.Info("connect to db",
		zap.String("addr", addr),
		zap.String("dbName", dbName),
	)

	d = &mongoDao{
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

func (d *mongoDao) tweetsCol() *mongoLib.Collection {
	return d.db.DB(d.dbName).Collection(d.tweetsColName)
}

func (d *mongoDao) GetTweetsIter(ctx context.Context, cond bson.M) (*mongoLib.Cursor, error) {
	log.Logger.Debug("load tweets", zap.Any("condition", cond))
	return d.tweetsCol().Find(ctx, cond, options.Find().SetSort(bson.M{"id": 1}))
}

func (d *mongoDao) GetLargestID(ctx context.Context) (*Tweet, error) {
	tweet := new(Tweet)
	err := d.tweetsCol().FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.M{"id": -1})).Decode(tweet)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load tweet with largest id")
	}
	return tweet, nil
}

func (d *mongoDao) Upsert(ctx context.Context, cond, docu bson.M) (*mongoLib.UpdateResult, error) {
	log.Logger.Info("upsert tweet", zap.Any("condition", cond))
	opt := options.Update().SetUpsert(true)
	return d.tweetsCol().UpdateOne(ctx, cond, docu, opt)
}

type ClickhouseDao struct {
	db *gorm.DB
}

func NewSearchDao(dsn string) (*ClickhouseDao, error) {
	db, err := clickhouse.New(dsn)
	if err != nil {
		return nil, err
	}

	return &ClickhouseDao{db: db}, nil
}

// GetLargestID returns the largest id of tweets
//
// do not use this API, twitter's id is not monotonical
func (d *ClickhouseDao) GetLargestID() (string, error) {
	var id string
	if err := d.db.Model(ClickhouseTweet{}).
		Order("id desc").
		Limit(1).
		Pluck("id", &id).Error; err != nil {
		return "", errors.Wrapf(err, "load largest id")
	}

	return id, nil
}

func (d *ClickhouseDao) GetLatestCreatedAt() (time.Time, error) {
	var t time.Time
	if err := d.db.Model(ClickhouseTweet{}).
		Order("created_at DESC").
		Limit(1).
		Pluck("created_at", &t).Error; err != nil {
		return t, errors.Wrapf(err, "load latest created_at")
	}

	return t, nil
}

func (d *ClickhouseDao) SaveTweet(tweet ClickhouseTweet) error {
	return d.db.FirstOrCreate(&tweet, ClickhouseTweet{
		TweetID: tweet.TweetID,
	}).Error
}

type ElasticsearchDao struct {
	api    string
	logger glog.Logger
	cli    *http.Client
}

func newElasticsearchDao(logger glog.Logger, api string) (ins *ElasticsearchDao, err error) {
	ins = &ElasticsearchDao{
		logger: logger,
		api:    strings.TrimSuffix(api, "/") + "/",
	}

	if ins.cli, err = gutils.NewHTTPClient(); err != nil {
		return nil, errors.Wrap(err, "new http client")
	}

	// check es health
	if resp, err := ins.cli.Get(ins.api + "_cluster/health"); err != nil {
		return nil, errors.Wrap(err, "check es health")
	} else {
		defer gutils.LogErr(resp.Body.Close, logger)
		if resp.StatusCode != http.StatusOK {
			respCnt, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, errors.Wrap(err, "read response")
			}

			return nil, errors.Errorf("check es health failed: [%v]%s", resp.Status, string(respCnt))
		}
	}

	return ins, nil
}

func (d *ElasticsearchDao) GetLargestID(ctx context.Context) (largestID float64, err error) {
	query := map[string]interface{}{
		"aggs": map[string]interface{}{
			"max_id": map[string]interface{}{
				"max": map[string]interface{}{
					"field": "id",
				},
			},
		},
	}

	bodyPayload, err := json.Marshal(query)
	if err != nil {
		return largestID, errors.Wrap(err, "marshal query")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		d.api+"_search", bytes.NewReader(bodyPayload))
	if err != nil {
		return largestID, errors.Wrap(err, "new request")
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := d.cli.Do(req)
	if err != nil {
		return largestID, errors.Wrap(err, "do request")
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger)

	var result struct {
		Aggregations struct {
			MaxID struct {
				Value float64 `json:"value"`
			} `json:"max_id"`
		} `json:"aggregations"`
	}

	bodyPayload, err = io.ReadAll(resp.Body)
	if err != nil {
		return largestID, errors.Wrap(err, "read response")
	}
	// fmt.Println(string(bodyPayload))

	if err := json.Unmarshal(bodyPayload, &result); err != nil {
		return largestID, errors.Wrap(err, "decode response")
	}

	return result.Aggregations.MaxID.Value, nil
}

func (d *ElasticsearchDao) SaveTweet(ctx context.Context, tweet *Tweet) error {
	body, err := json.Marshal(tweet)
	if err != nil {
		return errors.Wrap(err, "marshal tweet")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		d.api+"tweets/_doc/"+tweet.ID, bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "new request")
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := d.cli.Do(req)
	if err != nil {
		return errors.Wrap(err, "do request")
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger)

	if !gutils.Contains([]int{200, 201}, resp.StatusCode) {
		respCnt, err := io.ReadAll(resp.Body)
		if err != nil {
			return errors.Wrap(err, "read response")
		}

		return errors.Errorf("save tweet failed: [%v]%s", resp.Status, string(respCnt))
	}

	return nil
}
