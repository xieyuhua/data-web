package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"bi-platform/internal/middleware"
	"bi-platform/internal/model"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/pkg/sqlguard"
	"bi-platform/internal/repository"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

// DashboardHandler 看板接口：列表、详情、增删改、数据查询与导出。
type DashboardHandler struct {
	service *service.DashboardService
	repo    *repository.DashboardRepository
	scope   *service.ScopeService
	pref    *service.DashboardPreferenceService
}

// NewDashboardHandler 创建看板处理器。
func NewDashboardHandler(service *service.DashboardService, repo *repository.DashboardRepository, scope *service.ScopeService, pref *service.DashboardPreferenceService) *DashboardHandler {
	return &DashboardHandler{service: service, repo: repo, scope: scope, pref: pref}
}

// List 查询当前用户可见的看板列表（按发布状态、角色、组织范围过滤）。
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
	// 合并收藏 / 置顶 / 最近浏览等个人偏好，前端首页据此排序与分区展示
	h.applyPreferences(user.ID, result)
	response.OK(c, result)
}

// Get 查询看板详情，未发布或无权限时拒绝访问。
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
	// 详情页同样带回个人偏好，用于渲染收藏按钮的初始状态
	single := []service.DashboardView{*item}
	h.applyPreferences(middleware.CurrentUser(c).ID, single)
	*item = single[0]
	response.OK(c, item)
}

// Create 新建看板。
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

// Update 更新看板；仅创建者或管理员可操作。
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

// Delete 删除看板；仅创建者或管理员可操作。
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
	// 清理所有用户对该看板的收藏 / 浏览记录，避免残留孤立数据
	if h.pref != nil {
		_ = h.pref.DeleteByDashboard(id)
	}
	response.OK(c, gin.H{"deleted": true})
}

// Duplicate 复制看板为当前用户的新看板。
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

// Data 查询看板所有组件的数据，默认最近 30 天。
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

// WidgetTrend 查询单个组件的指标趋势，默认最近 7 天。
// 权限与数据范围复用组织下钻 / 看板的统一校验逻辑，不会因为趋势查询绕过数据边界。
func (h *DashboardHandler) WidgetTrend(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	_, orgIDs, ok := h.authorizedDashboard(c, id)
	if !ok {
		return
	}
	startDate := strings.TrimSpace(c.Query("start_date"))
	endDate := strings.TrimSpace(c.Query("end_date"))
	// 未指定时默认最近 7 天，与组织下钻趋势保持一致
	if startDate == "" {
		startDate = time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}
	result, err := h.service.WidgetTrend(c.Request.Context(), id, service.WidgetTrendRequest{
		WidgetID:   c.Param("widgetId"),
		Field:      c.Query("field"),
		GroupField: c.Query("group_field"),
		GroupValue: c.Query("group_value"),
		StartDate:  startDate,
		EndDate:    endDate,
	}, orgIDs, service.QueryOptions{Filters: filterParams(c)})
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
	filters := filterParams(c)
	data, err := h.service.Query(c.Request.Context(), id, startDate, endDate, orgIDs,
		service.QueryOptions{Filters: filters})
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
	// 导出说明：把口径、范围、筛选与导出人写进 Excel 首页，方便对账与审计
	meta := service.ExportMeta{
		Operator:   exportOperator(middleware.CurrentUser(c)),
		Scope:      exportScope(middleware.CurrentRole(c), orgIDs),
		Filters:    describeFilterValues(filters),
		ExportedAt: time.Now(),
	}
	content, err := service.BuildDashboardExcel(dashboard.Name, data, widgetID, meta)
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

// applyPreferences 把当前用户的看板偏好合并进视图列表（原地修改）。
// 偏好读取失败时不影响看板展示，仅退化为无收藏状态。
func (h *DashboardHandler) applyPreferences(userID uint64, items []service.DashboardView) {
	if h.pref == nil || len(items) == 0 {
		return
	}
	prefs, err := h.pref.Map(userID)
	if err != nil {
		return
	}
	for i := range items {
		if pref, ok := prefs[items[i].ID]; ok {
			items[i].Favorite = pref.Favorite
			items[i].Pinned = pref.Pinned
			items[i].ViewCount = pref.ViewCount
			items[i].LastViewedAt = pref.LastViewedAt
		}
	}
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

// exportOperator 导出人描述：优先「姓名（账号）」，无姓名时退化为账号。
func exportOperator(user *model.User) string {
	if user == nil {
		return ""
	}
	name := strings.TrimSpace(user.Name)
	if name == "" {
		return user.Username
	}
	return fmt.Sprintf("%s（%s）", name, user.Username)
}

// exportScope 数据范围描述：角色名称 + 本次查询实际可见的组织数量。
func exportScope(role *model.Role, orgIDs []uint64) string {
	roleName := "未知角色"
	if role != nil && strings.TrimSpace(role.Name) != "" {
		roleName = role.Name
	}
	return fmt.Sprintf("角色：%s · 可见组织 %d 个", roleName, len(orgIDs))
}

// describeFilterValues 把全局筛选条件转成可读文本，例如「channel=线上、city=上海 / 杭州」。
// 多选值在服务端用 sqlguard.FilterValueSeparator 编码，这里还原为「/」分隔便于阅读。
func describeFilterValues(filters map[string]string) string {
	if len(filters) == 0 {
		return ""
	}
	keys := make([]string, 0, len(filters))
	for key := range filters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := filters[key]
		if strings.Contains(value, sqlguard.FilterValueSeparator) {
			value = strings.Join(strings.Split(value, sqlguard.FilterValueSeparator), " / ")
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, "、")
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

// canManageDashboard 判断当前用户是否可维护指定看板。
func canManageDashboard(c *gin.Context, item *model.Dashboard) bool {
	return service.IsAdmin(middleware.CurrentRole(c)) || item.CreatedBy == middleware.CurrentUser(c).ID
}

// dateParams 解析起止日期，缺省为最近 30 天。
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

// TemplateHandler 看板模板接口。
type TemplateHandler struct {
	service *service.TemplateService
}

// NewTemplateHandler 创建模板处理器。
func NewTemplateHandler(service *service.TemplateService) *TemplateHandler {
	return &TemplateHandler{service: service}
}

// List 查询模板列表。
func (h *TemplateHandler) List(c *gin.Context) {
	items, err := h.service.List()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.OK(c, items)
}

// Create 保存当前看板为模板。
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

// Use 使用模板创建一份新看板。
func (h *TemplateHandler) Use(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	item, err := h.service.CreateDashboard(id, middleware.CurrentUser(c).ID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, item)
}

// Delete 删除模板。
func (h *TemplateHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.service.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
