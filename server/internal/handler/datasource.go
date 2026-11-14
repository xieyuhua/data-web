package handler

import (
	"strconv"

	"bi-platform/internal/middleware"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

// DatasourceHandler 数据源管理接口（仅管理员）。
type DatasourceHandler struct {
	service *service.DatasourceService
}

// NewDatasourceHandler 创建数据源处理器。
func NewDatasourceHandler(service *service.DatasourceService) *DatasourceHandler {
	return &DatasourceHandler{service: service}
}

// List 查询数据源列表（密码字段已脱敏）。
func (h *DatasourceHandler) List(c *gin.Context) {
	items, err := h.service.List()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.OK(c, items)
}

// Create 新增数据源。
func (h *DatasourceHandler) Create(c *gin.Context) {
	var input service.DatasourceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "数据源参数不正确")
		return
	}
	item, err := h.service.Create(input, middleware.CurrentUser(c).ID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, item)
}

// Update 更新数据源。
func (h *DatasourceHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var input service.DatasourceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "数据源参数不正确")
		return
	}
	item, err := h.service.Update(id, input)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, item)
}

// Delete 删除数据源。
func (h *DatasourceHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.service.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Test 测试数据源连通性；可携带未保存的临时连接参数。
func (h *DatasourceHandler) Test(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var input service.DatasourceInput
	_ = c.ShouldBindJSON(&input)
	result, err := h.service.Test(id, input)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}
