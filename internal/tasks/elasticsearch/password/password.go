// Package password generate random password monthly
package password

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"time"

	"github.com/Laisky/go-ramjet/library/log"

	"github.com/Laisky/zap"
)

func GeneratePasswdByDate(now time.Time, secret string) string {
	log.Logger.Debug("GeneratePasswdByDate", zap.Time("now", now))
	h := sha1.New()
	var err error
	if _, err = io.WriteString(h, now.Format("200601")); err != nil {
		log.Logger.Error("write datestr", zap.Error(err))
	}
	if _, err = io.WriteString(h, secret); err != nil {
		log.Logger.Error("write secret", zap.Error(err))
	}
	return hex.EncodeToString(h.Sum(nil))[:15]
}
