package handler

import (
	"strconv"

	"bi-platform/internal/middleware"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/repository"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	service  *service.UserService
	roleRepo *repository.RoleRepository
	scope    *service.ScopeService
}

func NewUserHandler(service *service.UserService, roleRepo *repository.RoleRepository, scope *service.ScopeService) *UserHandler {
	return &UserHandler{service: service, roleRepo: roleRepo, scope: scope}
}

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

func (h *UserHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.service.Delete(id, middleware.CurrentUser(c).ID); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

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

func (h *UserHandler) Export(c *gin.Context) {
	data, err := h.service.Export()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	writeExcel(c, data, "contacts.xlsx")
}

func (h *UserHandler) Template(c *gin.Context) {
	data, err := h.service.Template()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	writeExcel(c, data, "contacts-template.xlsx")
}

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
