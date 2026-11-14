package handler

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"bi-platform/internal/middleware"
	"bi-platform/internal/model"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/pkg/sqlguard"
	"bi-platform/internal/repository"

	"github.com/gin-gonic/gin"
)

// currentUser 获取当前登录用户（未登录返回 nil）。
func currentUser(c *gin.Context) *model.User { return middleware.CurrentUser(c) }

// filterParams 解析看板级全局筛选器参数（filters 为 JSON 对象）。
// 值可以是字符串，也可以是字符串数组（多选）；多选值会在服务端编码为一个字段，
// 具体长度、数量与 SQL 转义仍由 sqlguard 在查询时统一校验。
func filterParams(c *gin.Context) map[string]string {
	raw := strings.TrimSpace(c.Query("filters"))
	if raw == "" {
		return nil
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil || len(parsed) == 0 {
		return nil
	}
	if len(parsed) > 16 {
		return nil
	}
	result := make(map[string]string, len(parsed))
	for key, value := range parsed {
		var single string
		if err := json.Unmarshal(value, &single); err == nil {
			result[key] = single
			continue
		}
		var list []string
		if err := json.Unmarshal(value, &list); err == nil {
			if len(list) > 50 {
				continue
			}
			result[key] = sqlguard.JoinFilterValues(list)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// AuditHandler 审计日志查询接口。
type AuditHandler struct {
	repo *repository.AuditRepository
}

// NewAuditHandler 创建审计日志处理器。
func NewAuditHandler(repo *repository.AuditRepository) *AuditHandler {
	return &AuditHandler{repo: repo}
}

// List 分页查询审计日志，同时返回概览统计与动作字典。
func (h *AuditHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}

	items, total, err := h.repo.Search(repository.AuditFilter{
		Keyword:  c.Query("keyword"),
		Action:   c.Query("action"),
		Status:   c.Query("status"),
		StartAt:  parseAuditTime(c.Query("start")),
		EndAt:    parseAuditTime(c.Query("end"), 24*time.Hour),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		response.ServerError(c, err)
		return
	}
	stats, err := h.repo.Stats()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	actions, err := h.repo.Actions()
	if err != nil {
		actions = nil
	}
	response.OK(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"stats":     stats,
		"actions":   actions,
	})
}

// parseAuditTime 解析 2006-01-02 或 RFC3339 时间；add 用于把结束日期顺延到次日零点（左闭右开）。
func parseAuditTime(raw string, add ...time.Duration) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var parsed time.Time
	var err error
	if len(raw) == len("2006-01-02") {
		parsed, err = time.ParseInLocation("2006-01-02", raw, time.Local)
	} else {
		parsed, err = time.ParseInLocation(time.RFC3339, raw, time.Local)
	}
	if err != nil {
		return nil
	}
	if len(add) > 0 {
		parsed = parsed.Add(add[0])
	}
	return &parsed
}
