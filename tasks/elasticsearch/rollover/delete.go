package rollover

import (
	"context"
	"net/http"
	"time"

	"github.com/spf13/viper"

	"golang.org/x/sync/semaphore"

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
	"github.com/Laisky/go-ramjet/utils"
)

func RunDeleteTask(ctx context.Context, sem *semaphore.Weighted, st *IdxSetting) {
	sem.Acquire(ctx, 1)
	defer sem.Release(1)
	log.Debugf("start to running delete expired index for %v...", st.IdxAlias)

	var (
		allIdx []string
		err    error
	)

	allIdx, err = LoadAllIndicesNames(st.API)
	if err != nil {
		log.Errorf("load indices got error %+v", err)
	}

	// Delete expired indices
	tobeDeleteIdx, err := FilterToBeDeleteIndicies(allIdx, st)
	if err != nil {
		log.Errorf("try to filter delete indices got error %+v", err)
	}

	for _, idx := range tobeDeleteIdx {
		err = RemoveIndexByName(st.API, idx)
		if err != nil {
			log.Errorf("try to delete index %v got error %+v", idx, err)
		}
	}
}

// RemoveIndexByName delete index by elasticsearch RESTful API
func RemoveIndexByName(api, index string) (err error) {
	log.Infof("remove es index %v", index)
	url := api + index
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return errors.Wrap(err, "make request error")
	}

	log.Debugf("remove index %v", index)
	if viper.GetBool("dry") {
		return nil
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "do request got error")
	}
	err = utils.CheckResp(resp)
	if err != nil {
		return errors.Wrap(err, "remove index got error")
	}

	log.Infof("success delete index %v", index)
	return nil
}

// IsIdxShouldDelete check whether a index is should tobe deleted
// dateStr like `2016.10.31`, treated as +0800
func IsIdxShouldDelete(now time.Time, dateStr string, expires float64) (bool, error) {
	log.Debugf("check is index %v-%v should be deleted", dateStr, now)
	layout := "2006.01.02 -0700"
	t, err := time.Parse(layout, dateStr+" +0800")
	if err != nil {
		return false, errors.Wrapf(err, "parse date %v with layout %v error", dateStr, layout)
	}
	return now.Sub(t).Seconds() > expires, nil
}

// FilterToBeDeleteIndicies return the indices that need be delete
func FilterToBeDeleteIndicies(allInd []string, idxSt *IdxSetting) (indices []string, err error) {
	log.Debugf("start to filter tobe delete indices %+v %+v", allInd, idxSt.Regexp)
	var (
		idx      string
		subS     []string
		toDelete bool
	)
	for _, idx = range allInd {
		subS = idxSt.Regexp.FindStringSubmatch(idx)
		if len(subS) < 2 {
			continue
		}

		toDelete, err = IsIdxShouldDelete(time.Now(), subS[1], idxSt.Expires)
		if err != nil {
			err = errors.Wrapf(err, "check whether index %v(%v) should delete got error", idx, idxSt.Expires)
			return
		}
		if !toDelete {
			continue
		}

		indices = append(indices, subS[0])
	}

	log.Debugf("tobe delete indices %+v", indices)
	return indices, nil
}
