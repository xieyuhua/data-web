package handler

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/model"
	"biaoge-bi/internal/service"
	"biaoge-bi/internal/web"
)

type AIHandler struct {
	ai        *service.AIService
	datasets  *service.DatasetService
	audit     *service.AuditService
}

func NewAIHandler(aiSvc *service.AIService, datasets *service.DatasetService, audit *service.AuditService) *AIHandler {
	return &AIHandler{ai: aiSvc, datasets: datasets, audit: audit}
}

// Status tells the editor whether AI features are available.
func (h *AIHandler) Status(c *gin.Context) {
	web.OK(c, h.ai.Status())
}

// Ping verifies the configured provider responds.
func (h *AIHandler) Ping(c *gin.Context) {
	if err := h.ai.Ping(c.Request.Context()); err != nil {
		web.Unprocessable(c, err.Error())
		return
	}
	web.OK(c, gin.H{"message": "AI 连接正常"})
}

type aiSQLRequest struct {
	DataSourceID uint   `json:"dataSourceId"`
	Requirement  string `json:"requirement"`
	SQLText      string `json:"sqlText"`
}

func (h *AIHandler) GenerateSQL(c *gin.Context) {
	var req aiSQLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	if req.DataSourceID == 0 {
		web.BadRequest(c, "请先选择数据源")
		return
	}
	sqlText, err := h.ai.GenerateSQL(c.Request.Context(), req.DataSourceID, req.Requirement, req.SQLText)
	if err != nil {
		h.writeAIError(c, err)
		return
	}
	h.audit.Log(c, "ai_generate_sql", "datasource", req.DataSourceID, gin.H{"requirement": req.Requirement})
	web.OK(c, gin.H{"sqlText": sqlText})
}

type aiCompleteRequest struct {
	DataSourceID uint   `json:"dataSourceId"`
	SQLText      string `json:"sqlText"`
	Cursor       int    `json:"cursor"`
}

func (h *AIHandler) CompleteSQL(c *gin.Context) {
	var req aiCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	if strings.TrimSpace(req.SQLText) == "" {
		web.BadRequest(c, "SQL 为空，无法补全")
		return
	}
	out, err := h.ai.CompleteSQL(c.Request.Context(), req.DataSourceID, req.SQLText, req.Cursor)
	if err != nil {
		h.writeAIError(c, err)
		return
	}
	web.OK(c, gin.H{"completion": out})
}

type aiParamsRequest struct {
	SQLText string                      `json:"sqlText"`
	Params  []service.DatasetParamInput `json:"params"`
}

func (h *AIHandler) SuggestParams(c *gin.Context) {
	var req aiParamsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	suggestions, err := h.ai.SuggestParams(c.Request.Context(), req.SQLText, req.Params)
	if err != nil {
		h.writeAIError(c, err)
		return
	}
	web.OK(c, suggestions)
}

type aiMetadataRequest struct {
	SQLText   string              `json:"sqlText"`
	Columns   []model.ColumnMeta  `json:"columns"`
}

func (h *AIHandler) SuggestMetadata(c *gin.Context) {
	var req aiMetadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	name, desc, err := h.ai.SuggestMetadata(c.Request.Context(), req.SQLText, req.Columns)
	if err != nil {
		h.writeAIError(c, err)
		return
	}
	web.OK(c, gin.H{"name": name, "description": desc})
}

// TestSQL executes an unsaved SQL (editor "运行测试" button).
func (h *AIHandler) TestSQL(c *gin.Context) {
	var req service.TestSQLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	res, err := h.datasets.TestSQL(c.Request.Context(), req)
	if err != nil {
		web.Unprocessable(c, err.Error())
		return
	}
	web.OK(c, res)
}

// InferParams returns parameter definitions derived from the SQL text.
func (h *AIHandler) InferParams(c *gin.Context) {
	var req aiParamsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	web.OK(c, h.datasets.InferParams(req.SQLText, req.Params))
}

func (h *AIHandler) writeAIError(c *gin.Context, err error) {
	msg := err.Error()
	switch {
	case errors.Is(err, service.ErrAIDisabled):
		web.Fail(c, 503, msg)
	case strings.Contains(msg, "AI 未启用"):
		web.Fail(c, 503, msg)
	case strings.Contains(msg, "鉴权"), strings.Contains(msg, "限流"):
		web.Unprocessable(c, msg)
	default:
		web.Unprocessable(c, msg)
	}
}
