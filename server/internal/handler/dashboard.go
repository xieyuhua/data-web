package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bi-platform/internal/middleware"
	"bi-platform/internal/model"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/repository"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

type DashboardHandler struct {
	service *service.DashboardService
	repo    *repository.DashboardRepository
	scope   *service.ScopeService
}

func NewDashboardHandler(service *service.DashboardService, repo *repository.DashboardRepository, scope *service.ScopeService) *DashboardHandler {
	return &DashboardHandler{service: service, repo: repo, scope: scope}
}

func (h *DashboardHandler) List(c *gin.Context) {
	items, err := h.service.List(c.Query("keyword"), c.Query("category"))
	if err != nil {
		response.ServerError(c, err)
		return
	}
	user := middleware.CurrentUser(c)
	role := middleware.CurrentRole(c)
	scope, err := h.scope.Resolve(user)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	result := make([]service.DashboardView, 0, len(items))
	for _, item := range items {
		viewRef := item
		if service.CanViewDashboard(user, role, service.DashboardModelFromView(&viewRef), scope.OrgIDs) {
			result = append(result, item)
		}
	}
	response.OK(c, result)
}

func (h *DashboardHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	item, err := h.service.Get(id)
	if err != nil {
		response.NotFound(c, "看板不存在")
		return
	}
	scope, err := h.scope.Resolve(middleware.CurrentUser(c))
	if err != nil {
		response.ServerError(c, err)
		return
	}
	if !service.CanViewDashboard(middleware.CurrentUser(c), middleware.CurrentRole(c), service.DashboardModelFromView(item), scope.OrgIDs) {
		response.Forbidden(c, "看板尚未发布")
		return
	}
	response.OK(c, item)
}

func (h *DashboardHandler) Create(c *gin.Context) {
	var input service.DashboardInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "看板参数不正确")
		return
	}
	item, err := h.service.Create(input, middleware.CurrentUser(c).ID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, item)
}

func (h *DashboardHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	existing, err := h.repo.FindByID(id)
	if err != nil {
		response.NotFound(c, "看板不存在")
		return
	}
	if !canManageDashboard(c, existing) {
		response.Forbidden(c, "只能编辑自己创建的看板")
		return
	}
	var input service.DashboardInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "看板参数不正确")
		return
	}
	item, err := h.service.Update(id, input)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, item)
}

func (h *DashboardHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	existing, err := h.repo.FindByID(id)
	if err != nil {
		response.NotFound(c, "看板不存在")
		return
	}
	if !canManageDashboard(c, existing) {
		response.Forbidden(c, "只能删除自己创建的看板")
		return
	}
	if err := h.service.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

func (h *DashboardHandler) Duplicate(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	existing, err := h.repo.FindByID(id)
	if err != nil {
		response.NotFound(c, "看板不存在")
		return
	}
	if existing.Status != "published" && !service.IsAdmin(middleware.CurrentRole(c)) && existing.CreatedBy != middleware.CurrentUser(c).ID {
		response.Forbidden(c, "无法复制未发布看板")
		return
	}
	item, err := h.service.Duplicate(id, middleware.CurrentUser(c).ID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, item)
}

func (h *DashboardHandler) Data(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	_, orgIDs, ok := h.authorizedDashboard(c, id)
	if !ok {
		return
	}
	startDate, endDate := dateParams(c)
	options := service.QueryOptions{NoCache: c.Query("nocache") == "1", Filters: filterParams(c)}
	result, err := h.service.Query(c.Request.Context(), id, startDate, endDate, orgIDs, options)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

// Export 导出整份看板 Excel 或单个组件 CSV。
// 权限、组织范围与在线查看完全一致，不会绕过 CanViewDashboard。
func (h *DashboardHandler) Export(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	dashboard, orgIDs, ok := h.authorizedDashboard(c, id)
	if !ok {
		return
	}
	startDate, endDate := dateParams(c)
	data, err := h.service.Query(c.Request.Context(), id, startDate, endDate, orgIDs,
		service.QueryOptions{Filters: filterParams(c)})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	format := strings.ToLower(strings.TrimSpace(c.Query("format")))
	widgetID := strings.TrimSpace(c.Query("widget_id"))
	if format == "csv" {
		content, widgetTitle, err := service.BuildDashboardWidgetCSV(data, widgetID)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		filename := exportFileName(dashboard.Name, "csv")
		if widgetTitle != "" {
			filename = exportFileName(dashboard.Name+"_"+widgetTitle, "csv")
		}
		writeDownload(c, "text/csv; charset=utf-8", filename, content)
		return
	}
	content, err := service.BuildDashboardExcel(dashboard.Name, data, widgetID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	writeDownload(c, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		exportFileName(dashboard.Name, "xlsx"), content)
}

// authorizedDashboard 统一完成看板存在性、查看权限与组织数据范围校验。
func (h *DashboardHandler) authorizedDashboard(c *gin.Context, id uint64) (*model.Dashboard, []uint64, bool) {
	dashboard, err := h.repo.FindByID(id)
	if err != nil {
		response.NotFound(c, "看板不存在")
		return nil, nil, false
	}
	scope, err := h.scope.Resolve(middleware.CurrentUser(c))
	if err != nil {
		response.ServerError(c, err)
		return nil, nil, false
	}
	if !service.CanViewDashboard(middleware.CurrentUser(c), middleware.CurrentRole(c), dashboard, scope.OrgIDs) {
		response.Forbidden(c, "无权查看该看板")
		return nil, nil, false
	}
	orgIDs := scope.OrgIDs
	if dashboard.ScopeMode == "all" {
		all, err := h.scope.AllOrgIDs()
		if err != nil {
			response.ServerError(c, err)
			return nil, nil, false
		}
		orgIDs = service.DashboardQueryOrgIDs(dashboard, scope, all)
	}
	return dashboard, orgIDs, true
}

func exportFileName(name, ext string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|', '\r', '\n', '\t':
			return '_'
		}
		return r
	}, strings.TrimSpace(name))
	cleaned = strings.Trim(cleaned, " ._")
	if cleaned == "" {
		cleaned = "dashboard"
	}
	runes := []rune(cleaned)
	if len(runes) > 60 {
		cleaned = string(runes[:60])
	}
	return cleaned + "." + ext
}

// writeDownload 输出附件下载，文件名同时提供 ASCII 回退与 RFC 5987 编码。
func writeDownload(c *gin.Context, contentType, filename string, content []byte) {
	fallback := "export." + strings.TrimPrefix(strings.ToLower(filename[strings.LastIndex(filename, ".")+1:]), ".")
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s",
		fallback, url.PathEscape(filename)))
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, contentType, content)
}

// ToDashboard 数据集一键生成看板。
func (h *DashboardHandler) ToDashboard(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var input service.DatasetDashboardInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "生成看板参数不正确")
		return
	}
	item, err := h.service.CreateFromDataset(id, input, middleware.CurrentUser(c).ID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, item)
}

func canManageDashboard(c *gin.Context, item *model.Dashboard) bool {
	return service.IsAdmin(middleware.CurrentRole(c)) || item.CreatedBy == middleware.CurrentUser(c).ID
}

func dateParams(c *gin.Context) (string, string) {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if startDate == "" {
		startDate = time.Now().AddDate(0, 0, -29).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}
	return startDate, endDate
}

type TemplateHandler struct {
	service *service.TemplateService
}

func NewTemplateHandler(service *service.TemplateService) *TemplateHandler {
	return &TemplateHandler{service: service}
}

func (h *TemplateHandler) List(c *gin.Context) {
	items, err := h.service.List()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.OK(c, items)
}

func (h *TemplateHandler) Create(c *gin.Context) {
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Category    string `json:"category"`
		Theme       string `json:"theme"`
		Cover       string `json:"cover"`
		Layout      any    `json:"layout"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "模板参数不正确")
		return
	}
	layout, _ := json.Marshal(input.Layout)
	item, err := h.service.Create(input.Name, input.Description, input.Category, input.Theme, input.Cover, layout, middleware.CurrentUser(c).ID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, item)
}

func (h *TemplateHandler) Use(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	item, err := h.service.CreateDashboard(id, middleware.CurrentUser(c).ID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, item)
}

func (h *TemplateHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.service.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
