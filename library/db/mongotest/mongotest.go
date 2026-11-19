// Package mongotest provides test helpers that let packages built on the
// laisky-blog-graphql mongo.DB interface run against any *mongo.Client, such as
// the mock deployment of go.mongodb.org/mongo-driver/mongo/integration/mtest.
//
// It lets tests capture the exact command (filter, options) a handler sends to
// MongoDB without a running server.
package mongotest

import (
	"context"

	blogmongo "github.com/Laisky/laisky-blog-graphql/library/db/mongo"
	"go.mongodb.org/mongo-driver/mongo"
)

// DB adapts a *mongo.Client to the laisky-blog-graphql mongo.DB interface.
type DB struct {
	// Client is the driver client every collection is resolved from.
	Client *mongo.Client
	// Name is the database returned by CurrentDB and used by GetCol.
	Name string
}

var _ blogmongo.DB = (*DB)(nil)

// New returns a DB that resolves collections from client inside database name.
//
// Parameters:
//   - client: driver client, typically mtest.T.Client of a mock deployment.
//   - name: default database name.
//
// Returns:
//   - *DB: adapter implementing the laisky-blog-graphql mongo.DB interface.
func New(client *mongo.Client, name string) *DB {
	return &DB{Client: client, Name: name}
}

// Close is a no-op because the caller owns the client lifecycle.
//
// Parameters:
//   - ctx: unused.
//
// Returns:
//   - error: always nil.
func (d *DB) Close(_ context.Context) error {
	return nil
}

// GetCol returns collection colName of the default database.
//
// Parameters:
//   - colName: collection name.
//
// Returns:
//   - *mongo.Collection: collection handle bound to the client.
func (d *DB) GetCol(colName string) *mongo.Collection {
	return d.CurrentDB().Collection(colName)
}

// DB returns database name from the wrapped client.
//
// Parameters:
//   - name: database name.
//
// Returns:
//   - *mongo.Database: database handle bound to the client.
func (d *DB) DB(name string) *mongo.Database {
	return d.Client.Database(name)
}

// CurrentDB returns the default database.
//
// Returns:
//   - *mongo.Database: database handle for d.Name.
func (d *DB) CurrentDB() *mongo.Database {
	return d.Client.Database(d.Name)
}
