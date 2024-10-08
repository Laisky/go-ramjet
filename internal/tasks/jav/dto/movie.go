// Package dto is a package for jav tasks
package dto

type MovieResponse struct {
	Code         string   `json:"code"`
	ImageURLs    []string `json:"image_urls"`
	Tags         []string `json:"tags"`
	Descriptions []string `json:"descriptions"`
	Actresses    []string `json:"actresses"`
	Downloads    []string `json:"downloads"`
}
