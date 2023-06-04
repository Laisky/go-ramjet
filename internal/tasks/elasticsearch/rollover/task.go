package rollover

import (
	"context"
	"html/template"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/go-utils/v4/json"
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
	var (
		taskSts []*IdxSetting
		st      *IdxSetting
		ctx     = context.Background()
		sem     = semaphore.NewWeighted(
			gconfig.Shared.GetInt64("tasks.elasticsearch-v2.concurrent"))
	)

	taskSts = LoadSettings()

	for _, st = range taskSts {
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
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, errors.Wrapf(err, "http get error for url %v", urlMasking(url))
	}
	defer resp.Body.Close() // nolint: errcheck,gosec

	respBytes, err := ioutil.ReadAll(resp.Body)
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
func LoadSettings() (idxSettings []*IdxSetting) {
	var (
		idx    *IdxSetting
		itemI  interface{}
		item   map[interface{}]interface{}
		action string
	)
	for _, itemI = range gconfig.Shared.Get("tasks.elasticsearch-v2.configs").([]interface{}) {
		item = itemI.(map[interface{}]interface{})
		if action = item["action"].(string); action != "rollover" {
			continue
		}

		idx = &IdxSetting{
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

	return
}
