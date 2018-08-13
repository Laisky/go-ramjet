package rollover

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/Laisky/go-utils"
	"github.com/pkg/errors"
)

// RunDeleteTask start to delete indices
func RunDeleteTask(ctx context.Context, sem *semaphore.Weighted, st *IdxSetting) {
	sem.Acquire(ctx, 1)
	defer sem.Release(1)
	utils.Logger.Debugf("start to running delete expired index for %v...", st.IdxAlias)

	var (
		allIdx        []string
		tobeDeleteIdx []string
		err           error
	)

	allIdx, err = LoadAllIndicesNames(st.API)
	if err != nil {
		utils.Logger.Errorf("load indices got error %+v", err)
	}

	// Delete expired indices
	tobeDeleteIdx, err = FilterToBeDeleteIndicies(allIdx, st)
	if err != nil {
		utils.Logger.Errorf("try to filter delete indices got error %+v", err)
	}

	// Do not delete write-alias
	tobeDeleteIdx, err = FilterReadyToBeDeleteIndices(GetAliasURL(st), tobeDeleteIdx)
	if err != nil {
		utils.Logger.Errorf("try to filter indices aliases got error %+v", err)
	}

	utils.Logger.Infof("try to delete indices %+v", tobeDeleteIdx)
	for _, idx := range tobeDeleteIdx {
		err = RemoveIndexByName(st.API, idx)
		if err != nil {
			utils.Logger.Errorf("try to delete index %v got error %+v", idx, err)
		}
	}
}

// RemoveIndexByName delete index by elasticsearch RESTful API
func RemoveIndexByName(api, index string) (err error) {
	utils.Logger.Infof("remove es index %v", index)
	url := api + index
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return errors.Wrap(err, "make request error")
	}

	utils.Logger.Debugf("remove index %v", index)
	if utils.Settings.GetBool("dry") {
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

	utils.Logger.Infof("success delete index %v", index)
	return nil
}

// IsIdxShouldDelete check whether a index is should tobe deleted
// dateStr like `2016.10.31`, treated as +0800
func IsIdxShouldDelete(now time.Time, dateStr string, expires time.Duration) (bool, error) {
	layout := "2006.01.02 -0700"
	t, err := time.Parse(layout, dateStr+" +0800")
	if err != nil {
		return false, errors.Wrapf(err, "parse date %v with layout %v error", dateStr, layout)
	}
	t = t.Add(24 * time.Hour) // elasticsearch dateStr has 1 day delay
	return now.Sub(t) > expires, nil
}

// FilterToBeDeleteIndicies return the indices that need be delete
func FilterToBeDeleteIndicies(allInd []string, idxSt *IdxSetting) (indices []string, err error) {
	utils.Logger.Debugf("start to filter tobe delete indices %+v %+v", allInd, idxSt.Regexp)
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

		utils.Logger.Debugf("check is index %v should be deleted with expires %v", idx, idxSt.Expires)
		toDelete, err = IsIdxShouldDelete(time.Now(), subS[1], idxSt.Expires)
		if err != nil {
			err = errors.Wrapf(err, "check whether index %v(%v) should delete got error", idx, idxSt.Expires)
			return nil, err
		}
		if toDelete {
			indices = append(indices, subS[0])
		}
	}

	utils.Logger.Debugf("tobe delete indices %+v", indices)
	return indices, nil
}
