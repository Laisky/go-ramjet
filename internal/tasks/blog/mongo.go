package blog

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/db/mongo"
	"github.com/Laisky/zap"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mongoLib "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/Laisky/go-ramjet/library/log"
)

type Post struct {
	ID        primitive.ObjectID `bson:"_id"`
	Cnt       string             `bson:"post_content"`
	Title     string             `bson:"post_title"`
	Name      string             `bson:"post_name"`
	CreatedAt time.Time          `bson:"post_created_at"`
}

type Blog struct {
	db mongo.DB
	dbName,
	postColName,
	keywordColName string
}

func NewBlogDB(ctx context.Context, addr, dbName, user, pwd, postColName, keywordColName string) (b *Blog, err error) {
	log.Logger.Info("connect to db",
		zap.String("user", user),
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
	b.db, err = mongo.NewDB(ctx, mongo.DialInfo{
		Addr:   addr,
		DBName: dbName,
		User:   user,
		Pwd:    pwd,
	})
	if err != nil {
		return nil, err
	}

	return b, nil
}

func (b *Blog) postCol() *mongoLib.Collection {
	return b.db.DB(b.dbName).Collection(b.postColName)
}

func (b *Blog) keywordCol() *mongoLib.Collection {
	return b.db.DB(b.dbName).Collection(b.keywordColName)
}

func (b *Blog) LoadAllPostsCnt(ctx context.Context) (cnt string, err error) {
	cnt = ""
	iter, err := b.GetPostIter(ctx)
	if err != nil {
		return "", errors.Wrap(err, "try to load all posts content got error")
	}
	defer iter.Close(ctx) // nolint: errcheck

	for iter.Next(ctx) {
		p := &Post{}
		if err = iter.Decode(p); err != nil {
			return "", errors.Wrap(err, "try to load all posts content got error")
		}

		cnt += p.Cnt
	}

	return cnt, nil
}

func (b *Blog) GetPostIter(ctx context.Context) (*mongoLib.Cursor, error) {
	return b.postCol().Find(ctx,
		bson.D{},
		options.Find().
			SetSort(bson.M{"_id": -1}),
	)
}

func (b *Blog) UpdatePostTagsByID(ctx context.Context, bid primitive.ObjectID, tags []string) (err error) {
	_, err = b.postCol().UpdateByID(ctx,
		bid,
		bson.M{"$set": bson.M{"post_tags": tags}},
	)
	if err != nil {
		return errors.Wrap(err, "try to update post got error")
	}
	return nil
}

func (b *Blog) Close(ctx context.Context) {
	b.db.Close(ctx) // nolint: errcheck
}
