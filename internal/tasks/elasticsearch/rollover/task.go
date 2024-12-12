package rollover

import (
	"context"
	"html/template"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/go-utils/v5/json"
	"github.com/Laisky/zap"
	"golang.org/x/sync/semaphore"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

var (
	httpClient = http.Client{
		Timeout: time.Second * 30,
	}
)

// IdxSetting is the task settings
type IdxSetting struct {
	Regexp        *regexp.Regexp
	Rollover      string
	Expires       time.Duration
	IdxAlias      string
	NRepls        int
	NShards       int
	IdxWriteAlias string
	Mapping       template.HTML
	API           string
	IsSkipCreate  bool
}

// BindRolloverIndices bind the task to rollover indices
func BindRolloverIndices() {
	log.Logger.Info("bind rollover indices...")

	if gconfig.Shared.GetBool("debug") { // set for debug
		gconfig.Shared.Set("tasks.elasticsearch-v2.interval", 10)
	}

	bindHTTP()
	go store.TaskStore.TickerAfterRun(
		gconfig.Shared.GetDuration("tasks.elasticsearch-v2.interval")*time.Second,
		runTask)
}

func runTask() {
	ctx := context.Background()
	sem := semaphore.NewWeighted(
		gconfig.Shared.GetInt64("tasks.elasticsearch-v2.concurrent"))

	taskSts, err := LoadSettings()
	if err != nil {
		log.Logger.Error("cannot load elasticsearch rollover settings", zap.Error(err))
		return
	}

	for _, st := range taskSts {
		go RunDeleteTask(ctx, sem, st)
		if !st.IsSkipCreate {
			go RunRolloverTask(ctx, sem, st)
		}
	}
}

func urlMasking(val string) string {
	return gutils.URLMasking(val, "*****")
}

// LoadAllIndicesNames load all indices name by ES API
func LoadAllIndicesNames(api string) (indices []string, err error) {
	log.Logger.Info("load indices by api", zap.String("api", urlMasking(api)))
	var (
		url     = api + "_cat/indices/?h=index&format=json"
		idxList = []map[string]string{}
		idxItm  map[string]string
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "new http request error for url %v", urlMasking(url))
	}

	resp, err := httpClient.Do(req) //nolint: bodyclose
	if err != nil {
		return nil, errors.Wrapf(err, "http get error for url %v", urlMasking(url))
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger) // nolint: errcheck,gosec

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "load body error for url %v", urlMasking(url))
	}
	err = json.Unmarshal(respBytes, &idxList)
	if err != nil {
		return nil, errors.Wrapf(err, "parse json body for url %v error %v", urlMasking(url), string(respBytes[:]))
	}

	for _, idxItm = range idxList {
		indices = append(indices, idxItm["index"])
	}

	return indices, nil
}

// LoadSettings load task settings
func LoadSettings() (idxSettings []*IdxSetting, err error) {
	cfgi, ok := gconfig.Shared.Get("tasks.elasticsearch-v2.configs").([]interface{})
	if !ok {
		return nil, errors.New("invalid config")
	}

	for _, itemI := range cfgi {
		item, ok := itemI.(map[interface{}]interface{})
		if !ok {
			return nil, errors.New("invalid config")
		}

		action, ok := item["action"].(string)
		if !ok {
			return nil, errors.New("invalid config for `action`")
		}
		if action != "rollover" {
			continue
		}

		//nolint: forcetypeassert
		idx := &IdxSetting{
			Regexp:        regexp.MustCompile(item["index"].(string)),
			Expires:       time.Duration(item["expires"].(int)) * time.Second,
			IdxAlias:      item["index-alias"].(string),
			IdxWriteAlias: item["index-write-alias"].(string),
			Mapping:       getESMapping(item["mapping"].(string)),
			API:           item["api"].(string),
			Rollover:      item["rollover"].(string),
			NRepls:        gutils.FallBack(func() interface{} { return item["n-replicas"].(int) }, 1).(int),
			NShards:       gutils.FallBack(func() interface{} { return item["n-shards"].(int) }, 5).(int),
			IsSkipCreate:  gutils.FallBack(func() interface{} { return item["skip-create"].(bool) }, false).(bool),
		}
		log.Logger.Debug("load rollover setting",
			zap.String("action", action),
			zap.String("index", idx.IdxAlias))
		idxSettings = append(idxSettings, idx)
	}

	return idxSettings, nil
}
