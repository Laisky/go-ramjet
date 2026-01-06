package http

import (
	"math"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/library/log"
)

// VisionTokenPrice vision token price($/500000)
const VisionTokenPrice = 5000

// CountVisionImagePrice count vision image tokens
//
// https://openai.com/pricing
func CountVisionImagePrice(width int, height int, resolution VisionImageResolution) (int, error) {
	switch resolution {
	case VisionImageResolutionLow:
		return 85, nil // fixed price
	case VisionImageResolutionHigh:
		// Pricing uses 768-tile units per OpenAI's latest docs.
		// Base 85 and 170 per tile count.
		h := math.Ceil(float64(height) / 768)
		w := math.Ceil(float64(width) / 768)
		n := w * h
		total := 85 + 170*n
		return int(total) * VisionTokenPrice, nil
	default:
		return 0, errors.Errorf("unsupport resolution %q", resolution)
	}
}

func imageType(cnt []byte) string {
	contentType := http.DetectContentType(cnt)
	if strings.HasPrefix(contentType, "image/") {
		return contentType
	}

	log.Logger.Warn("unsupport image content type", zap.String("type", contentType))
	return "image/jpeg"
}

// func imageSize(cnt []byte) (width, height int, err error) {
// 	contentType := http.DetectContentType(cnt)
// 	switch contentType {
// 	case "image/jpeg", "image/jpg":
// 		img, err := jpeg.Decode(bytes.NewReader(cnt))
// 		if err != nil {
// 			return 0, 0, errors.Wrap(err, "decode jpeg")
// 		}

// 		bounds := img.Bounds()
// 		return bounds.Dx(), bounds.Dy(), nil
// 	case "image/png":
// 		img, err := png.Decode(bytes.NewReader(cnt))
// 		if err != nil {
// 			return 0, 0, errors.Wrap(err, "decode png")
// 		}

// 		bounds := img.Bounds()
// 		return bounds.Dx(), bounds.Dy(), nil
// 	default:
// 		return 0, 0, errors.Errorf("unsupport image content type %q", contentType)
// 	}
// }

// var (
// 	// hdResolutionMarker enable hd resolution for gpt-4-vision only
// 	// if user has permission and mention "hd" in prompt
// 	hdResolutionMarker = regexp.MustCompile(`\b@hd\b`)
// )

// processVisionRequest process vision request
// func processVisionRequest(user *config.UserConfig, frontendReq *FrontendReq) (*OpenaiChatReq[[]OpenaiVisionMessageContent], error) {
// 	req := new(OpenaiChatReq[[]OpenaiVisionMessageContent])
// 	if err := copier.Copy(req, frontendReq); err != nil {
// 		return nil, errors.Wrap(err, "copy to chat req")
// 	}

// 	// Convert all messages from frontend request to vision format
// 	req.Messages = make([]OpenaiReqMessage[[]OpenaiVisionMessageContent], 0, len(frontendReq.Messages))

// 	var nImages int
// 	for _, msg := range frontendReq.Messages {
// 		// Create a new message with the same role
// 		visionMsg := OpenaiReqMessage[[]OpenaiVisionMessageContent]{
// 			Role:    msg.Role,
// 			Content: []OpenaiVisionMessageContent{},
// 		}

// 		// Add text content if present
// 		if len(msg.Content.ArrayContent) > 0 {
// 			visionMsg.Content = append(visionMsg.Content, msg.Content.ArrayContent...)
// 		} else if msg.Content.StringContent != "" {
// 			visionMsg.Content = append(visionMsg.Content, OpenaiVisionMessageContent{
// 				Type: OpenaiVisionMessageContentTypeText,
// 				Text: msg.Content.StringContent,
// 			})
// 		}

// 		// Add image content if present
// 		totalFileSize := 0
// 		for _, f := range msg.Files {
// 			nImages += 1
// 			resolution := VisionImageResolutionLow
// 			// if user has permission and image size is large than 1MB,
// 			// use high resolution
// 			if (user.BYOK || user.NoLimitExpensiveModels) && hdResolutionMarker.MatchString(msg.Content.String()) {
// 				resolution = VisionImageResolutionHigh
// 			}

// 			visionMsg.Content = append(visionMsg.Content, OpenaiVisionMessageContent{
// 				Type: OpenaiVisionMessageContentTypeImageUrl,
// 				ImageUrl: &OpenaiVisionMessageContentImageUrl{
// 					URL: fmt.Sprintf("data:%s;base64,", imageType(f.Content)) +
// 						base64.StdEncoding.EncodeToString(f.Content),
// 					Detail: resolution,
// 				},
// 			})

// 			if user.IsFree {
// 				if nImages >= 2 {
// 					break // only support 6 images per message for cost saving
// 				}
// 			}

// 			totalFileSize += len(f.Content)
// 			if totalFileSize > 10*1024*1024 {
// 				return nil, errors.Errorf("total file size should be less than 10MB, got %d", totalFileSize)
// 			}
// 		}

// 		// If a system message has no content, skip it
// 		if msg.Role == OpenaiMessageRoleSystem && len(visionMsg.Content) == 0 {
// 			continue
// 		}

// 		// For empty user or AI messages, add an empty text content
// 		// This handles cases where a message might only have images
// 		if len(visionMsg.Content) == 0 {
// 			visionMsg.Content = append(visionMsg.Content, OpenaiVisionMessageContent{
// 				Type: OpenaiVisionMessageContentTypeText,
// 				Text: "",
// 			})
// 		}

// 		req.Messages = append(req.Messages, visionMsg)
// 	}

// 	// Ensure we have at least one message
// 	if len(req.Messages) == 0 {
// 		return nil, errors.New("no valid messages after processing")
// 	}

// 	return req, nil
// }
