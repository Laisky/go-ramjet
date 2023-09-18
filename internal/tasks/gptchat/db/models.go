// package db is a package for database
package db

import "go.mongodb.org/mongo-driver/bson/primitive"

// OpenaiMessage message from openai
type OpenaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenaiConservation save each conservation of openai
type OpenaiConservation struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Model      string             `bson:"model" json:"model"`
	MaxTokens  uint               `bson:"max_tokens" json:"max_tokens"`
	Prompt     []OpenaiMessage    `bson:"prompt" json:"prompt"`
	Completion string             `bson:"completion" json:"completion"`
}
