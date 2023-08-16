package twitter

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Media struct {
	ID  int64  `bson:"id" json:"id"`
	URL string `bson:"media_url_https" json:"media_url_https"`
}

type Entities struct {
	Media []*Media `bson:"media" json:"media"`
}

type Tweet struct {
	MongoID primitive.ObjectID `bson:"_id,omitempty" json:"mongo_id,omitempty"`
	ID      string             `bson:"id_str" json:"id"`
	// CreatedAt       *time.Time         `bson:"created_at" json:"created_at"`
	Text string `bson:"text" json:"text"`
	// Topics          []string           `bson:"topics" json:"topics"`
	// User            *User              `bson:"user" json:"user"`
	// ReplyToStatusID string             `bson:"in_reply_to_status_id_str" json:"in_reply_to_status_id"`
	// Entities        *Entities          `bson:"entities" json:"entities"`
	// IsRetweeted     bool               `bson:"retweeted" json:"is_retweeted"`
	// RetweetedTweet  *Tweet             `bson:"retweeted_status,omitempty" json:"retweeted_tweet"`
	// IsQuoted        bool               `bson:"is_quote_status" json:"is_quote_status"`
	// QuotedTweet     *Tweet             `bson:"quoted_status,omitempty" json:"quoted_status"`
	// Viewer          []int64            `bson:"viewer,omitempty" json:"viewer"`
}

type User struct {
	ID         string `bson:"id_str" json:"id"`
	ScreenName string `bson:"screen_name" json:"screen_name"`
	Name       string `bson:"name" json:"name"`
	Dscription string `bson:"dscription" json:"dscription"`
}

type ClickhouseTweet struct {
	TweetID   string     `gorm:"column:tweet_id" json:"tweet_id"`
	Text      string     `gorm:"column:text" json:"text"`
	UserID    string     `gorm:"column:user_id" json:"user_id"`
	CreatedAt *time.Time `gorm:"column:created_at" json:"created_at"`
}

func (ClickhouseTweet) TableName() string {
	return "tweets"
}
