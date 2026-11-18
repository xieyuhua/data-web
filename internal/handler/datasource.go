package handler

import (
	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/model"
	"biaoge-bi/internal/web"
	"biaoge-bi/internal/service"
)

type DataSourceHandler struct {
	svc   *service.DataSourceService
	audit *service.AuditService
}

func NewDataSourceHandler(svc *service.DataSourceService, audit *service.AuditService) *DataSourceHandler {
	return &DataSourceHandler{svc: svc, audit: audit}
}

type dataSourcePayload struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	DSNExtra    string `json:"dsnExtra"`
	OptionsJSON string `json:"optionsJson"`
}

func (p dataSourcePayload) toInput() service.DataSourceInput {
	return service.DataSourceInput{
		Name: p.Name, Type: p.Type, Host: p.Host, Port: p.Port, Database: p.Database,
		Username: p.Username, Password: p.Password, DSNExtra: p.DSNExtra, OptionsJSON: p.OptionsJSON,
	}
}

// view strips secrets before returning to clients.
func viewDataSource(ds model.DataSource) gin.H {
	return gin.H{
		"id":              ds.ID,
		"name":            ds.Name,
		"type":            ds.Type,
		"host":            ds.Host,
		"port":            ds.Port,
		"database":        ds.Database,
		"username":        ds.Username,
		"dsnExtra":        ds.DSNExtra,
		"optionsJson":     ds.OptionsJSON,
		"hasPassword":     ds.HasPassword(),
		"createdBy":       ds.CreatedByID,
		"lastTestedAt":    ds.LastTestedAt,
		"lastTestStatus":  ds.LastTestStatus,
		"lastTestMessage": ds.LastTestMessage,
		"createdAt":       ds.CreatedAt,
		"updatedAt":       ds.UpdatedAt,
	}
}

func (h *DataSourceHandler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		web.ServerError(c, "查询数据源失败")
		return
	}
	items := make([]gin.H, 0, len(list))
	for _, ds := range list {
		items = append(items, viewDataSource(ds))
	}
	web.OK(c, items)
}

func (h *DataSourceHandler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	ds, err := h.svc.Get(id)
	if err != nil {
		web.NotFound(c, "数据源不存在")
		return
	}
	web.OK(c, viewDataSource(*ds))
}

func (h *DataSourceHandler) Create(c *gin.Context) {
	var p dataSourcePayload
	if err := c.ShouldBindJSON(&p); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	u := web.CurrentUser(c)
	ds, err := h.svc.Create(p.toInput(), u.ID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	h.audit.Log(c, "datasource_create", "datasource", ds.ID, gin.H{"name": ds.Name, "type": ds.Type})
	web.OK(c, viewDataSource(*ds))
}

func (h *DataSourceHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	var p dataSourcePayload
	if err := c.ShouldBindJSON(&p); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	ds, err := h.svc.Update(id, p.toInput())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	h.audit.Log(c, "datasource_update", "datasource", id, gin.H{"name": ds.Name})
	web.OK(c, viewDataSource(*ds))
}

func (h *DataSourceHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if err := h.svc.Delete(id); err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	h.audit.Log(c, "datasource_delete", "datasource", id, nil)
	web.OK(c, gin.H{"message": "已删除"})
}

func (h *DataSourceHandler) TestUnsaved(c *gin.Context) {
	var p dataSourcePayload
	if err := c.ShouldBindJSON(&p); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	msg, err := h.svc.TestUnsaved(c.Request.Context(), p.toInput())
	if err != nil {
		web.BadRequest(c, "连接失败: "+err.Error())
		return
	}
	web.OK(c, gin.H{"message": msg})
}

func (h *DataSourceHandler) TestByID(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if err := h.svc.TestByID(c.Request.Context(), id); err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	web.OK(c, gin.H{"message": "连接成功"})
}
