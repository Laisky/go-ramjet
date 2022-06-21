package crawler

import (
	"strings"
	"time"

	"github.com/pkg/errors"
	"gorm.io/gorm"

	"github.com/Laisky/go-ramjet/library/db/clickhouse"
)

type Dao struct {
	DB *gorm.DB
}

func NewDao(dsn string) (*Dao, error) {
	db, err := clickhouse.New(dsn)
	if err != nil {
		return nil, err
	}

	return &Dao{DB: db}, nil
}

func (d *Dao) RemoveLegacy(olderThan time.Time) error {
	return d.DB.Where("updated_at < ?", olderThan).
		Delete(SearchText{}).Error
}

type SearchResult struct {
	Context string `json:"context"`
	URL     string `json:"url"`
	Title   string `json:"title"`
}

func (d *Dao) Search(text string) (rets []SearchResult, err error) {
	rets = []SearchResult{}
	if err := d.DB.Model(SearchText{}).
		Where("lowerUTF8(text) LIKE ?", "%"+strings.ToLower(text)+"%").
		Select(`url, title`).
		Scan(&rets).Error; err != nil {
		return nil, err
	}

	return rets, nil
}

func (d *Dao) Save(title, text, url string) error {
	if err := d.DB.FirstOrCreate(&SearchText{},
		SearchText{
			URL: url,
		},
	).Error; err != nil {
		return errors.Wrap(err, "first or create")
	}

	if err := d.DB.Model(SearchText{}).
		Where("url = ?", url).
		Updates(SearchText{
			Title: title,
			Text:  text,
		}).Error; err != nil {
		return errors.Wrap(err, "save")
	}

	return nil
}
