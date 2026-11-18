package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/service"
	"biaoge-bi/internal/web"
)

type WeComHandler struct {
	svc       *service.WeComService
	dashboard *service.DashboardService
	audit     *service.AuditService
}

func NewWeComHandler(svc *service.WeComService, dashboard *service.DashboardService, audit *service.AuditService) *WeComHandler {
	return &WeComHandler{svc: svc, dashboard: dashboard, audit: audit}
}

// PublicStatus tells the login page whether SSO is available.
func (h *WeComHandler) PublicStatus(c *gin.Context) {
	web.OK(c, h.svc.Status())
}

// AuthorizeURL returns the QR login URL.
func (h *WeComHandler) AuthorizeURL(c *gin.Context) {
	web.OK(c, gin.H{"url": h.svc.AuthorizeURL(c.Query("state"))})
}

type wecomLoginRequest struct {
	Code string `json:"code"`
}

// Login exchanges an OAuth code for a BI JWT.
func (h *WeComHandler) Login(c *gin.Context) {
	var req wecomLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		web.BadRequest(c, "缺少企业微信授权码")
		return
	}
	res, err := h.svc.Login(c.Request.Context(), req.Code)
	if err != nil {
		h.audit.Log(c, "wecom_login_failed", "user", 0, gin.H{"error": err.Error()})
		web.Unauthorized(c, err.Error())
		return
	}
	h.audit.Log(c, "wecom_login", "user", res.User.ID, gin.H{"wecomUserId": res.User.WeComUserID})
	web.OK(c, res)
}

// Sync triggers a full contacts synchronization (admin only).
func (h *WeComHandler) Sync(c *gin.Context) {
	res, err := h.svc.Sync(c.Request.Context())
	if err != nil {
		web.Unprocessable(c, err.Error())
		return
	}
	h.audit.Log(c, "wecom_sync", "org", 0, gin.H{"departments": res.Departments, "users": res.Users})
	web.OK(c, res)
}

// Departments lists synced departments for pickers.
func (h *WeComHandler) Departments(c *gin.Context) {
	list, err := h.svc.Departments()
	if err != nil {
		web.ServerError(c, "查询部门失败")
		return
	}
	web.OK(c, list)
}

// Status returns sync state for the admin settings page.
func (h *WeComHandler) Status(c *gin.Context) {
	web.OK(c, h.svc.Status())
}

// ---------- Dashboard department grants ----------

type deptGrantPayload struct {
	WeComDeptID int    `json:"wecomDeptId"`
	Permission  string `json:"permission"`
}

func (h *WeComHandler) ListDeptGrants(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	h.guardEdit(c, id, func() {
		grants, gerr := h.dashboard.ListDeptGrants(id)
		if gerr != nil {
			web.ServerError(c, "查询部门授权失败")
			return
		}
		web.OK(c, grants)
	})
}

func (h *WeComHandler) AddDeptGrant(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	var p deptGrantPayload
	if err := c.ShouldBindJSON(&p); err != nil || p.WeComDeptID == 0 {
		web.BadRequest(c, "参数错误")
		return
	}
	h.guardEdit(c, id, func() {
		if aerr := h.dashboard.AddDeptGrant(id, p.WeComDeptID, p.Permission); aerr != nil {
			web.BadRequest(c, aerr.Error())
			return
		}
		h.audit.Log(c, "dashboard_dept_grant", "dashboard", id,
			gin.H{"wecomDeptId": p.WeComDeptID, "permission": p.Permission})
		web.OK(c, gin.H{"message": "已授权部门"})
	})
}

func (h *WeComHandler) RemoveDeptGrant(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	gid, perr := strconv.ParseUint(c.Param("grantId"), 10, 32)
	if perr != nil {
		web.BadRequest(c, "无效的授权 ID")
		return
	}
	h.guardEdit(c, id, func() {
		if rerr := h.dashboard.RemoveDeptGrant(id, uint(gid)); rerr != nil {
			web.ServerError(c, "删除部门授权失败")
			return
		}
		web.OK(c, gin.H{"message": "已取消部门授权"})
	})
}

// guardEdit runs fn only when the current user may edit the dashboard.
func (h *WeComHandler) guardEdit(c *gin.Context, dashboardID uint, fn func()) {
	perm, err := h.dashboard.Access(dashboardID, web.CurrentUser(c))
	if err != nil {
		web.NotFound(c, "仪表盘不存在")
		return
	}
	if perm != "edit" {
		web.Forbidden(c, "无权编辑该仪表盘")
		return
	}
	fn()
}
