package keyword

import (
	"strings"

	"github.com/Laisky/go-chaining"
	utils "github.com/Laisky/go-utils"

	"github.com/yanyiwu/gojieba"
)

var (
	isUseHMM = false
)

type Analyser struct {
	j *gojieba.Jieba
}

func NewAnalyser() *Analyser {
	return &Analyser{j: gojieba.NewJieba()}
}

func (a *Analyser) Cut2Words(cnt string, minialCount, topN int) (words []string) {
	return chaining.Flow(
		FilterFmt,
		a.cut2Words,
		FilterDiscardWords,
		Convert2WrodsMap,
		FilterMinimalWordsCount(minialCount),
		FilterMostFreqWords(topN),
	)(cnt, nil).GetSliceString()
}

func (a *Analyser) cut2Words(c *chaining.Chain) (interface{}, error) {
	return a.j.CutAll(c.GetString()), nil
}

func FilterDiscardWords(c *chaining.Chain) (interface{}, error) {
	var (
		w, dw string
	)
	ret := []string{}
	for _, w = range c.GetSliceString() {
		if discardWordsRegex.MatchString(w) {
			goto PASS
		}

		for _, dw = range discardWords {
			if w == dw {
				goto PASS
			}
		}

		ret = append(ret, w)
	PASS:
	}

	return ret, nil
}

func FilterFmt(c *chaining.Chain) (interface{}, error) {
	ret := discardFmtRegex.ReplaceAllString(c.GetString(), "")
	return strings.Replace(ret, " ", "", -1), nil
}

func Convert2WrodsMap(c *chaining.Chain) (interface{}, error) {
	wordsMap := map[string]int{}
	var ok bool
	for _, w := range c.GetSliceString() {
		if _, ok = wordsMap[w]; !ok {
			wordsMap[w] = 1
		} else {
			wordsMap[w]++
		}
	}

	return wordsMap, nil
}

func FilterMinimalWordsCount(minialCount int) func(c *chaining.Chain) (interface{}, error) {
	return func(c *chaining.Chain) (interface{}, error) {
		wordsMap := c.GetVal().(map[string]int)
		for k, v := range wordsMap {
			if v < minialCount {
				delete(wordsMap, k)
			}
		}

		return wordsMap, nil
	}
}

type sortItem struct {
	k string
	v int
}

func (i *sortItem) GetValue() int {
	return i.v
}
func (i *sortItem) GetKey() interface{} {
	return i.k
}

func FilterMostFreqWords(topN int) func(c *chaining.Chain) (interface{}, error) {
	pairLs := utils.PairList{}
	keyLs := []string{}
	return func(c *chaining.Chain) (interface{}, error) {
		wordsMap := c.GetVal().(map[string]int)
		for k, v := range wordsMap {
			pairLs = append(pairLs, &sortItem{k, v})
		}

		utils.SortBiggest(pairLs)
		for i, k := range pairLs {
			if i >= topN {
				break
			}
			keyLs = append(keyLs, k.GetKey().(string))
		}

		return keyLs, nil
	}
}
