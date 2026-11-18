package handler

import (
	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/service"
	"biaoge-bi/internal/web"
)

type SettingHandler struct {
	svc   *service.SettingService
	audit *service.AuditService
}

func NewSettingHandler(svc *service.SettingService, audit *service.AuditService) *SettingHandler {
	return &SettingHandler{svc: svc, audit: audit}
}

// List returns every editable configuration field grouped by section.
func (h *SettingHandler) List(c *gin.Context) {
	sections, err := h.svc.List()
	if err != nil {
		web.ServerError(c, "读取配置失败")
		return
	}
	web.OK(c, gin.H{"sections": sections})
}

type updateSettingRequest struct {
	Values map[string]string `json:"values"`
}

// Update persists and hot-applies configuration changes.
func (h *SettingHandler) Update(c *gin.Context) {
	var req updateSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.Update(req.Values); err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	keys := make([]string, 0, len(req.Values))
	for k := range req.Values {
		keys = append(keys, k)
	}
	h.audit.Log(c, "settings_update", "setting", 0, gin.H{"keys": keys})
	web.OK(c, gin.H{"message": "配置已保存并生效"})
}

type resetSettingRequest struct {
	Keys []string `json:"keys"`
}

// Reset removes overrides so file/default values apply again.
func (h *SettingHandler) Reset(c *gin.Context) {
	var req resetSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.Reset(req.Keys); err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	h.audit.Log(c, "settings_reset", "setting", 0, gin.H{"keys": req.Keys})
	web.OK(c, gin.H{"message": "已恢复默认值"})
}
