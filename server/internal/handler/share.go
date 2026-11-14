package handler

import (
	"strconv"
	"strings"

	"bi-platform/internal/middleware"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

// ShareHandler 看板分享接口：生成 / 列表 / 撤销 / 公开访问。
type ShareHandler struct {
	share     *service.ShareService
	dashboard *service.DashboardService
	scope     *service.ScopeService
	drill     *service.DrillService
}

// NewShareHandler 创建分享处理器。
func NewShareHandler(share *service.ShareService, dashboard *service.DashboardService, scope *service.ScopeService, drill *service.DrillService) *ShareHandler {
	return &ShareHandler{share: share, dashboard: dashboard, scope: scope, drill: drill}
}

// Create 生成分享链接。
// 组织范围在生成时固定下来，后续公开访问只能看到该范围内的数据。
func (h *ShareHandler) Create(c *gin.Context) {
	var input struct {
		DashboardID uint64 `json:"dashboard_id"`
		ExpireHours int    `json:"expire_hours"`
		MaxViews    int    `json:"max_views"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.DashboardID == 0 {
		response.BadRequest(c, "请选择要分享的看板")
		return
	}
	user := middleware.CurrentUser(c)
	dashboard, err := h.dashboard.Get(input.DashboardID)
	if err != nil {
		response.NotFound(c, "看板不存在")
		return
	}
	scope, err := h.scope.Resolve(user)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	if !service.CanViewDashboard(user, middleware.CurrentRole(c), service.DashboardModelFromView(dashboard), scope.OrgIDs) {
		response.Forbidden(c, "无法分享未发布看板")
		return
	}
	orgIDs := scope.OrgIDs
	if dashboard.ScopeMode == "all" {
		all, err := h.scope.AllOrgIDs()
		if err != nil {
			response.ServerError(c, err)
			return
		}
		orgIDs = all
	}
	item, err := h.share.Create(user.ID, input.DashboardID, orgIDs, input.ExpireHours, input.MaxViews, requestBaseURL(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, item)
}

// requestBaseURL 从当前请求推断对外可访问的站点根地址（兼容反向代理）。
func requestBaseURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); proto != "" {
		scheme = strings.Split(proto, ",")[0]
	}
	host := strings.TrimSpace(c.Request.Host)
	if forwarded := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); forwarded != "" {
		host = strings.Split(forwarded, ",")[0]
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

// List 查询分享链接列表；非管理员只能看到自己创建的链接。
func (h *ShareHandler) List(c *gin.Context) {
	user := middleware.CurrentUser(c)
	items, err := h.share.List(user.ID, service.IsAdmin(middleware.CurrentRole(c)))
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.OK(c, items)
}

// Revoke 撤销分享链接。
func (h *ShareHandler) Revoke(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	user := middleware.CurrentUser(c)
	if err := h.share.Revoke(id, user.ID, service.IsAdmin(middleware.CurrentRole(c))); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"revoked": true})
}

// Public 公开访问分享看板（无需登录），校验 Token 有效期与访问次数。
func (h *ShareHandler) Public(c *gin.Context) {
	token := c.Param("token")
	link, orgIDs, err := h.share.Resolve(token)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	dashboard, err := h.dashboard.Get(link.DashboardID)
	if err != nil {
		response.NotFound(c, "看板不存在")
		return
	}
	startDate, endDate := dateParams(c)
	data, err := h.dashboard.Query(c.Request.Context(), link.DashboardID, startDate, endDate, orgIDs,
		service.QueryOptions{Filters: filterParams(c), ShareMode: true})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{
		"dashboard": dashboard,
		"data":      data,
		"share": gin.H{
			"expires_at": link.ExpiresAt,
			"max_views":  link.MaxViews,
			"views":      link.Views + 1,
		},
	})
}

// PublicDrill 授权分享页的组织下钻，数据范围严格限制在分享链接绑定的组织集合内。
func (h *ShareHandler) PublicDrill(c *gin.Context) {
	token := c.Param("token")
	link, orgIDs, err := h.share.ResolveForRead(token)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	dashboard, err := h.dashboard.Get(link.DashboardID)
	if err != nil {
		response.NotFound(c, "看板不存在")
		return
	}
	if !dashboard.DrillEnabled {
		response.Forbidden(c, "该看板未开启组织下钻")
		return
	}
	orgID, _ := strconv.ParseUint(c.Query("org_id"), 10, 64)
	result, err := h.drill.QueryScoped(
		service.DataScope{OrgIDs: orgIDs}, 0, orgID,
		c.Query("period_type"), c.Query("period"), c.Query("metric"), c.Query("mode"),
	)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

// PublicDrillTrend 授权分享页的趋势图，范围同样限制在分享令牌绑定的组织内。
func (h *ShareHandler) PublicDrillTrend(c *gin.Context) {
	token := c.Param("token")
	link, orgIDs, err := h.share.ResolveForRead(token)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	dashboard, err := h.dashboard.Get(link.DashboardID)
	if err != nil {
		response.NotFound(c, "看板不存在")
		return
	}
	if !dashboard.DrillEnabled {
		response.Forbidden(c, "该看板未开启组织下钻")
		return
	}
	nodeID, _ := strconv.ParseUint(c.Query("node_id"), 10, 64)
	result, err := h.drill.TrendScoped(
		service.DataScope{OrgIDs: orgIDs}, 0, nodeID, c.Query("node_type"),
		c.Query("metric"), c.Query("start_date"), c.Query("end_date"),
	)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}
