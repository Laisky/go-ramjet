package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/laisky-blog-graphql/library/db/mongo"
	mongosdk "go.mongodb.org/mongo-driver/mongo"
)

var (
	javDB mongo.DB
)

// SetupDB setup db
func SetupDB(ctx context.Context) (err error) {
	javDB, err = mongo.NewDB(ctx, mongo.DialInfo{
		Addr:   gconfig.S.GetString("db.jav.addr"),
		DBName: gconfig.S.GetString("db.jav.db"),
		User:   gconfig.S.GetString("db.jav.user"),
		Pwd:    gconfig.S.GetString("db.jav.passwd"),
	})
	if err != nil {
		return errors.Wrap(err, "new db")
	}

	return nil
}

// SetDB installs db as the package-level jav database handle.
//
// SetupDB is the normal way to connect; SetDB lets callers (and tests that use
// a mock deployment) provide an already constructed handle.
//
// Parameters:
//   - db: the database handle every Get*Col accessor resolves collections from.
func SetDB(db mongo.DB) {
	javDB = db
}

// GetDB get db
func GetDB() mongo.DB {
	return javDB
}

// GetColActress get actress collection
func GetColActress() *mongosdk.Collection {
	return GetDB().GetCol("actress")
}

// GetColMovie get movie collection
func GetColMovie() *mongosdk.Collection {
	return GetDB().GetCol("movies")
}

// GetColFulltext get fulltext collection
func GetColFulltext() *mongosdk.Collection {
	return GetDB().GetCol("fulltext")
}
