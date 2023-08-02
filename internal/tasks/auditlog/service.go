package auditlog

import (
	"context"
	"crypto/x509"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v4/log"
	"github.com/Laisky/zap"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type service struct {
	logger     glog.Logger
	db         *AuditDB
	rootcaPool *x509.CertPool
}

// newService new auditlog service
func newService(logger glog.Logger, db *AuditDB, rootcaPool *x509.CertPool) (*service, error) {
	return &service{
		logger:     logger,
		db:         db,
		rootcaPool: rootcaPool,
	}, nil
}

// SaveLog save log to db
func (s *service) SaveLog(ctx context.Context, log *Log) (err error) {
	if err = log.Valid(s.rootcaPool); err != nil {
		return errors.Wrap(err, "invalid log")
	}

	if _, err = s.db.logCol().InsertOne(ctx, log); err != nil {
		return errors.Wrap(err, "insert log")
	}

	s.logger.Debug("save log", zap.String("log", log.UUID))
	return nil
}

// ListLogs list all logs
func (s *service) ListLogs(ctx context.Context) ([]Log, error) {
	logs := make([]Log, 0)
	cur, err := s.db.logCol().Find(ctx, bson.M{},
		options.Find().SetSort(map[string]int{"_id": -1}),
	)
	if err != nil {
		return nil, errors.Wrap(err, "find logs")
	}

	if err = cur.All(ctx, &logs); err != nil {
		return nil, errors.Wrap(err, "get logs")
	}

	return logs, nil
}
