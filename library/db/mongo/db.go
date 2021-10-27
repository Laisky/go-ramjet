package mongo

import (
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
)

type DB struct {
	S  *mgo.Session
	DB *mgo.Database
}

func (d *DB) Dial(dialInfo *mgo.DialInfo) error {
	s, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		return errors.Wrap(err, "can not connect to db")
	}

	d.S = s
	return nil
}

func (d *DB) Close() {
	d.S.Close()
}
