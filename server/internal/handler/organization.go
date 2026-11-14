package handler

import (
	"net/http"
	"strconv"

	"bi-platform/internal/middleware"
	"bi-platform/internal/model"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

// OrganizationHandler 组织架构管理接口。
type OrganizationHandler struct {
	service *service.OrganizationService
	scope   *service.ScopeService
}

// NewOrganizationHandler 创建组织处理器。
func NewOrganizationHandler(service *service.OrganizationService, scope *service.ScopeService) *OrganizationHandler {
	return &OrganizationHandler{service: service, scope: scope}
}

// Tree 返回组织树；非管理员只返回自己数据范围内的组织。
func (h *OrganizationHandler) Tree(c *gin.Context) {
	tree, err := h.service.Tree()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	if service.IsAdmin(middleware.CurrentRole(c)) {
		response.OK(c, tree)
		return
	}
	scope, err := h.scope.Resolve(middleware.CurrentUser(c))
	if err != nil {
		response.ServerError(c, err)
		return
	}
	allowed := make(map[uint64]bool, len(scope.OrgIDs))
	for _, id := range scope.OrgIDs {
		allowed[id] = true
	}
	response.OK(c, filterOrgTree(tree, allowed))
}

// Create 新增组织。
func (h *OrganizationHandler) Create(c *gin.Context) {
	var item model.Organization
	if err := c.ShouldBindJSON(&item); err != nil {
		response.BadRequest(c, "组织参数不正确")
		return
	}
	if err := h.service.Create(&item); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, item)
}

// Update 更新组织。
func (h *OrganizationHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var item model.Organization
	if err := c.ShouldBindJSON(&item); err != nil {
		response.BadRequest(c, "组织参数不正确")
		return
	}
	item.ID = id
	if err := h.service.Update(&item); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, item)
}

// Delete 删除组织（存在下级或用户时拒绝）。
func (h *OrganizationHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.service.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Export 导出组织架构 Excel。
func (h *OrganizationHandler) Export(c *gin.Context) {
	data, err := h.service.Export()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", "attachment; filename=organizations.xlsx")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

// Import 导入组织架构 Excel，按编码新增或更新。
func (h *OrganizationHandler) Import(c *gin.Context) {
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
	count, err := h.service.Import(file.Filename, reader)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"imported": count})
}

// filterOrgTree 按允许的组织 ID 过滤树，保留命中节点的祖先链。
func filterOrgTree(items []model.Organization, allowed map[uint64]bool) []model.Organization {
	result := make([]model.Organization, 0)
	for _, item := range items {
		children := filterOrgTree(item.Children, allowed)
		if allowed[item.ID] || len(children) > 0 {
			item.Children = children
			result = append(result, item)
		}
	}
	return result
}
