package crawler

import (
	"context"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	gutils "github.com/Laisky/go-utils/v2"
	"github.com/Laisky/zap"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/pkg/errors"

	"github.com/Laisky/go-ramjet/library/log"
)

var httpCli *http.Client

func init() {
	var err error
	httpCli, err = gutils.NewHTTPClient()
	if err != nil {
		log.Logger.Panic("new http client", zap.Error(err))
	}
}

type Service struct {
	dao         *Dao
	searchCache *gutils.ExpCache
}

func NewService(ctx context.Context, addr, dbName, user, pwd, docusColName string) (*Service, error) {
	dao, err := NewDao(ctx, addr, dbName, user, pwd, docusColName)
	if err != nil {
		return nil, err
	}

	return &Service{
		dao:         dao,
		searchCache: gutils.NewExpCache(ctx, time.Minute),
	}, nil
}

// func (s *Service) RemoveOldPages() error {
// 	return s.dao.RemoveLegacy(time.Now().Add(10 * time.Minute))
// }

func (s *Service) Search(text string) (rets []SearchResult, err error) {
	if data, ok := s.searchCache.Load(text); ok {
		return data.([]SearchResult), nil
	}

	rets, err = s.dao.Search(text)
	if err != nil {
		return nil, errors.Wrapf(err, "search %s", text)
	}

	s.searchCache.Store(text, rets)
	return rets, nil
}

func (s *Service) CrawlAllPages(sitemaps []string) error {
	startAt := gutils.Clock.GetUTCNow()
	for _, u := range loadAllURLs(sitemaps) {
		log.Logger.Debug("crawl", zap.String("url", u))
		raw, err := httpGet(u)
		if err != nil {
			log.Logger.Error("crawl page", zap.String("url", u), zap.Error(err))
			continue
		}

		text := extractAllText(raw)
		title := extractTitle(raw)
		if err := s.dao.Save(title, text, u); err != nil {
			return errors.Wrapf(err, "save text `%s`", u)
		}
	}

	if err := s.dao.RemoveLegacy(startAt); err != nil {
		return errors.Wrapf(err, "remove legacy before %s", startAt.Format(time.RFC3339))
	}

	return nil
}

var regexpLoadURLFromSitemap = regexp.MustCompile(`<loc>(.*?)</loc>`)

func loadAllURLs(sitemaps []string) []string {
	urls := mapset.NewSet[string]()
	for _, s := range sitemaps {
		cnt, err := httpGet(s)
		if err != nil {
			log.Logger.Error("get sitemap", zap.String("url", s), zap.Error(err))
			continue
		}

		for _, matched := range regexpLoadURLFromSitemap.FindAllStringSubmatch(cnt, -1) {
			urls.Add(matched[1])
		}
	}

	return urls.ToSlice()
}

var (
	regexpExtractText  = regexp.MustCompile(`<[\w\d =]+>(.*?)</\w+>`)
	regexpExtractTitle = regexp.MustCompile(`<title>(.*?)</title>`)
)

func extractTitle(raw string) string {
	for _, matched := range regexpExtractTitle.FindAllStringSubmatch(raw, -1) {
		return matched[1]
	}

	return ""
}

func extractAllText(raw string) string {
	var result string
	for _, matched := range regexpExtractText.FindAllStringSubmatch(raw, -1) {
		result += matched[1]
	}

	return result
}

func httpGet(url string) (string, error) {
	resp, err := httpCli.Get(url)
	if err != nil {
		return "", errors.Wrapf(err, "get url %s", url)
	}

	defer gutils.CloseQuietly(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("status code %d", resp.StatusCode)
	}

	cnt, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "read body")
	}

	return string(cnt), nil
}
