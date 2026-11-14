package handler

import (
	"strconv"

	"bi-platform/internal/middleware"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/repository"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

// UserHandler 用户（通讯录）管理接口。
type UserHandler struct {
	service  *service.UserService
	roleRepo *repository.RoleRepository
	scope    *service.ScopeService
}

// NewUserHandler 创建用户处理器。
func NewUserHandler(service *service.UserService, roleRepo *repository.RoleRepository, scope *service.ScopeService) *UserHandler {
	return &UserHandler{service: service, roleRepo: roleRepo, scope: scope}
}

// List 查询用户列表；按当前用户数据范围过滤。
func (h *UserHandler) List(c *gin.Context) {
	scope, err := h.scope.Resolve(middleware.CurrentUser(c))
	if err != nil {
		response.ServerError(c, err)
		return
	}
	users, err := h.service.List(c.Query("keyword"), c.Query("role"), scope)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.OK(c, users)
}

// Create 新增用户。
func (h *UserHandler) Create(c *gin.Context) {
	var input service.UserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "用户参数不正确")
		return
	}
	user, err := h.service.Create(input)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, user)
}

// Update 更新用户。
func (h *UserHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var input service.UserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "用户参数不正确")
		return
	}
	user, err := h.service.Update(id, input)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, user)
}

// Delete 删除用户（禁止删除自己）。
func (h *UserHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.service.Delete(id, middleware.CurrentUser(c).ID); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Roles 返回角色列表（不返回权限串）。
func (h *UserHandler) Roles(c *gin.Context) {
	roles, err := h.roleRepo.All()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	for i := range roles {
		roles[i].Permissions = ""
	}
	response.OK(c, roles)
}

// Export 导出通讯录 Excel。
func (h *UserHandler) Export(c *gin.Context) {
	data, err := h.service.Export()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	writeExcel(c, data, "contacts.xlsx")
}

// Template 下载通讯录导入模板。
func (h *UserHandler) Template(c *gin.Context) {
	data, err := h.service.Template()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	writeExcel(c, data, "contacts-template.xlsx")
}

// Import 导入通讯录 Excel。
func (h *UserHandler) Import(c *gin.Context) {
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
	result, err := h.service.Import(file.Filename, reader)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}
