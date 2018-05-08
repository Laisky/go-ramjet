package utils

import (
	"errors"

	"github.com/astaxie/beego"
	log "github.com/cihub/seelog"
	jwt "github.com/dgrijalva/jwt-go"
)

var secret = []byte(beego.AppConfig.String("secret_key"))

// GenerateToken 生成 JWT token
func GenerateToken(payload map[string]interface{}) (string, error) {
	jwtPayload := jwt.MapClaims{}
	for k, v := range payload {
		jwtPayload[k] = v
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtPayload)
	tokenStr, err := token.SignedString(secret)
	if err != nil {
		return "", err
	}
	return tokenStr, nil
}

// ValidateToken 校验 token 是否合法
func ValidateToken(tokenStr string, payload map[string]interface{}) (bool, error) {
	log.Debugf("ValidateToken for token %v", tokenStr)

	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return false, errors.New("JWT method not allowd")
		}
		return secret, nil
	})
	if err != nil {
		log.Debug("token validate error for ", err.Error())
		return false, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		for k, v := range claims {
			payload[k] = v
		}
		if _, ok := payload["username"]; !ok {
			return false, nil
		}
		log.Debug("token validated for username ", payload["username"])
		return true, nil
	}
	return false, nil
}
