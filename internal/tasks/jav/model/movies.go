package model

import "go.mongodb.org/mongo-driver/bson/primitive"

type Movie struct {
	ID          primitive.ObjectID   `bson:"_id,omitempty" json:"mongo_id"`
	Actresses   []primitive.ObjectID `bson:"actresses" json:"actresses"`
	Description string               `bson:"description" json:"description"`
	ImgUrls     []string             `bson:"img_urls" json:"img_urls"`
	Name        string               `bson:"name" json:"name"`
	Tags        []string             `bson:"tags" json:"tags"`
}
