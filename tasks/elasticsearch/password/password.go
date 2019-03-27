// Package password generate random password monthly
package password

import (
	"crypto/sha1"
	"encoding/base64"
	"io"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
)

func GeneratePasswdByDate(now time.Time, secret string) string {
	utils.Logger.Debug("GeneratePasswdByDate", zap.Time("now", now))
	h := sha1.New()
	io.WriteString(h, now.Format("200601"))
	io.WriteString(h, secret)
	return base64.URLEncoding.EncodeToString(h.Sum(nil))[:15]
}
