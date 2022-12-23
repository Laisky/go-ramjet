package blog

import (
	"context"
	"time"

	"github.com/Laisky/errors"
	"github.com/Laisky/laisky-blog-graphql/library/db/mongo"
	"github.com/Laisky/zap"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/Laisky/go-ramjet/library/log"
)

const (
	defaultTimeout = 30 * time.Second
)

type Post struct {
	ID        bson.ObjectId `bson:"_id"`
	Cnt       string        `bson:"post_content"`
	Title     string        `bson:"post_title"`
	Name      string        `bson:"post_name"`
	CreatedAt time.Time     `bson:"post_created_at"`
}

type Blog struct {
	db mongo.DB
	dbName,
	postColName,
	keywordColName string
}

func NewBlogDB(ctx context.Context, addr, dbName, user, pwd, postColName, keywordColName string) (b *Blog, err error) {
	log.Logger.Info("connect to db",
		zap.String("addr", addr),
		zap.String("dbName", dbName),
		zap.String("postColName", postColName),
		zap.String("keywordColName", keywordColName),
	)
	b = &Blog{
		dbName:         dbName,
		postColName:    postColName,
		keywordColName: keywordColName,
	}
	b.db, err = mongo.NewDB(ctx,
		addr,
		dbName,
		user,
		pwd,
	)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func (b *Blog) postCol() *mgo.Collection {
	return b.db.DB(b.dbName).C(b.postColName)
}

func (b *Blog) keywordCol() *mgo.Collection {
	return b.db.DB(b.dbName).C(b.keywordColName)
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
	return b.postCol().Find(nil).Iter()
}

func (b *Blog) UpdatePostTagsByID(bid string, tags []string) (err error) {
	err = b.postCol().UpdateId(
		bson.ObjectIdHex(bid),
		bson.M{"$set": bson.M{"post_tags": tags}},
	)
	if err != nil {
		return errors.Wrap(err, "try to update post got error")
	}
	return nil
}

func (b *Blog) Close() {
	b.db.Close()
}
