package handler

import (
	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/web"
	"biaoge-bi/internal/service"
)

type DatasetHandler struct {
	svc   *service.DatasetService
	audit *service.AuditService
}

func NewDatasetHandler(svc *service.DatasetService, audit *service.AuditService) *DatasetHandler {
	return &DatasetHandler{svc: svc, audit: audit}
}

type datasetPayload struct {
	Name         string                    `json:"name"`
	Description  string                    `json:"description"`
	DataSourceID uint                      `json:"dataSourceId"`
	SQLText      string                    `json:"sqlText"`
	DefaultLimit int                       `json:"defaultLimit"`
	Params       []service.DatasetParamInput `json:"params"`
}

func (h *DatasetHandler) List(c *gin.Context) {
	list, err := h.svc.List(c.Query("keyword"))
	if err != nil {
		web.ServerError(c, "查询数据集失败")
		return
	}
	web.OK(c, list)
}

func (h *DatasetHandler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	ds, err := h.svc.Get(id)
	if err != nil {
		web.NotFound(c, "数据集不存在")
		return
	}
	web.OK(c, ds)
}

func (h *DatasetHandler) Create(c *gin.Context) {
	var p datasetPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	u := web.CurrentUser(c)
	ds, err := h.svc.Create(c.Request.Context(), service.DatasetInput{
		Name: p.Name, Description: p.Description, DataSourceID: p.DataSourceID,
		SQLText: p.SQLText, DefaultLimit: p.DefaultLimit, Params: p.Params,
	}, u.ID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	h.audit.Log(c, "dataset_create", "dataset", ds.ID, gin.H{"name": ds.Name})
	web.OK(c, ds)
}

func (h *DatasetHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	var p datasetPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	ds, err := h.svc.Update(c.Request.Context(), id, service.DatasetInput{
		Name: p.Name, Description: p.Description, DataSourceID: p.DataSourceID,
		SQLText: p.SQLText, DefaultLimit: p.DefaultLimit, Params: p.Params,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	web.OK(c, ds)
}

func (h *DatasetHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if err := h.svc.Delete(id); err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	h.audit.Log(c, "dataset_delete", "dataset", id, nil)
	web.OK(c, gin.H{"message": "已删除"})
}

func (h *DatasetHandler) Preview(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	var req service.PreviewRequest
	_ = c.ShouldBindJSON(&req)
	rs, err := h.svc.Preview(c.Request.Context(), id, req)
	if err != nil {
		web.Unprocessable(c, err.Error())
		return
	}
	web.OK(c, rs)
}

func (h *DatasetHandler) Columns(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	cols, err := h.svc.Columns(id)
	if err != nil {
		web.Unprocessable(c, err.Error())
		return
	}
	web.OK(c, cols)
}
