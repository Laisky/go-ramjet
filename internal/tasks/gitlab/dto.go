package gitlab

type GetFileResponse struct {
	Content  string `json:"content"`
	LineFrom uint   `json:"line_from"`
}
