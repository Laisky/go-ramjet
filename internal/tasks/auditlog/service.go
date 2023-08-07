package auditlog

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/go-utils/v4/json"
	glog "github.com/Laisky/go-utils/v4/log"
	auditProto "github.com/Laisky/protocols/proto/auditlog/v1"
	"github.com/Laisky/zap"
	"go.mongodb.org/mongo-driver/bson"
	mongoLib "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/protobuf/proto"

	"github.com/Laisky/go-ramjet/library/log"
)

var (
	httpcli *http.Client
)

func init() {
	var err error
	if httpcli, err = gutils.NewHTTPClient(); err != nil {
		log.Logger.Panic("new http client", zap.Error(err))
	}
}

type service struct {
	logger      glog.Logger
	db          *AuditDB
	rootcaPool  *x509.CertPool
	alertPusher *glog.Alert
}

// newService new auditlog service
func newService(logger glog.Logger,
	db *AuditDB,
	rootcaPool *x509.CertPool,
	alertPusher *glog.Alert,
) (*service, error) {
	return &service{
		logger:      logger,
		db:          db,
		rootcaPool:  rootcaPool,
		alertPusher: alertPusher,
	}, nil
}

// SaveLog save log to db
func (s *service) SaveLog(ctx context.Context, logEnt *Log) (err error) {
	if err = logEnt.ValidFormat(); err != nil {
		return errors.Wrap(err, "invalid log")
	}

	logEnt.Verified = false
	if s.rootcaPool != nil {
		if err = logEnt.ValidRootCA(s.rootcaPool); err == nil {
			logEnt.Verified = true
		}
	}

	if _, err = s.db.logCol().InsertOne(ctx, logEnt); err != nil {
		return errors.Wrap(err, "insert log")
	}

	s.logger.Debug("save log", zap.String("log", logEnt.UUID))
	return nil
}

// ListLogs list all logs
func (s *service) ListLogs(ctx context.Context) ([]Log, error) {
	logs := make([]Log, 0)
	cur, err := s.db.logCol().Find(ctx, bson.M{},
		options.Find().SetLimit(500),
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

type clusterFingerprintTask struct {
	LastVersion uint32 `json:"last_version"`
}

func (s *service) checkClunterFingerprint(ctx context.Context, furl string) error {
	logger := s.logger.Named("cluster_fingerprint")
	logger.Debug("run", zap.String("url", furl))

	// download cluster fingerprint file
	req, err := http.NewRequestWithContext(ctx, "GET", furl, nil)
	if err != nil {
		return errors.Wrapf(err, "download cluster fingerprint from %s", furl)
	}

	resp, err := httpcli.Do(req)
	if err != nil {
		return errors.Wrapf(err, "download cluster fingerprint from %s", furl)
	}
	defer gutils.LogErr(resp.Body.Close, s.logger)

	filecnt, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "download cluster fingerprint from %s", furl)
	}

	data := new(auditProto.ClusterFingerprint)
	if err = proto.Unmarshal(filecnt, data); err != nil {
		return errors.Wrapf(err, "download cluster fingerprint from %s", furl)
	}

	// load latest saved version
	task := new(Task)
	taskData := new(clusterFingerprintTask)
	if err = s.db.taskCol().FindOne(ctx,
		bson.M{"type": string(TaskTypeClusterFingerprint)}).
		Decode(task); err != nil {
		if err == mongoLib.ErrNoDocuments {
			task = &Task{
				Type: TaskTypeClusterFingerprint,
			}
		} else {
			return errors.Wrap(err, "find task")
		}
	} else if err = json.UnmarshalFromString(task.Data, taskData); err != nil {
		return errors.Wrap(err, "unmarshal task data")
	}

	// check version must be monotonically increasing
	if taskData.LastVersion > data.Version {
		errMsg := fmt.Sprintf(
			"[fingerprint check] cluster fingerprint version %d < last version %d",
			data.Version, taskData.LastVersion)
		if err = s.alertPusher.Send(errMsg); err != nil {
			logger.Error("send alert", zap.Error(err))
		}

		return errors.Errorf(errMsg)
	}

	// save task
	taskData.LastVersion = data.Version
	if task.Data, err = json.MarshalToString(taskData); err != nil {
		return errors.Wrap(err, "marshal task data")
	}

	if _, err = s.db.taskCol().UpdateOne(ctx,
		bson.M{"type": string(TaskTypeClusterFingerprint)},
		bson.M{"$set": task},
		options.Update().SetUpsert(true),
	); err != nil {
		return errors.Wrap(err, "update task")
	}

	logger.Debug("succeed check cluster fingerprint",
		zap.Uint32("version", data.Version),
	)
	return nil
}
