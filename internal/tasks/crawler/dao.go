package crawler

import (
	"fmt"
	"time"

	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

type Dao struct {
	DB *BBT
}

func NewDao(addr, dbName, user, pwd, docusColName string) (*Dao, error) {
	db, err := NewBBTDB(addr, dbName, user, pwd, docusColName)
	if err != nil {
		return nil, err
	}

	return &Dao{DB: db}, nil
}

type SearchResult struct {
	Context string `json:"context"`
	URL     string `json:"url"`
	Title   string `json:"title"`
}

func (d *Dao) Search(text string) (rets []SearchResult, err error) {
	rets = []SearchResult{}
	if err = d.DB.colDocus.
		Find(bson.M{"text": bson.M{"$regex": fmt.Sprintf("/%s/", text)}}).
		Limit(99).
		All(&rets); err != nil {
		return nil, errors.Wrap(err, "search")
	}

	return rets, nil
}

func (d *Dao) Save(title, text, url string) error {
	now := time.Now().UTC()
	_, err := d.DB.colDocus.Upsert(
		bson.M{"url": url},
		bson.M{
			"$set": bson.M{
				"title":      title,
				"text":       text,
				"url":        url,
				"updated_at": now,
			},
			"$setOnInsert": bson.M{
				"created_at": now,
			},
		},
	)
	if err != nil {
		return errors.Wrap(err, "save")
	}

	log.Logger.Debug("save", zap.String("url", url))
	return nil
}
