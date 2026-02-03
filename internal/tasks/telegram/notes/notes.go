package notes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/laisky-blog-graphql/library/db/mongo"
	model "github.com/Laisky/laisky-blog-graphql/library/models/telegram"
	"github.com/Laisky/zap"
	"github.com/PuerkitoBio/goquery"
	"go.mongodb.org/mongo-driver/bson"
	mongolib "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Service represents the service for handling notes.
type Service struct {
	logger glog.Logger
	col    *mongolib.Collection
}

// NewService creates a new Service instance.
func NewService(ctx context.Context, logger glog.Logger,
	addr, db, user, passwd, col string) (svc *Service, err error) {
	client, err := mongo.NewDB(ctx, mongo.DialInfo{
		Addr:   addr,
		DBName: db,
		User:   user,
		Pwd:    passwd,
	})
	if err != nil {
		return nil, errors.Wrap(err, "connect to mongodb")
	}

	return &Service{
		logger: logger,
		col:    client.GetCol("notes"),
	}, nil
}

// GetLatestPostID retrieves the latest post ID from the database.
func (s *Service) GetLatestPostID(ctx context.Context) (int, error) {
	var note model.TelegramNote
	err := s.col.FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.M{"post_id": -1})).
		Decode(&note)
	if err != nil {
		if errors.Is(err, mongolib.ErrNoDocuments) {
			return 0, nil
		}
		return 0, errors.Wrap(err, "get latest post id")
	}

	return note.PostID, nil
}

// FetchContent fetches the content of a post by its ID.
func (s *Service) FetchContent(ctx context.Context, postID int) error {
	url := fmt.Sprintf("https://t.me/laiskynotes/%d", postID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrap(err, "create request")
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "fetch content")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "read body")
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "parse html")
	}

	content := doc.Find("head > meta:nth-child(8)").AttrOr("content", "")
	if content == "" || strings.Contains(content, "Record and share interesting information.") {
		s.logger.Info("skip empty content", zap.Int("post_id", postID))
		return nil
	}

	note := model.TelegramNote{
		Content:   content,
		PostID:    postID,
		UpdatedAt: time.Now(),
	}
	changed := note.UpdateNote(content)
	if !changed {
		return nil
	}

	_, err = s.col.UpdateOne(ctx,
		bson.M{"post_id": postID},
		bson.M{
			"$set": note,
			"$setOnInsert": bson.M{
				"created_at": time.Now(),
			},
		},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return errors.Wrap(err, "update note")
	}

	s.logger.Info("updated content", zap.Int("post_id", postID))
	return nil
}
