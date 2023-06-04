// Package clickhouse implements clickhouse db.
package clickhouse

import (
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
)

func New(dsn string) (*gorm.DB, error) {
	//nolint:lll
	// dsn := fmt.Sprintf("tcp://%s?database=%s&username=%s&password=%s&read_timeout=10&write_timeout=20", addr, database, user, passwd)

	return gorm.Open(clickhouse.Open(dsn), &gorm.Config{})
}
