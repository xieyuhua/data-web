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

type OrganizationHandler struct {
	service *service.OrganizationService
	scope   *service.ScopeService
}

func NewOrganizationHandler(service *service.OrganizationService, scope *service.ScopeService) *OrganizationHandler {
	return &OrganizationHandler{service: service, scope: scope}
}

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

func (h *OrganizationHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.service.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

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
