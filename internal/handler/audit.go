package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/service"
	"biaoge-bi/internal/web"
)

type AuditHandler struct {
	audit *service.AuditService
}

func NewAuditHandler(audit *service.AuditService) *AuditHandler {
	return &AuditHandler{audit: audit}
}

func (h *AuditHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
	list, total, err := h.audit.List(page, pageSize, c.Query("action"))
	if err != nil {
		web.ServerError(c, "查询审计日志失败")
		return
	}
	web.Page(c, list, total, page, pageSize)
}
