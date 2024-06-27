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

// Price how many quotes for 1 usd
type Price int

// Int return int value
func (p Price) Int() int {
	return int(p)
}

// USD100 return how many usd in cents
func (p Price) USDCents() int {
	return p.Int() / 5000
}

const (
	// PriceTxt2Image how many quotes for txt2image
	PriceTxt2Image  Price = 20000      // 0.04 usd
	PriceUploadFile Price = 2500       // 0.005 usd
	PriceTTS        Price = 20         // 0.00004 usd
	PriceUSD        Price = 500000     // 1 usd
	PriceRMB        Price = 500000 / 8 // 1 rmb
)

// BillingType billing type
type BillingType string

const (
	BillTypeTxt2Image BillingType = "txt2image"
)

// Billing billing for user
type Billing struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	BillingType BillingType        `bson:"type" json:"type"`
	Username    string             `bson:"username" json:"username"`
	// UsedQuota how many quotes used totally, 1usd = 500000 quotes
	UsedQuota Price `bson:"used_quota" json:"used_quota"`
}
