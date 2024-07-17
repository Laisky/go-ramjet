package dns

// CreateRecordRequest is a request to create record
type CreateRecordRequest struct {
	Name   string `json:"name"`
	FileID string `json:"file_id"`
	Owner  *owner `json:"owner"`
}

// Record is each file stored in s3
type Record struct {
	Records []recordItem `json:"records"`
}

type recordItem struct {
	Name   string `json:"name"`
	FileID string `json:"file_id"`
	Owner  *owner `json:"owner"`
}

type owner struct {
	TelegramUID int `json:"telegram_uid"`
}
