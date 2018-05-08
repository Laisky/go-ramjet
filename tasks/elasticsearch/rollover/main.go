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

	"golang.org/x/sync/semaphore"
	"github.com/Laisky/go-ramjet/tasks/store"

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
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
	Expires       float64
	IdxAlias      string
	IdxWriteAlias string
	Mapping       template.HTML
	API           string
}

// BindRolloverIndices bind the task to rollover indices
func BindRolloverIndices() {
	log.Info("bind rollover indices...")

	if viper.GetBool("debug") { // set for debug
		viper.Set("tasks.elasticsearch-v2.interval", 1)
	}

	go store.Ticker(viper.GetDuration("tasks.elasticsearch-v2.interval")*time.Second, runTask)
}

func runTask() {
	var (
		taskSts []*IdxSetting
		st      *IdxSetting
		ctx     = context.Background()
		sem     = semaphore.NewWeighted(viper.GetInt64("tasks.elasticsearch-v2.concurrent"))
	)

	taskSts = LoadSettings()

	for _, st = range taskSts {
		go RunDeleteTask(ctx, sem, st)
		go RunRolloverTask(ctx, sem, st)
	}
}

// LoadAllIndicesNames load all indices name by ES API
func LoadAllIndicesNames(api string) (indices []string, err error) {
	log.Infof("load indices by api %v", strings.Split(api, "@")[1])
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
	for _, itemI = range viper.Get("tasks.elasticsearch-v2.configs").([]interface{}) {
		item = itemI.(map[interface{}]interface{})
		if action = item["action"].(string); action != "rollover" {
			continue
		}

		idx = &IdxSetting{
			Regexp:        regexp.MustCompile(item["index"].(string)),
			Expires:       float64(item["expires"].(int)),
			IdxAlias:      item["index-alias"].(string),
			IdxWriteAlias: item["index-write-alias"].(string),
			Mapping:       Mappings[item["mapping"].(string)],
			API:           item["api"].(string),
			Rollover:      item["rollover"].(string),
		}
		idxSettings = append(idxSettings, idx)
	}

	return
}
