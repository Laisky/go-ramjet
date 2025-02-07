package http

import (
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	rutils "github.com/Laisky/go-ramjet/library/redis"
	"github.com/Laisky/go-ramjet/library/web"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
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

	req := new(CreateDeepresearchRequest)
	err = c.ShouldBindJSON(req)
	if web.AbortErr(c, errors.WithStack(err)) {
		return
	}

	// =====================================
	// FOR TEST
	// =====================================
	// c.JSON(http.StatusOK, gin.H{
	// 	"task_id": "0194de67-9011-71c6-8006-6b39c7a11145",
	// })
	// return
	// =====================================

	taskID, err := rutils.GetCli().AddLLMStormTask(c.Request.Context(), req.Prompt, user.Token)
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

	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		web.AbortErr(c, errors.New("should set task_id"))
		return
	}

	task, err := rutils.GetCli().GetLLMStormTaskResult(c.Request.Context(), taskID)
	if web.AbortErr(c, errors.WithStack(err)) {
		return
	}

	logger.Info("get deepresearch status",
		zap.String("task_id", task.TaskID),
		zap.String("status", task.Status))

	task.APIKey = "*******" // hide api key
	c.JSON(http.StatusOK, task)
}
