package crawler

import (
	"time"
)

type SearchText struct {
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
	Text      string    `gorm:"column:text" json:"text"`
	Title     string    `gorm:"column:title" json:"title"`
	URL       string    `gorm:"column:url" json:"url"`
}

func (SearchText) TableName() string {
	return "search_text"
}
