package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/auth"
	"biaoge-bi/internal/web"
	"biaoge-bi/internal/service"
)

// PublicHandler serves token-authorized dashboard views without a session.
// It never exposes SQL, datasource details or user data.
type PublicHandler struct {
	share     *service.ShareService
	dashboard *service.DashboardService
	rate      *auth.Limiter
}

func NewPublicHandler(share *service.ShareService, dashboard *service.DashboardService, rate *auth.Limiter) *PublicHandler {
	return &PublicHandler{share: share, dashboard: dashboard, rate: rate}
}

// publicView filters dashboard internals down to what a viewer needs.
func publicView(c *gin.Context, d *service.DashboardPublicView) {
	c.JSON(http.StatusOK, web.APIResponse{Code: 0, Message: "ok", Data: d})
}

func (h *PublicHandler) View(c *gin.Context) {
	if !h.rate.Allow(c.ClientIP()) {
		web.Fail(c, http.StatusTooManyRequests, "访问过于频繁")
		return
	}
	view, err := h.dashboard.PublicView(c.Param("token"), h.share)
	if err != nil {
		web.NotFound(c, err.Error())
		return
	}
	publicView(c, view)
}

func (h *PublicHandler) Data(c *gin.Context) {
	if !h.rate.Allow(c.ClientIP()) {
		web.Fail(c, http.StatusTooManyRequests, "访问过于频繁")
		return
	}
	dashboardID, err := h.share.ResolveDashboardID(c.Param("token"))
	if err != nil {
		web.NotFound(c, err.Error())
		return
	}
	var req service.DashboardDataRequest
	_ = c.ShouldBindJSON(&req)
	data, err := h.dashboard.ExecuteData(c.Request.Context(), dashboardID, req)
	if err != nil {
		web.Unprocessable(c, err.Error())
		return
	}
	web.OK(c, data)
}

func (h *PublicHandler) FilterOptions(c *gin.Context) {
	if !h.rate.Allow(c.ClientIP()) {
		web.Fail(c, http.StatusTooManyRequests, "访问过于频繁")
		return
	}
	dashboardID, err := h.share.ResolveDashboardID(c.Param("token"))
	if err != nil {
		web.NotFound(c, err.Error())
		return
	}
	fid, perr := strconv.ParseUint(c.Param("filterId"), 10, 32)
	if perr != nil {
		web.BadRequest(c, "无效的筛选器 ID")
		return
	}
	options, err := h.dashboard.FilterOptions(c.Request.Context(), dashboardID, uint(fid))
	if err != nil {
		web.Unprocessable(c, err.Error())
		return
	}
	web.OK(c, options)
}
