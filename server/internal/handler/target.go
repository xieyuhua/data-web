package handler

import (
	"net/http"
	"strconv"

	"bi-platform/internal/middleware"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

// TargetHandler 目标任务管理接口（基础任务 / 挑战任务）。
type TargetHandler struct {
	service *service.TargetService
	scope   *service.ScopeService
}

// NewTargetHandler 创建目标任务处理器。
func NewTargetHandler(service *service.TargetService, scope *service.ScopeService) *TargetHandler {
	return &TargetHandler{service: service, scope: scope}
}

// List 查询目标任务列表，按当前用户数据范围过滤。
func (h *TargetHandler) List(c *gin.Context) {
	scope, err := h.scope.Resolve(middleware.CurrentUser(c))
	if err != nil {
		response.ServerError(c, err)
		return
	}
	items, err := h.service.List(c.Query("period_type"), c.Query("period"), c.Query("metric"), c.Query("task_type"), targetScope(scope))
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.OK(c, items)
}

// Achievement 计算目标达成率（实际值 / 目标值）。
func (h *TargetHandler) Achievement(c *gin.Context) {
	scope, err := h.scope.Resolve(middleware.CurrentUser(c))
	if err != nil {
		response.ServerError(c, err)
		return
	}
	result, err := h.service.Achievement(targetScope(scope), c.Query("period_type"), c.Query("period"), c.Query("metric"), c.Query("task_type"))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}

// Periods 查询已有任务的周期列表。
func (h *TargetHandler) Periods(c *gin.Context) {
	items, err := h.service.Periods(c.Query("period_type"))
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.OK(c, items)
}

// Metrics 返回支持的指标字典。
func (h *TargetHandler) Metrics(c *gin.Context) {
	metrics := make([]gin.H, 0, len(service.TargetMetrics))
	for _, item := range service.TargetMetrics {
		metrics = append(metrics, gin.H{"key": item.Key, "label": item.Label, "unit": item.Unit})
	}
	response.OK(c, metrics)
}

// Create 新增目标任务。
func (h *TargetHandler) Create(c *gin.Context) {
	var input service.TargetInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "目标参数不正确")
		return
	}
	item, err := h.service.Create(input, middleware.CurrentUser(c).ID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, item)
}

// Update 更新目标任务。
func (h *TargetHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var input service.TargetInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "目标参数不正确")
		return
	}
	item, err := h.service.Update(id, input)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, item)
}

// Delete 删除目标任务。
func (h *TargetHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.service.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Export 导出全量目标任务 Excel。
func (h *TargetHandler) Export(c *gin.Context) {
	data, err := h.service.Export(nil)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	writeExcel(c, data, "targets.xlsx")
}

// Template 下载任务导入模板。
func (h *TargetHandler) Template(c *gin.Context) {
	data, err := h.service.Template()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	writeExcel(c, data, "target-template.xlsx")
}

// Import 导入目标任务 Excel（按组织+周期+指标+任务类型去重）。
func (h *TargetHandler) Import(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请选择要导入的 Excel 文件")
		return
	}
	if file.Size > 10<<20 {
		response.BadRequest(c, "文件不能超过 10MB")
		return
	}
	reader, err := file.Open()
	if err != nil {
		response.BadRequest(c, "无法读取文件")
		return
	}
	defer reader.Close()
	created, updated, err := h.service.Import(file.Filename, reader)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"created": created, "updated": updated})
}

// targetScope 将数据范围转换为组织 ID 列表；All=true 时返回 nil 表示不限制。
func targetScope(scope service.DataScope) []uint64 {
	if scope.All {
		return nil
	}
	return scope.OrgIDs
}

// writeExcel 以附件形式输出 Excel 文件。
func writeExcel(c *gin.Context, data []byte, filename string) {
	contentType := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, contentType, data)
}
