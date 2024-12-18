// Package singleton implements global variables.
package singleton

import (
	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gjwt "github.com/Laisky/go-utils/v5/jwt"
	"github.com/Laisky/laisky-blog-graphql/library/auth"
)

var Jwt gjwt.JWT

func Setup() error {
	if err := setupJwt(); err != nil {
		return errors.Wrap(err, "setup jwt")
	}

	if err := auth.Initialize([]byte(gconfig.Shared.GetString("server.jwt_secret"))); err != nil {
		return errors.Wrap(err, "initialize jwt auth")
	}

	return nil
}

func setupJwt() (err error) {
	Jwt, err = gjwt.New(
		gjwt.WithSignMethod(gjwt.SignMethodHS256),
		gjwt.WithSecretByte([]byte(gconfig.Shared.GetString("server.jwt_secret"))),
	)
	if err != nil {
		return errors.Wrap(err, "new jwt")
	}

	return nil
}
