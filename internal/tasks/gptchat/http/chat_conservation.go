package http

import (
	"context"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
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

	if err := storeCachedResponse(freq, req.Response); err != nil {
		gmw.GetLogger(ctx).Warn("cache conservation response", zap.Error(err))
	}

	go saveLLMConservation(conservationLogger(), freq, req.Response, req.Reasoning)
}

// conservationLogger returns the named process logger used by saveLLMConservation.
// Callers evaluate it as an argument of the go statement, so it is resolved on the
// calling goroutine and the detached goroutine never reads the mutable global
// log.Logger. It takes no parameters and returns the "save_llm" logger.
func conservationLogger() glog.Logger {
	return log.Logger.Named("save_llm")
}

// saveLLMConservation persists the exchange to the openai conservations collection.
// It does not touch llmRespCache; callers cache through storeCachedResponse, which
// applies the shared responseCacheAllowed policy. logger receives all diagnostics
// and must be resolved by the caller before the goroutine is spawned; req is the
// frontend request, respContent the final answer, and reasoning the optional
// reasoning text. It returns nothing; every failure is logged.
func saveLLMConservation(logger glog.Logger, req *FrontendReq, respContent string, reasoning string) {
	if req == nil {
		logger.Debug("skip saving conservation for empty request")
		return
	}

	if len(req.Messages) == 0 {
		logger.Debug("skip saving conservation for request without messages")
		return
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
		Reasoning:  reasoning,
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
