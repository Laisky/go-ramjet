package http

import (
	"time"

	"github.com/Laisky/errors/v2"
)

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

// AzureCreateImageResponse return from azure image api
type AzureCreateImageResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		RevisedPrompt string `json:"revised_prompt"`
		Url           string `json:"url"`
	} `json:"data"`
}

// DrawImageByTextRequest draw image by text and prompt
type DrawImageByTextRequest struct {
	Prompt string `binding:"required,min=1" json:"prompt"`
	Model  string `binding:"required,min=1" json:"model"`
	N      int    `json:"n"`
	Size   string `json:"size"`
}

// DrawImageByFluxSegmind is request to draw image by flux schnell
//
// https://www.segmind.com/models/flux-schnell/api
type DrawImageByFluxSegmind struct {
	// Prompt is the text prompt for generating the image
	Prompt string `binding:"required" json:"prompt"`

	// Steps is the number of inference steps for image generation
	// min: 1, max: 100
	Steps int `binding:"required,min=1,max=100" json:"steps"`

	// Seed is the seed for random number generation
	Seed int `json:"seed"`

	// SamplerName is the sampler for the image generation process
	SamplerName string `binding:"required" json:"sampler_name"`

	// Scheduler is the scheduler for the image generation process
	Scheduler string `binding:"required" json:"scheduler"`

	// Samples is the number of samples to generate
	Samples int `binding:"required" json:"samples"`

	// Width is the image width, can be between 512 and 2048 in multiples of 8
	Width int `binding:"required,min=512,max=2048" json:"width"`

	// Height is the image height, can be between 512 and 2048 in multiples of 8
	Height int `binding:"required,min=512,max=2048" json:"height"`

	// Denoise is the denoise level for the generated image
	Denoise float64 `binding:"required" json:"denoise"`
}

// DrawImageByFluxReplicateRequest draw image by fluxpro
//
// https://replicate.com/black-forest-labs/flux-pro?prediction=kg1krwsdf9rg80ch1sgsrgq7h8&output=json
type DrawImageByFluxReplicateRequest struct {
	Input FluxInput `json:"input"`
}

// FluxInput is input of DrawImageByFluxProRequest
//
// https://replicate.com/black-forest-labs/flux-1.1-pro/api/schema
type FluxInput struct {
	Steps  int    `binding:"required,min=1" json:"steps"`
	Prompt string `binding:"required,min=1" json:"prompt"`
	// ImagePrompt is the image prompt, only works for flux-1.1-pro
	ImagePrompt *string `json:"image_prompt,omitempty"`
	// InputImage is the input image, only works for flux-kontext-pro
	InputImage      *string `json:"input_image,omitempty"`
	Guidance        int     `binding:"required,min=2,max=5"                         json:"guidance"`
	Interval        int     `binding:"required,min=1,max=4"                         json:"interval"`
	AspectRatio     string  `binding:"required,oneof=1:1 16:9 2:3 3:2 4:5 5:4 9:16" json:"aspect_ratio"`
	SafetyTolerance int     `binding:"required,min=1,max=5"                         json:"safety_tolerance"`
	Seed            int     `json:"seed"`
	NImages         int     `binding:"required,min=1,max=8"                         json:"n_images"`
	Width           int     `binding:"required,min=256,max=1440"                    json:"width"`
	Height          int     `binding:"required,min=256,max=1440"                    json:"height"`
}

// InpaintingImageByFlusReplicateRequest is request to inpainting image by flux pro
//
// https://replicate.com/black-forest-labs/flux-fill-pro/api/schema
type InpaintingImageByFlusReplicateRequest struct {
	Input FluxInpaintingInput `json:"input"`
}

// FluxInpaintingInput is input of DrawImageByFluxProRequest
//
// https://replicate.com/black-forest-labs/flux-fill-pro/api/schema
type FluxInpaintingInput struct {
	Mask             string `binding:"required"             json:"mask"`
	Image            string `binding:"required"             json:"image"`
	Seed             int    `json:"seed"`
	Steps            int    `binding:"required,min=1"       json:"steps"`
	Prompt           string `binding:"required,min=5"       json:"prompt"`
	Guidance         int    `binding:"required,min=2,max=5" json:"guidance"`
	OutputFormat     string `json:"output_format"`
	SafetyTolerance  int    `binding:"required,min=1,max=5" json:"safety_tolerance"`
	PromptUnsampling bool   `json:"prompt_unsampling"`
}

// DrawImageByFluxProResponse is response of DrawImageByFluxProRequest
//
// https://replicate.com/black-forest-labs/flux-pro?prediction=kg1krwsdf9rg80ch1sgsrgq7h8&output=json
type DrawImageByFluxProResponse struct {
	CompletedAt time.Time                       `json:"completed_at"`
	CreatedAt   time.Time                       `json:"created_at"`
	DataRemoved bool                            `json:"data_removed"`
	Error       string                          `json:"error"`
	ID          string                          `json:"id"`
	Input       DrawImageByFluxReplicateRequest `json:"input"`
	Logs        string                          `json:"logs"`
	Metrics     FluxMetrics                     `json:"metrics"`
	// Output could be `string` or `[]string`
	Output    any       `json:"output"`
	StartedAt time.Time `json:"started_at"`
	Status    string    `json:"status"`
	URLs      FluxURLs  `json:"urls"`
	Version   string    `json:"version"`
}

// GetOutput return output
func (r *DrawImageByFluxProResponse) GetOutput() ([]string, error) {
	switch v := r.Output.(type) {
	case string:
		return []string{v}, nil
	case []string:
		return v, nil
	case nil:
		return nil, nil
	case []interface{}:
		// convert []interface{} to []string
		ret := make([]string, len(v))
		for idx, vv := range v {
			if vvv, ok := vv.(string); ok {
				ret[idx] = vvv
			} else {
				return nil, errors.Errorf("unknown output type: [%T]%v", vv, vv)
			}
		}

		return ret, nil
	default:
		return nil, errors.Errorf("unknown output type: [%T]%v", r.Output, r.Output)
	}
}

// FluxMetrics is metrics of DrawImageByFluxProResponse
type FluxMetrics struct {
	ImageCount  int     `json:"image_count"`
	PredictTime float64 `json:"predict_time"`
	TotalTime   float64 `json:"total_time"`
}

// FluxURLs is urls of DrawImageByFluxProResponse
type FluxURLs struct {
	Get    string `json:"get"`
	Cancel string `json:"cancel"`
}

// DrawImageByImageRequest draw image by image and prompt
type DrawImageByImageRequest struct {
	Prompt      string `binding:"required,min=1" json:"prompt"`
	Model       string `binding:"required,min=1" json:"model"`
	ImageBase64 string `binding:"required,min=1" json:"image_base64"`
}

// DrawImageByLcmRequest draw image by image and prompt with lcm
type DrawImageByLcmRequest struct {
	// Data consist of 6 strings:
	//  1. prompt,
	//  2. base64 encoded image with fixed prefix "data:image/png;base64,"
	//  3. steps
	//  4. cfg
	//  5. sketch strength
	//  6. seed
	Data    [6]any `json:"data"`
	FnIndex int    `json:"fn_index"`
}

// DrawImageBySdxlturboRequest draw image by image and prompt with sdxlturbo
type DrawImageBySdxlturboRequest struct {
	Model string `binding:"required,min=1" json:"model"`
	// Text prompt
	Text           string `binding:"required,min=1" json:"text"`
	NegativePrompt string `json:"negative_prompt"`
	ImageB64       string `json:"image"`
	// N how many images to generate
	N int `json:"n"`
}

// DrawImageBySdxlturboResponse draw image by image and prompt with sdxlturbo
type DrawImageBySdxlturboResponse struct {
	B64Images []string `json:"images"`
}

// NvidiaTextPrompt text prompt
type NvidiaTextPrompt struct {
	Text string `json:"text"`
}

// NvidiaDrawImageBySdxlturboRequest draw image by image and prompt with sdxlturbo
//
// https://build.nvidia.com/explore/discover?snippet_tab=Python#sdxl-turbo
type NvidiaDrawImageBySdxlturboRequest struct {
	TextPrompts []NvidiaTextPrompt `json:"text_prompts"`
	Seed        int                `json:"seed"`
	Sampler     string             `json:"sampler"`
	Steps       int                `json:"steps"`
}

// NewNvidiaDrawImageBySdxlturboRequest create new request
func NewNvidiaDrawImageBySdxlturboRequest(prompt string) NvidiaDrawImageBySdxlturboRequest {
	return NvidiaDrawImageBySdxlturboRequest{
		TextPrompts: []NvidiaTextPrompt{
			{Text: prompt},
		},
		Seed:    int(time.Now().UnixNano()) % 4294967296,
		Sampler: "K_EULER_ANCESTRAL",
		Steps:   4,
	}
}

// NvidiaDrawImageBySdxlturboResponse draw image by image and prompt with sdxlturbo
type NvidiaDrawImageBySdxlturboResponse struct {
	Artifacts []NvidiaArtifact `json:"artifacts"`
}

// NvidiaArtifact draw image artifact
type NvidiaArtifact struct {
	Base64       string `json:"base64"`
	FinishReason string `json:"finish_reason"`
	Seed         int    `json:"seed"`
}

// DrawImageByLcmResponse draw image by image and prompt with lcm
type DrawImageByLcmResponse struct {
	// Data base64 encoded image with fixed prefix "data:image/png;base64,"
	Data            []string `json:"data"`
	IsGenerating    bool     `json:"is_generating"`
	Duration        float64  `json:"duration"`
	AverageDuration float64  `json:"average_duration"`
}
