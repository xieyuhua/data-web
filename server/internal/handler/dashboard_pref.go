package handler

import (
	"strconv"

	"bi-platform/internal/middleware"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

// DashboardPreferenceHandler 看板个人偏好接口：收藏、置顶与最近浏览。
type DashboardPreferenceHandler struct {
	service *service.DashboardPreferenceService
}

// NewDashboardPreferenceHandler 创建看板偏好处理器。
func NewDashboardPreferenceHandler(service *service.DashboardPreferenceService) *DashboardPreferenceHandler {
	return &DashboardPreferenceHandler{service: service}
}

// List 查询当前用户的全部看板偏好。
func (h *DashboardPreferenceHandler) List(c *gin.Context) {
	prefs, err := h.service.Map(middleware.CurrentUser(c).ID)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	result := make([]service.DashboardPreferenceView, 0, len(prefs))
	for _, item := range prefs {
		result = append(result, item)
	}
	response.OK(c, result)
}

// Update 设置收藏 / 置顶；只传需要变更的字段。
func (h *DashboardPreferenceHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "看板参数不正确")
		return
	}
	var input struct {
		Favorite *bool `json:"favorite"`
		Pinned   *bool `json:"pinned"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "偏好参数不正确")
		return
	}
	// 显式标记为偏好更新，便于审计日志区分
	c.Set("audit_action", "dashboard_pref")
	item, err := h.service.Set(middleware.CurrentUser(c).ID, id, input.Favorite, input.Pinned)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, item)
}

// Touch 记录一次打开，用于首页"最近浏览"排序。
func (h *DashboardPreferenceHandler) Touch(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "看板参数不正确")
		return
	}
	if err := h.service.Touch(middleware.CurrentUser(c).ID, id); err != nil {
		response.ServerError(c, err)
		return
	}
	response.OK(c, gin.H{"recorded": true})
}
