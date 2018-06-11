package rollover

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/Laisky/go-utils"
	"github.com/pkg/errors"
)

var (
	aliasAPI = []string{"_cat/aliases/", "/?v&format=json"}
)

// AliasesResp is the response of es aliases API
type AliasesResp struct {
	Index string `json:"index"`
}

// GetAliasURL return the ES alisese endpoint
func GetAliasURL(st *IdxSetting) string {
	return st.API + aliasAPI[0] + st.IdxWriteAlias + aliasAPI[1]
}

// FilterReadyToBeDeleteIndices filter indices that is ready to be deleted
func FilterReadyToBeDeleteIndices(aliasURL string, allIdx []string) (indices []string, err error) {
	utils.Logger.Debugf("FilterReadyToBeDeleteIndices for aliasURL %v, allIdx %+v", aliasURL, allIdx)
	var (
		aliases []*AliasesResp
	)
	aliases, err = LoadAliases(aliasURL)
	if err != nil {
		return nil, errors.Wrap(err, "try to load aliases got error")
	}
	for _, idx := range allIdx {
		if !IsIdxIsWriteAlias(idx, aliases) {
			indices = append(indices, idx)
		}
	}

	return indices, nil
}

// LoadAliases load all indices aliases from ES
func LoadAliases(url string) (aliases []*AliasesResp, err error) {
	var (
		resp  *http.Response
		respB []byte
	)
	resp, err = httpClient.Get(url)
	if err != nil {
		return nil, errors.Wrap(err, "request aliases api error")
	}

	err = utils.CheckResp(resp)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	respB, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "try to read resp body error")
	}

	err = json.Unmarshal(respB, &aliases)
	if err != nil {
		return nil, errors.Wrapf(err, "try to parse json error for json %v", string(respB[:]))
	}

	return aliases, nil
}

// IsIdxIsWriteAlias check is indices' alias is write-alias
func IsIdxIsWriteAlias(idx string, aliases []*AliasesResp) (ret bool) {
	for _, ad := range aliases {
		if ad.Index == idx {
			utils.Logger.Debugf("IsIdxIsWriteAlias for idx %v, aliases %v --> true", idx, ad.Index)
			return true
		}
		utils.Logger.Debugf("IsIdxIsWriteAlias for idx %v, aliases %v --> false", idx, ad.Index)
	}

	return false
}
