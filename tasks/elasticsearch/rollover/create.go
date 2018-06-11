package rollover

import (
	"bytes"
	"context"
	"html/template"
	"net/http"

	"github.com/Laisky/go-utils"
	"golang.org/x/sync/semaphore"

	"github.com/pkg/errors"
)

var (
	idxRolloverReqBodyTpl *template.Template
)

func RunRolloverTask(ctx context.Context, sem *semaphore.Weighted, st *IdxSetting) {
	sem.Acquire(ctx, 1)
	defer sem.Release(1)

	var (
		err error
	)

	err = RolloverNewIndex(st.API, st)
	if err != nil {
		utils.Logger.Errorf("rollover index %v got error %v", st.IdxAlias, err.Error())
	}
}

func init() {
	var err error
	idxRolloverReqBodyTpl, err = template.New("idxRolloverReqBodyTpl").Parse(`
	{
		"conditions": {
			"max_age": "{{.Rollover}}"
		},
		"aliases": {
			"{{.IdxAlias}}": {}
		},
		"settings": {
			"index": {
				"number_of_shards": {{.NShards}},
				"number_of_replicas": {{.NRepls}},
				"store.type": "niofs"
			}
		},
		{{.Mapping}}
	}`)
	if err != nil {
		err = errors.Wrap(err, "load index rollover template error")
		panic(err)
	}
}

func GetIdxRolloverReqBodyByIdxAlias(idxSt *IdxSetting) (jb *bytes.Buffer, err error) {
	utils.Logger.Debugf("get rollover json body for index %v", idxSt.IdxAlias)
	jb = new(bytes.Buffer)
	if err = idxRolloverReqBodyTpl.Execute(jb, idxSt); err != nil {
		return nil, errors.Wrapf(err, "parse index rollover for %v got error", idxSt.IdxAlias)
	}

	return jb, nil
}

func RolloverNewIndex(api string, st *IdxSetting) (err error) {
	utils.Logger.Infof("rollover index for %v", st.IdxAlias)
	var (
		url  = api + st.IdxWriteAlias + "/_rollover"
		jb   *bytes.Buffer
		req  *http.Request
		resp *http.Response
	)
	jb, err = GetIdxRolloverReqBodyByIdxAlias(st)
	if err != nil {
		return errors.Wrap(err, "get index rollover body got error")
	}

	req, err = http.NewRequest("POST", url, jb)
	if err != nil {
		return errors.Wrap(err, "try to make rollover index http request got error")
	}
	req.Header.Set("Content-Type", "application/json")

	utils.Logger.Debugf("request to rollover index %+v", jb.String())
	if utils.Settings.GetBool("dry") {
		return nil
	}

	resp, err = httpClient.Do(req)
	if err != nil {
		err = errors.Wrap(err, "try to request rollover api got error")
	}

	err = utils.CheckResp(resp)
	if err != nil {
		return errors.Wrap(err, "try to request rollover api got error")
	}

	utils.Logger.Infof("suceess rollover index %v", st.IdxAlias)
	return nil
}
