package http

import (
	"context"
	"time"

	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/web"
)

// SaveLlmConservationHandler save llm conservation
func SaveLlmConservationHandler(ctx *gin.Context) {
	req := new(LLMConservationReq)
	if err := ctx.BindJSON(req); web.AbortErr(ctx, err) {
		return
	}

	freq := new(FrontendReq)
	if err := copier.Copy(freq, req); web.AbortErr(ctx, err) {
		return
	}

	go saveLLMConservation(freq, req.Response)
}

func saveLLMConservation(req *FrontendReq, respContent string) {
	logger := log.Logger.Named("save_llm")

	if req == nil {
		logger.Debug("skip saving conservation for empty request")
		return
	}

	if len(req.Messages) == 0 {
		logger.Debug("skip saving conservation for request without messages")
		return
	}

	// save to cache
	if cacheKey, err := req2CacheKey(req); err != nil {
		logger.Warn("marshal req for cache key", zap.Error(err))
	} else {
		llmRespCache.Store(cacheKey, respContent)
	}

	// save to db
	openaidb, err := db.GetOpenaiDB()
	if err != nil {
		logger.Error("get openai db", zap.Error(err))
		return
	}

	docu := &db.OpenaiConservation{
		Model:      req.Model,
		MaxTokens:  req.MaxTokens,
		Completion: respContent,
	}
	for _, msg := range req.Messages {
		docu.Prompt = append(docu.Prompt, db.OpenaiMessage{
			Role:    msg.Role.String(),
			Content: msg.Content.String(),
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ret, err := openaidb.GetCol("conservations").InsertOne(ctx, docu)
	if err != nil {
		logger.Error("insert conservation", zap.Error(err))
		return
	}

	logger.Debug("save conservation", zap.Any("id", ret.InsertedID))
}
