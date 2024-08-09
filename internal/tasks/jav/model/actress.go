package model

import "go.mongodb.org/mongo-driver/bson/primitive"

type Actress struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"mongo_id"`
	Name       string             `bson:"name" json:"name"`
	OtherNames []string           `bson:"other_names" json:"other_names"`
}
