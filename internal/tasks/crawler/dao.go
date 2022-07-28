package crawler

import (
	"fmt"
	"strings"
	"time"

	gutils "github.com/Laisky/go-utils/v2"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"

	"github.com/Laisky/go-ramjet/library/log"
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
	Context string `bson:"context" json:"context"`
	URL     string `bson:"url" json:"url"`
	Title   string `bson:"title" json:"title"`

	ID   bson.ObjectId `bson:"_id" json:"-"`
	Text string        `bson:"text" json:"-"`
}

func (d *Dao) Search(text string) (rets []SearchResult, err error) {
	if err = d.DB.colDocus.
		Find(bson.M{"text": bson.M{"$regex": bson.RegEx{
			Pattern: text,
			Options: "im",
		}}}).
		Limit(99).
		All(&rets); err != nil {
		return nil, errors.Wrap(err, "search")
	}

	d.extractSearchContext(text, rets)
	return rets, nil
}

const searchCtxSpan = 20

func (d *Dao) extractSearchContext(text string, rets []SearchResult) {
	for i := range rets {
		idx := strings.Index(rets[i].Text, text)
		top := gutils.Max(idx-searchCtxSpan, 0)
		bottom := gutils.Min(idx+searchCtxSpan, len(rets[i].Text))
		rets[i].Context = fmt.Sprintf("%s<mark>%s</mark>%s",
			rets[i].Text[top:top+searchCtxSpan],
			rets[i].Text[idx:idx+len(text)],
			rets[i].Text[idx+len(text):bottom],
		)

	}
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
