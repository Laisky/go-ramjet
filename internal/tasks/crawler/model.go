package crawler

import (
	"context"
	"time"

	"github.com/Laisky/zap"
	"gopkg.in/mgo.v2"

	"github.com/Laisky/laisky-blog-graphql/library/db/mongo"

	"github.com/Laisky/go-ramjet/library/log"
)

type Docu struct {
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
	Text      string    `bson:"text" json:"text"`
	Title     string    `bson:"title" json:"title"`
	URL       string    `bson:"url" json:"url"`
}

type BBT struct {
	mongo.DB
	colDocus *mgo.Collection
}

func NewBBTDB(ctx context.Context, addr, dbName, user, pwd, docusColName string) (b *BBT, err error) {
	log.Logger.Info("connect to db",
		zap.String("addr", addr),
		zap.String("dbName", dbName),
		zap.String("docusColName", docusColName),
	)
	b = &BBT{}

	b.DB, err = mongo.NewDB(ctx,
		addr, dbName, user, pwd,
	)
	if err != nil {
		return nil, err
	}

	db := b.DB.DB(dbName)
	b.colDocus = db.C(docusColName)
	return b, nil
}
