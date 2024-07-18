package dns

import "time"

// CreateRecordRequest is a request to create record
type CreateRecordRequest struct {
	Name   string `json:"name"`
	FileID string `json:"file_id"`
	Owner  *owner `json:"owner,omitempty"`
}

// Record is each file stored in s3
type Record struct {
	Records []recordItem `json:"records"`
}

type recordItem struct {
	Name    string        `json:"name"`
	FileID  string        `json:"file_id"`
	Owner   *owner        `json:"owner,omitempty"`
	History []historyItem `json:"history,omitempty"`
}

type owner struct {
	TelegramUID int `json:"telegram_uid"`
}

type historyItem struct {
	Time   time.Time `json:"time"`
	FileID string    `json:"file_id"`
}
