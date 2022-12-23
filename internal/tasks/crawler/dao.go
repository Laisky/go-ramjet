package crawler

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/Laisky/errors"
	gutils "github.com/Laisky/go-utils/v3"
	"github.com/Laisky/zap"
	"gopkg.in/mgo.v2/bson"

	"github.com/Laisky/go-ramjet/library/log"
)

type Dao struct {
	DB *BBT
}

func NewDao(ctx context.Context, addr, dbName, user, pwd, docusColName string) (*Dao, error) {
	db, err := NewBBTDB(ctx, addr, dbName, user, pwd, docusColName)
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
	rets = make([]SearchResult, 0)
	if err = d.DB.docusCol().
		Find(bson.M{"text": bson.M{"$regex": bson.RegEx{
			Pattern: text,
			Options: "im",
		}}}).
		Limit(99).
		All(&rets); err != nil {
		return nil, errors.Wrap(err, "search")
	}

	rets = d.extractSearchContext(text, rets)
	return rets, nil
}

const searchCtxSpan = 20

func (d *Dao) extractSearchContext(pattern string, rets []SearchResult) []SearchResult {
	reg, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		log.Logger.Warn("compile pattern", zap.Error(err))
		return rets
	}

	var filtered []SearchResult
	for i := range rets {
		loc := reg.FindStringIndex(rets[i].Text)
		if loc == nil {
			continue
		}

		begin := gutils.Max(loc[0]-searchCtxSpan, 0)
		end := gutils.Min(loc[1]+searchCtxSpan, len(rets[i].Text))
		rets[i].Context = fmt.Sprintf("%s<mark>%s</mark>%s",
			rets[i].Text[begin:loc[0]],
			rets[i].Text[loc[0]:loc[1]],
			rets[i].Text[loc[1]:end],
		)

		filtered = append(filtered, rets[i])
	}

	return filtered
}

func (d *Dao) RemoveLegacy(updateBefore time.Time) error {
	if _, err := d.DB.docusCol().RemoveAll(
		bson.M{"updated_at": bson.M{"$lt": updateBefore}},
	); err != nil {
		return errors.Wrap(err, "remove legacy")
	}

	return nil
}

func (d *Dao) Save(title, text, url string) error {
	now := time.Now().UTC()
	_, err := d.DB.docusCol().Upsert(
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
