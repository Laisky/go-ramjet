package http

import (
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	rutils "github.com/Laisky/go-ramjet/library/redis"
	"github.com/Laisky/go-ramjet/library/web"
)

// CreateDeepresearchRequest deepresearch request
type CreateDeepresearchRequest struct {
	Prompt string `json:"prompt" binding:"required,min=1"`
}

// CreateDeepResearchHandler deepresearch handler
func CreateDeepResearchHandler(c *gin.Context) {
	logger := gmw.GetLogger(c)
	user, err := getUserByAuthHeader(c)
	if web.AbortErr(c, errors.WithStack(err)) {
		return
	}

	if user.IsFree {
		web.AbortErr(c, errors.New("free user cannot create deepresearch task. "+
			"you need upgrade to a paid membership, "+
			"more info at https://wiki.laisky.com/projects/gpt/pay/cn/"))
		return
	}

	req := new(CreateDeepresearchRequest)
	err = c.ShouldBindJSON(req)
	if web.AbortErr(c, errors.WithStack(err)) {
		return
	}

	taskID, err := rutils.GetCli().
		AddLLMStormTask(gmw.Ctx(c), req.Prompt, user.Token)
	if web.AbortErr(c, errors.WithStack(err)) {
		return
	}

	logger.Info("deepresearch task created",
		zap.String("user", user.UserName),
		zap.String("task_id", taskID))
	c.JSON(http.StatusOK, gin.H{
		"task_id": taskID,
	})
}

// GetDeepResearchStatusHandler get deepresearch status
func GetDeepResearchStatusHandler(c *gin.Context) {
	logger := gmw.GetLogger(c)

	user, err := getUserByAuthHeader(c)
	if web.AbortErr(c, errors.WithStack(err)) {
		return
	}

	if user.IsFree {
		web.AbortErr(c, errors.New("free user cannot create deepresearch task. "+
			"you need upgrade to a paid membership, "+
			"more info at https://wiki.laisky.com/projects/gpt/pay/cn/"))
		return
	}

	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		web.AbortErr(c, errors.New("should set task_id"))
		return
	}

	task, err := rutils.GetCli().
		GetLLMStormTaskResult(gmw.Ctx(c), taskID)
	if web.AbortErr(c, errors.WithStack(err)) {
		return
	}

	logger.Info("get deepresearch status",
		zap.String("task_id", task.TaskID),
		zap.String("status", task.Status))

	task.APIKey = "*******" // hide api key
	c.JSON(http.StatusOK, task)
}
