package gitlab

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/go-utils/v5/json"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/library/log"
)

var svc *Service

var httpCli *http.Client

func init() {
	var err error
	httpCli, err = gutils.NewHTTPClient()
	if err != nil {
		log.Logger.Panic("new http client", zap.Error(err))
	}
}

type Service struct {
	gitAPI   string
	gitToken string
}

func NewService(gitAPI, gitToken string) *Service {
	return &Service{gitAPI: gitAPI, gitToken: gitToken}
}

func InitSvc(gitAPI, gitToken string) {
	svc = NewService(gitAPI, gitToken)
}

type gitFileResponse struct {
	Content string `json:"content"`
}

func (s *Service) GetFile(ctx context.Context, file string) (*GetFileResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	file, err := url.QueryUnescape(file)
	if err != nil {
		return nil, err
	}

	query, err := parseGitFileReq(file)
	if err != nil {
		return nil, err
	}

	gitUrl := fmt.Sprintf("%s/projects/%s/repository/files/%s?ref=%s", s.gitAPI, query.ID, query.Path, query.Ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gitUrl, nil)
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}

	req.Header.Set("PRIVATE-TOKEN", s.gitToken)
	resp, err := httpCli.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "do request")
	}

	defer gutils.LogErr(resp.Body.Close, log.Logger) // nolint: errcheck,gosec
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("status code %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read body")
	}

	gitResp := new(gitFileResponse)
	if err = json.Unmarshal(body, gitResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal body")
	}

	fileCnt, err := base64.StdEncoding.DecodeString(gitResp.Content)
	if err != nil {
		return nil, errors.Wrap(err, "base64 decode")
	}

	start, _, cnt := extractFileSurrounding(fileCnt, int(query.LineFrom), int(query.LineTo))
	return &GetFileResponse{
		Content:  cnt,
		LineFrom: start,
	}, nil
}

const gitCodeSurrounding = 0

func extractFileSurrounding(fileCnt []byte, lineFrom, lineTo int) (start uint, end uint, content string) {
	lines := strings.Split(string(fileCnt), "\n")
	lineFrom = gutils.Max(gutils.Min((lineFrom)-gitCodeSurrounding, len(lines)-1), 0)

	if lineTo == 0 {
		lineTo = len(lines)
	} else {
		lineTo = gutils.Min(lineTo+1+gitCodeSurrounding, len(lines))
	}

	return uint(lineFrom), uint(lineTo), strings.Join(lines[lineFrom:lineTo], "\n")
}

//nolint:lll
var gitFileReqRegexp = regexp.MustCompile(`(?m:https://git\.basebit\.me/(?P<id>[^/]+/[^/]+)/-/blob/(?P<ref>\w+)/(?P<path>[^#]+)(?:#L(?P<line_from>\d+)(?:-(?P<line_to>\d+))?)?)`)

type GitFileURL struct {
	ID       string
	Ref      string
	Path     string
	LineFrom uint
	LineTo   uint
}

func parseGitFileReq(file string) (ret *GitFileURL, err error) {
	file = strings.TrimSpace(file)
	for _, line := range strings.Split(file, "\n") {
		matched := gitFileReqRegexp.FindStringSubmatch(line)
		if len(matched) > 0 {
			ret = &GitFileURL{
				ID:   matched[1],
				Ref:  matched[2],
				Path: matched[3],
			}

			ret.ID = url.QueryEscape(ret.ID)
			ret.Path = url.QueryEscape(ret.Path)

			if len(matched) >= 4 && matched[4] != "" {
				lineFrom, err := strconv.Atoi(matched[4])
				if err != nil {
					return nil, errors.Wrapf(err, "parse line from %s", matched[4])
				}

				ret.LineFrom = uint(lineFrom)
			}

			if len(matched) >= 5 && matched[5] != "" {
				lineTo, err := strconv.Atoi(matched[5])
				if err != nil {
					return nil, errors.Wrapf(err, "parse line to %s", matched[5])
				}

				ret.LineTo = uint(lineTo)
			}

			// allow specifying only line_from (line_to==0 means till EOF)
			if ret.LineTo != 0 && ret.LineFrom != 0 && ret.LineFrom >= ret.LineTo {
				return nil, errors.Errorf("line_from %d should not bigger than line_to %d", ret.LineFrom, ret.LineTo)
			}

			return
		}
	}

	return nil, err
}
