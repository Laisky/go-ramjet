package keyword

import (
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	defaultTimeout = 30 * time.Second
)

var (
	keywordType = &bson.M{"types": "keyword"}
)

type Post struct {
	Id   bson.ObjectId `bson:"_id"`
	Cnt  string        `bson:"post_content"`
	Name string        `bson:"post_name"`
}

type DB struct {
	s  *mgo.Session
	db *mgo.Database
}

func (d *DB) Dial(dialInfo *mgo.DialInfo) error {
	s, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		return errors.Wrap(err, "can not connect to db")
	}

	d.s = s
	return nil
}

func (d *DB) Close() {
	d.s.Close()
}

type Blog struct {
	DB
	posts, keywords *mgo.Collection
}

func NewBlogDB(addr, dbName, user, pwd, postColName, keywordColName string) (b *Blog, err error) {
	utils.Logger.Info("connect to db",
		zap.String("addr", addr),
		zap.String("dbName", dbName),
		zap.String("postColName", postColName),
		zap.String("keywordColName", keywordColName),
	)
	b = &Blog{}

	dialInfo := &mgo.DialInfo{
		Addrs:     []string{addr},
		Direct:    true,
		Timeout:   defaultTimeout,
		Database:  dbName,
		Username:  user,
		Password:  pwd,
		PoolLimit: 1000,
	}
	err = b.Dial(dialInfo)
	if err != nil {
		return nil, err
	}

	b.db = b.s.DB(dbName)
	b.posts = b.db.C(postColName)
	b.keywords = b.db.C(keywordColName)
	return b, nil
}

func (b *Blog) LoadAllPostsCnt() (cnt string, err error) {
	p := &Post{}
	cnt = ""
	iter := b.GetPostIter()
	for iter.Next(p) {
		cnt += p.Cnt
	}

	if err = iter.Close(); err != nil {
		return "", errors.Wrap(err, "try to load all posts content got error")
	}
	return cnt, nil
}

func (b *Blog) GetPostIter() *mgo.Iter {
	return b.posts.Find(nil).Iter()
}

func (b *Blog) UpdatePostTagsById(bid string, tags []string) (err error) {
	err = b.posts.UpdateId(
		bson.ObjectIdHex(bid),
		bson.M{"$set": bson.M{"post_tags": tags}},
	)
	if err != nil {
		return errors.Wrap(err, "try to update post got error")
	}
	return nil
}
