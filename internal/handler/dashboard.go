package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/model"
	"biaoge-bi/internal/web"
	"biaoge-bi/internal/service"
)

type DashboardHandler struct {
	svc    *service.DashboardService
	share  *service.ShareService
	export *service.ExportService
	audit  *service.AuditService
}

func NewDashboardHandler(svc *service.DashboardService, share *service.ShareService, exportSvc *service.ExportService, audit *service.AuditService) *DashboardHandler {
	return &DashboardHandler{svc: svc, share: share, export: exportSvc, audit: audit}
}

type dashboardPayload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *DashboardHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	list, total, err := h.svc.List(web.CurrentUser(c), c.Query("keyword"), page, pageSize)
	if err != nil {
		web.ServerError(c, "查询仪表盘失败")
		return
	}
	web.Page(c, list, total, page, pageSize)
}

func (h *DashboardHandler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	d, err := h.svc.Get(id, web.CurrentUser(c))
	if err != nil {
		writeDashboardError(c, err)
		return
	}
	perm, _ := h.svc.Access(id, web.CurrentUser(c))
	bindings, _ := h.svc.Bindings(id)
	web.OK(c, gin.H{"dashboard": d, "permission": perm, "bindings": bindings})
}

func (h *DashboardHandler) Create(c *gin.Context) {
	var p dashboardPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	d, err := h.svc.Create(service.DashboardInput{Name: p.Name, Description: p.Description}, web.CurrentUser(c).ID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	h.audit.Log(c, "dashboard_create", "dashboard", d.ID, gin.H{"name": d.Name})
	web.OK(c, d)
}

func (h *DashboardHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if !h.requireEdit(c, id) {
		return
	}
	var p dashboardPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	d, err := h.svc.Update(id, service.DashboardInput{Name: p.Name, Description: p.Description})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	web.OK(c, d)
}

func (h *DashboardHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if !h.requireEdit(c, id) {
		return
	}
	if err := h.svc.Delete(id); err != nil {
		web.ServerError(c, "删除仪表盘失败")
		return
	}
	web.OK(c, gin.H{"message": "已删除"})
}

func (h *DashboardHandler) Duplicate(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if _, err := h.svc.Get(id, web.CurrentUser(c)); err != nil {
		writeDashboardError(c, err)
		return
	}
	d, err := h.svc.Duplicate(id, web.CurrentUser(c).ID)
	if err != nil {
		web.ServerError(c, "复制仪表盘失败")
		return
	}
	web.OK(c, d)
}

func (h *DashboardHandler) SaveLayout(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if !h.requireEdit(c, id) {
		return
	}
	var in service.LayoutInput
	if err := c.ShouldBindJSON(&in); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	d, err := h.svc.SaveLayout(id, in)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	web.OK(c, d)
}

type bindingsPayload struct {
	Bindings []service.BindingInput `json:"bindings"`
}

func (h *DashboardHandler) SaveBindings(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if !h.requireEdit(c, id) {
		return
	}
	var p bindingsPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.SaveBindings(id, p.Bindings); err != nil {
		writeServiceError(c, err)
		return
	}
	web.OK(c, gin.H{"message": "联动已保存"})
}

func (h *DashboardHandler) Data(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if _, err := h.svc.Get(id, web.CurrentUser(c)); err != nil {
		writeDashboardError(c, err)
		return
	}
	var req service.DashboardDataRequest
	_ = c.ShouldBindJSON(&req)
	data, err := h.svc.ExecuteData(c.Request.Context(), id, req)
	if err != nil {
		writeDashboardError(c, err)
		return
	}
	web.OK(c, data)
}

func (h *DashboardHandler) FilterOptions(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	fid, err2 := strconv.ParseUint(c.Param("filterId"), 10, 32)
	if err2 != nil {
		web.BadRequest(c, "无效的筛选器 ID")
		return
	}
	if _, err := h.svc.Get(id, web.CurrentUser(c)); err != nil {
		writeDashboardError(c, err)
		return
	}
	options, err := h.svc.FilterOptions(c.Request.Context(), id, uint(fid))
	if err != nil {
		web.Unprocessable(c, err.Error())
		return
	}
	web.OK(c, options)
}

func (h *DashboardHandler) Export(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if _, err := h.svc.Get(id, web.CurrentUser(c)); err != nil {
		writeDashboardError(c, err)
		return
	}
	var req service.DashboardDataRequest
	_ = c.ShouldBindJSON(&req)
	data, filename, err := h.export.DashboardXLSX(c.Request.Context(), id, req)
	if err != nil {
		web.Unprocessable(c, err.Error())
		return
	}
	setAttachment(c, filename)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

// ---------- Grants ----------

type grantPayload struct {
	UserID     uint   `json:"userId"`
	Permission string `json:"permission"`
}

func (h *DashboardHandler) ListGrants(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if !h.requireEdit(c, id) {
		return
	}
	var grants []model.DashboardGrant
	if err := h.svc.ListGrants(id, &grants); err != nil {
		web.ServerError(c, "查询授权失败")
		return
	}
	web.OK(c, grants)
}

func (h *DashboardHandler) AddGrant(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if !h.requireEdit(c, id) {
		return
	}
	var p grantPayload
	if err := c.ShouldBindJSON(&p); err != nil || p.UserID == 0 {
		web.BadRequest(c, "参数错误")
		return
	}
	if p.Permission != "edit" {
		p.Permission = "view"
	}
	if err := h.svc.AddGrant(id, p.UserID, p.Permission); err != nil {
		writeServiceError(c, err)
		return
	}
	h.audit.Log(c, "dashboard_grant", "dashboard", id, gin.H{"userId": p.UserID, "permission": p.Permission})
	web.OK(c, gin.H{"message": "已授权"})
}

func (h *DashboardHandler) RemoveGrant(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if !h.requireEdit(c, id) {
		return
	}
	gid, err2 := strconv.ParseUint(c.Param("grantId"), 10, 32)
	if err2 != nil {
		web.BadRequest(c, "无效的授权 ID")
		return
	}
	if err := h.svc.RemoveGrant(id, uint(gid)); err != nil {
		web.ServerError(c, "删除授权失败")
		return
	}
	web.OK(c, gin.H{"message": "已取消授权"})
}

// ---------- Shares ----------

type sharePayload struct {
	ExpiresInDays int `json:"expiresInDays"`
}

func (h *DashboardHandler) ListShares(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if !h.requireEdit(c, id) {
		return
	}
	links, err := h.share.List(id)
	if err != nil {
		web.ServerError(c, "查询分享链接失败")
		return
	}
	web.OK(c, links)
}

func (h *DashboardHandler) CreateShare(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if !h.requireEdit(c, id) {
		return
	}
	var p sharePayload
	_ = c.ShouldBindJSON(&p)
	var expires *time.Time
	if p.ExpiresInDays > 0 {
		t := time.Now().AddDate(0, 0, p.ExpiresInDays)
		expires = &t
	}
	link, _, err := h.share.Create(id, web.CurrentUser(c).ID, expires)
	if err != nil {
		web.ServerError(c, "创建分享链接失败")
		return
	}
	h.audit.Log(c, "share_create", "dashboard", id, gin.H{"shareId": link.ID, "expiresAt": link.ExpiresAt})
	web.OK(c, link)
}

func (h *DashboardHandler) RevokeShare(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if !h.requireEdit(c, id) {
		return
	}
	sid, err2 := strconv.ParseUint(c.Param("shareId"), 10, 32)
	if err2 != nil {
		web.BadRequest(c, "无效的分享 ID")
		return
	}
	if err := h.share.Revoke(uint(sid), id); err != nil {
		web.ServerError(c, "撤销分享失败")
		return
	}
	h.audit.Log(c, "share_revoke", "dashboard", id, gin.H{"shareId": sid})
	web.OK(c, gin.H{"message": "已撤销"})
}

func (h *DashboardHandler) requireEdit(c *gin.Context, id uint) bool {
	perm, err := h.svc.Access(id, web.CurrentUser(c))
	if err != nil {
		if errors.Is(err, service.ErrDashboardNotFound) {
			web.NotFound(c, "仪表盘不存在")
			return false
		}
		web.ServerError(c, "校验权限失败")
		return false
	}
	if perm != "edit" {
		web.Forbidden(c, "无权编辑该仪表盘")
		return false
	}
	return true
}

func writeDashboardError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrDashboardNotFound):
		web.NotFound(c, "仪表盘不存在")
	case errors.Is(err, service.ErrDashboardForbidden):
		web.Forbidden(c, "无权访问该仪表盘")
	default:
		web.ServerError(c, "操作失败")
	}
}
