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
	db                   mongo.DB
	dbName, docusColName string
}

func NewBBTDB(ctx context.Context, addr, dbName, user, pwd, docusColName string) (b *BBT, err error) {
	log.Logger.Info("connect to db",
		zap.String("addr", addr),
		zap.String("dbName", dbName),
		zap.String("docusColName", docusColName),
	)
	b = &BBT{
		dbName:       dbName,
		docusColName: docusColName,
	}

	b.db, err = mongo.NewDB(ctx,
		addr, dbName, user, pwd,
	)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func (db *BBT) docusCol() *mgo.Collection {
	return db.db.DB(db.dbName).C(db.docusColName)
}
