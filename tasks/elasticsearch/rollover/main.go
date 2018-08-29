package rollover

import (
	"context"
	"encoding/json"
	"html/template"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Laisky/go-utils"
	"go.uber.org/zap"

	"github.com/Laisky/go-ramjet/tasks/store"
	"golang.org/x/sync/semaphore"

	"github.com/pkg/errors"
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
}

// BindRolloverIndices bind the task to rollover indices
func BindRolloverIndices() {
	utils.Logger.Info("bind rollover indices...")

	if utils.Settings.GetBool("debug") { // set for debug
		utils.Settings.Set("tasks.elasticsearch-v2.interval", 10)
	}

	bindHTTP()
	go store.TickerAfterRun(utils.Settings.GetDuration("tasks.elasticsearch-v2.interval")*time.Second, runTask)
}

func runTask() {
	var (
		taskSts []*IdxSetting
		st      *IdxSetting
		ctx     = context.Background()
		sem     = semaphore.NewWeighted(utils.Settings.GetInt64("tasks.elasticsearch-v2.concurrent"))
	)

	taskSts = LoadSettings()

	for _, st = range taskSts {
		go RunDeleteTask(ctx, sem, st)
		go RunRolloverTask(ctx, sem, st)
	}
}

// LoadAllIndicesNames load all indices name by ES API
func LoadAllIndicesNames(api string) (indices []string, err error) {
	utils.Logger.Info("load indices by api", zap.String("api", strings.Split(api, "@")[1]))
	var (
		url     = api + "_cat/indices/?h=index&format=json"
		idxList = []map[string]string{}
		idxItm  map[string]string
	)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, errors.Wrapf(err, "http get error for url %v", url)
	}
	defer resp.Body.Close()
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "load body error for url %v", url)
	}
	err = json.Unmarshal(respBytes, &idxList)
	if err != nil {
		return nil, errors.Wrapf(err, "parse json body for url %v error %v", url, string(respBytes[:]))
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
	for _, itemI = range utils.Settings.Get("tasks.elasticsearch-v2.configs").([]interface{}) {
		item = itemI.(map[interface{}]interface{})
		if action = item["action"].(string); action != "rollover" {
			continue
		}

		idx = &IdxSetting{
			Regexp:        regexp.MustCompile(item["index"].(string)),
			Expires:       time.Duration(item["expires"].(int)) * time.Second,
			IdxAlias:      item["index-alias"].(string),
			IdxWriteAlias: item["index-write-alias"].(string),
			Mapping:       Mappings[item["mapping"].(string)],
			API:           item["api"].(string),
			Rollover:      item["rollover"].(string),
			NRepls:        utils.FallBack(func() interface{} { return item["n-replicas"].(int) }, 1).(int),
			NShards:       utils.FallBack(func() interface{} { return item["n-shards"].(int) }, 5).(int),
		}
		utils.Logger.Debug("load rollover setting")
		idxSettings = append(idxSettings, idx)
	}

	return
}
