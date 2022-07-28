package crawler

import (
	"time"

	"github.com/Laisky/zap"
	"gopkg.in/mgo.v2"

	"github.com/Laisky/go-ramjet/library/db/mongo"
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

func NewBBTDB(addr, dbName, user, pwd, docusColName string) (b *BBT, err error) {
	log.Logger.Info("connect to db",
		zap.String("addr", addr),
		zap.String("dbName", dbName),
		zap.String("docusColName", docusColName),
	)
	b = &BBT{}

	dialInfo := &mgo.DialInfo{
		Addrs:     []string{addr},
		Direct:    true,
		Timeout:   30 * time.Second,
		Database:  dbName,
		Username:  user,
		Password:  pwd,
		PoolLimit: 1000,
	}
	err = b.Dial(dialInfo)
	if err != nil {
		return nil, err
	}

	db := b.S.DB(dbName)
	b.colDocus = db.C(docusColName)
	return b, nil
}
