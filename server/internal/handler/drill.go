package handler

import (
	"strconv"

	"bi-platform/internal/middleware"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

type DrillHandler struct {
	service *service.DrillService
}

func NewDrillHandler(service *service.DrillService) *DrillHandler {
	return &DrillHandler{service: service}
}

func (h *DrillHandler) Query(c *gin.Context) {
	orgID, _ := strconv.ParseUint(c.Query("org_id"), 10, 64)
	result, err := h.service.Query(
		middleware.CurrentUser(c), orgID,
		c.Query("period_type"), c.Query("period"), c.Query("metric"), c.Query("mode"),
	)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

// Trend 返回组织 / 员工的指标趋势，默认最近 7 天。
func (h *DrillHandler) Trend(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Query("node_id"), 10, 64)
	result, err := h.service.Trend(
		middleware.CurrentUser(c), nodeID, c.Query("node_type"),
		c.Query("metric"), c.Query("start_date"), c.Query("end_date"),
	)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}
