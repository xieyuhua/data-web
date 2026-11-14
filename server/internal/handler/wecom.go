package handler

import (
	"errors"
	"net/http"
	"strconv"

	"bi-platform/internal/middleware"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

// WecomHandler 企业微信通讯录同步与登录配置接口。
type WecomHandler struct {
	service *service.WecomSyncService
}

// NewWecomHandler 创建企业微信管理处理器。
func NewWecomHandler(svc *service.WecomSyncService) *WecomHandler {
	return &WecomHandler{service: svc}
}

// Config 返回企业微信同步配置（Secret 已脱敏）。
func (h *WecomHandler) Config(c *gin.Context) {
	view, err := h.service.Config()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.OK(c, view)
}

// SaveConfig 保存企业微信应用配置与同步/登录策略。
func (h *WecomHandler) SaveConfig(c *gin.Context) {
	var input service.WecomConfigInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "配置参数不正确")
		return
	}
	view, err := h.service.SaveConfig(input)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, view)
}

// Test 校验 CorpID / Secret 是否可用，并返回部门与成员数量概览。
func (h *WecomHandler) Test(c *gin.Context) {
	var input service.WecomConfigInput
	_ = c.ShouldBindJSON(&input)
	result, err := h.service.Test(c.Request.Context(), input)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

// Sync 创建异步同步任务；已有任务运行时返回 409。
func (h *WecomHandler) Sync(c *gin.Context) {
	var input service.WecomSyncInput
	_ = c.ShouldBindJSON(&input)
	user := middleware.CurrentUser(c)
	var userID uint64
	if user != nil {
		userID = user.ID
	}
	job, err := h.service.Start(userID, input)
	if err != nil {
		if errors.Is(err, service.ErrWecomSyncRunning) {
			response.Fail(c, http.StatusConflict, err.Error())
			return
		}
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, job)
}

// Jobs 返回最近的同步任务列表。
func (h *WecomHandler) Jobs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, err := h.service.Jobs(limit)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.OK(c, items)
}

// Job 返回单个同步任务，供前端轮询进度。
func (h *WecomHandler) Job(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	item, err := h.service.Job(id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, item)
}
