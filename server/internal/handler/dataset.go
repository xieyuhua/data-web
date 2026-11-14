package handler

import (
	"strconv"
	"time"

	"bi-platform/internal/middleware"
	"bi-platform/internal/model"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

// DatasetHandler 数据集（SQL 模型）管理接口。
type DatasetHandler struct {
	service *service.DatasetService
	scope   *service.ScopeService
}

// NewDatasetHandler 创建数据集处理器。
func NewDatasetHandler(service *service.DatasetService, scope *service.ScopeService) *DatasetHandler {
	return &DatasetHandler{service: service, scope: scope}
}

// List 查询数据集列表；非管理员不返回 SQL 文本，避免泄露底层查询逻辑。
func (h *DatasetHandler) List(c *gin.Context) {
	items, err := h.service.List()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	if !service.IsAdmin(middleware.CurrentRole(c)) {
		for i := range items {
			items[i].SQLText = ""
		}
	}
	response.OK(c, items)
}

// Create 新增数据集。
func (h *DatasetHandler) Create(c *gin.Context) {
	var item model.Dataset
	if err := c.ShouldBindJSON(&item); err != nil {
		response.BadRequest(c, "数据集参数不正确")
		return
	}
	item.CreatedBy = middleware.CurrentUser(c).ID
	if err := h.service.Create(&item); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, item)
}

// Update 更新数据集。
func (h *DatasetHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var item model.Dataset
	if err := c.ShouldBindJSON(&item); err != nil {
		response.BadRequest(c, "数据集参数不正确")
		return
	}
	item.ID = id
	if err := h.service.Update(&item); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, item)
}

// Delete 删除数据集。
func (h *DatasetHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.service.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Preview 预览已保存数据集的结果，默认最近 7 天并自动注入数据权限。
func (h *DatasetHandler) Preview(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	scope, err := h.scope.Resolve(middleware.CurrentUser(c))
	if err != nil {
		response.ServerError(c, err)
		return
	}
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}
	if startDate == "" {
		startDate = time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	}
	result, err := h.service.Preview(c.Request.Context(), id, scope.OrgIDs, startDate, endDate)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

// PreviewDraft 执行尚未保存的 SQL 草稿，便于新建 / 编辑数据模型时即时查看查询效果。
func (h *DatasetHandler) PreviewDraft(c *gin.Context) {
	var input struct {
		SQLText      string  `json:"sql_text"`
		RequireScope bool    `json:"require_scope"`
		ScopeColumn  string  `json:"scope_column"`
		DatasourceID *uint64 `json:"datasource_id"`
		StartDate    string  `json:"start_date"`
		EndDate      string  `json:"end_date"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "预览参数不正确")
		return
	}
	scope, err := h.scope.Resolve(middleware.CurrentUser(c))
	if err != nil {
		response.ServerError(c, err)
		return
	}
	startDate := input.StartDate
	endDate := input.EndDate
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}
	if startDate == "" {
		startDate = time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	}
	dataset := &model.Dataset{
		SQLText: input.SQLText, RequireScope: input.RequireScope,
		ScopeColumn: input.ScopeColumn, DatasourceID: input.DatasourceID,
	}
	result, err := h.service.PreviewDraft(c.Request.Context(), dataset, scope.OrgIDs, startDate, endDate)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}
