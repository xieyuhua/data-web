package handler

import (
	"strconv"

	"bi-platform/internal/pkg/response"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

type PushHandler struct {
	service *service.PushService
	Reload  func()
}

func NewPushHandler(service *service.PushService) *PushHandler {
	return &PushHandler{service: service}
}

func (h *PushHandler) Templates(c *gin.Context) {
	items, err := h.service.ListTemplates()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.OK(c, items)
}

func (h *PushHandler) CreateTemplate(c *gin.Context) {
	var input service.PushTemplateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "推送模板参数不正确")
		return
	}
	item, err := h.service.CreateTemplate(input)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, item)
}

func (h *PushHandler) UpdateTemplate(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var input service.PushTemplateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "推送模板参数不正确")
		return
	}
	item, err := h.service.UpdateTemplate(id, input)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, item)
}

func (h *PushHandler) DeleteTemplate(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.service.DeleteTemplate(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

func (h *PushHandler) Subscriptions(c *gin.Context) {
	items, err := h.service.ListSubscriptions()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.OK(c, items)
}

func (h *PushHandler) CreateSubscription(c *gin.Context) {
	var input service.SubscriptionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "推送任务参数不正确")
		return
	}
	item, err := h.service.CreateSubscription(input, currentUserID(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if h.Reload != nil {
		h.Reload()
	}
	response.Created(c, item)
}

func (h *PushHandler) UpdateSubscription(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var input service.SubscriptionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "推送任务参数不正确")
		return
	}
	item, err := h.service.UpdateSubscription(id, input)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if h.Reload != nil {
		h.Reload()
	}
	response.OK(c, item)
}

func (h *PushHandler) DeleteSubscription(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.service.DeleteSubscription(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if h.Reload != nil {
		h.Reload()
	}
	response.OK(c, gin.H{"deleted": true})
}

func (h *PushHandler) SendSubscription(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	summary, err := h.service.SendSubscription(c.Request.Context(), id)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, summary)
}

func (h *PushHandler) Records(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	items, err := h.service.ListRecords(limit)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.OK(c, items)
}

func (h *PushHandler) RetryRecord(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if id == 0 {
		response.BadRequest(c, "推送记录不存在")
		return
	}
	summary, err := h.service.RetryRecord(c.Request.Context(), id)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, summary)
}

func (h *PushHandler) ChannelConfigs(c *gin.Context) {
	items, err := h.service.ListChannelConfigs()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.OK(c, items)
}

func (h *PushHandler) SaveChannelConfig(c *gin.Context) {
	channelName := c.Param("channel")
	var input struct {
		Name    string         `json:"name"`
		Enabled bool           `json:"enabled"`
		Config  map[string]any `json:"config"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "通道配置参数不正确")
		return
	}
	item, err := h.service.SaveChannelConfig(channelName, input.Name, input.Enabled, input.Config)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, item)
}

func (h *PushHandler) Preview(c *gin.Context) {
	templateID, _ := strconv.ParseUint(c.Query("template_id"), 10, 64)
	dashboardID, _ := strconv.ParseUint(c.Query("dashboard_id"), 10, 64)
	item, err := h.service.Preview(templateID, dashboardID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, item)
}

func currentUserID(c *gin.Context) uint64 {
	user := currentUser(c)
	if user == nil {
		return 0
	}
	return user.ID
}
