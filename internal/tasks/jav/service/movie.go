// Package service is a service package for jav tasks
package service

import (
	"context"

	"github.com/Laisky/errors/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/Laisky/go-ramjet/internal/tasks/jav/dto"
	"github.com/Laisky/go-ramjet/internal/tasks/jav/model"
)

// GetMovieInfo is a service to get movie info
func GetMovieInfo(ctx context.Context, movieID primitive.ObjectID) (*dto.MovieResponse, error) {
	// get movie from db
	movie := new(model.Movie)
	err := model.GetDB().
		GetCol("movies").
		FindOne(ctx, bson.M{"_id": movieID}).
		Decode(movie)
	if err != nil {
		return nil, errors.Wrap(err, "get movie info")
	}

	// get actress from db
	actress := new(model.Actress)
	err = model.GetDB().
		GetCol("actress").
		FindOne(ctx, bson.M{"_id": movie.ActressID}).
		Decode(actress)
	if err != nil {
		return nil, errors.Wrap(err, "get actress info")
	}

	return &dto.MovieResponse{
		Code:       movie.Name,
		PictureURL: movie.ImgUrl,
		Tags:       movie.Tags,
		Actress:    actress,
	}, nil
}
