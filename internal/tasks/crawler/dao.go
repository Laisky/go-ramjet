package crawler

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"time"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/Laisky/go-ramjet/library/db/mongosearch"
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
	URL     string `bson:"url"     json:"url"`
	Title   string `bson:"title"   json:"title"`

	ID   primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	Text string             `bson:"text"          json:"-"`
}

// Search search text in title and content
func (d *Dao) Search(ctx context.Context, text string) (rets []SearchResult, err error) {
	// search in title
	titleRets := make([]SearchResult, 0)
	textRegex := mongosearch.LiteralSubstring(text)
	if cursor, err := d.DB.docusCol().Find(ctx,
		bson.M{"title": textRegex},
		options.Find().SetLimit(10)); err != nil {
		return nil, errors.Wrap(err, "search")
	} else if err := cursor.All(ctx, &titleRets); err != nil {
		return nil, errors.Wrap(err, "search")
	}

	// search in content
	contentRets := make([]SearchResult, 0)
	if cursor, err := d.DB.docusCol().Find(ctx,
		bson.M{"text": textRegex},
		options.Find().SetLimit(10)); err != nil {
		return nil, errors.Wrap(err, "search")
	} else if err := cursor.All(ctx, &contentRets); err != nil {
		return nil, errors.Wrap(err, "search")
	}

	rets = d.extractSearchContext(text, append(titleRets, contentRets...))
	return rets, nil
}

const searchCtxSpan = 20

// extractSearchContext keeps the results whose text contains query and sets
// each one's Context to an HTML-escaped snippet with the match highlighted.
//
// query is matched literally and case-insensitively, exactly like the MongoDB
// filter built by Search, so regular-expression syntax in it only matches
// itself.
//
// Parameters:
//   - query: untrusted search text.
//   - rets: candidate results returned by MongoDB.
//
// Returns:
//   - []SearchResult: the results whose text contains query, with Context set.
func (d *Dao) extractSearchContext(query string, rets []SearchResult) []SearchResult {
	// QuoteMeta output always compiles.
	reg := regexp.MustCompile("(?i)" + regexp.QuoteMeta(query))

	var filtered []SearchResult
	for i := range rets {
		loc := reg.FindStringIndex(rets[i].Text)
		if loc == nil {
			continue
		}

		begin := gutils.Max(loc[0]-searchCtxSpan, 0)
		end := gutils.Min(loc[1]+searchCtxSpan, len(rets[i].Text))
		rets[i].Context = fmt.Sprintf("%s<mark>%s</mark>%s",
			html.EscapeString(rets[i].Text[begin:loc[0]]),
			html.EscapeString(rets[i].Text[loc[0]:loc[1]]),
			html.EscapeString(rets[i].Text[loc[1]:end]),
		)

		filtered = append(filtered, rets[i])
	}

	return filtered
}

func (d *Dao) RemoveLegacy(ctx context.Context, updateBefore time.Time) error {
	if _, err := d.DB.docusCol().DeleteMany(ctx,
		bson.M{"updated_at": bson.M{"$lt": updateBefore}},
	); err != nil {
		return errors.Wrap(err, "remove legacy")
	}

	return nil
}

func (d *Dao) Save(ctx context.Context, title, text, url string) error {
	now := time.Now().UTC()
	_, err := d.DB.docusCol().UpdateOne(ctx,
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
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return errors.Wrap(err, "save")
	}

	log.Logger.Debug("save", zap.String("url", url))
	return nil
}
