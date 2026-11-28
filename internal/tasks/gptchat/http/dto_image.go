package http

// OpenaiCreateImageRequest request to openai image api
type OpenaiCreateImageRequest struct {
	Model          string `json:"model,omitempty"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n"`
	Size           string `json:"size"`
	Quality        string `json:"quality,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	Style          string `json:"style,omitempty"`
}

// NewOpenaiCreateImageRequest create new request
func NewOpenaiCreateImageRequest(prompt string, n int) *OpenaiCreateImageRequest {
	return &OpenaiCreateImageRequest{
		Model:  "dall-e-3",
		Prompt: prompt,
		N:      n,
		Size:   "1024x1024",
		// Quality:        "hd",  // price double
		ResponseFormat: "b64_json",
	}
}

// OpenaiCreateImageEditRequest request to openai image edit api
type OpenaiCreateImageEditRequest struct {
	Model          string `json:"model,omitempty"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

// OpenaiCreateImageResponse return from openai image api
type OpenaiCreateImageResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		Url     string `json:"url"`
		B64Json string `json:"b64_json"`
	} `json:"data"`
}

// DrawImageByTextRequest draw image by text and prompt
type DrawImageByTextRequest struct {
	Prompt string `binding:"required,min=1" json:"prompt"`
	Model  string `binding:"required,min=1" json:"model"`
	N      int    `json:"n"`
	Size   string `json:"size"`
}
