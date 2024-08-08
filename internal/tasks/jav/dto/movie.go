// Package dto is a package for jav tasks
package dto

import "github.com/Laisky/go-ramjet/internal/tasks/jav/model"

type MovieResponse struct {
	Code       string         `json:"code"`
	PictureURL string         `json:"picture_url"`
	Tags       []string       `json:"tags"`
	Actress    *model.Actress `json:"actress"`
}
