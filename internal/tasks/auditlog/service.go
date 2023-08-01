package auditlog

import (
	"context"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v4/log"
	"github.com/Laisky/zap"
)

type Service struct {
	logger glog.Logger
	db     *AuditDB
}

func NewService(logger glog.Logger, db *AuditDB) (*Service, error) {
	return &Service{
		logger: logger,
		db:     db,
	}, nil
}

func (s *Service) SaveLog(ctx context.Context, log *Log) error {
	if _, err := s.db.logCol().InsertOne(ctx, log); err != nil {
		return errors.Wrap(err, "insert log")
	}

	s.logger.Debug("save log", zap.String("log", log.UUID))
	return nil
}
