package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/model"
	"biaoge-bi/internal/service"
	"biaoge-bi/internal/web"
)

// setAttachment emits an RFC 5987 encoded filename so non-ASCII names download correctly.
func setAttachment(c *gin.Context, filename string) {
	ascii := make([]rune, 0, len(filename))
	for _, r := range filename {
		if r < 128 {
			ascii = append(ascii, r)
		} else {
			ascii = append(ascii, '_')
		}
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		string(ascii), url.PathEscape(filename)))
}

type ChartHandler struct {
	charts *service.ChartService
	export *service.ExportService
	audit  *service.AuditService
}

func NewChartHandler(charts *service.ChartService, exportSvc *service.ExportService, audit *service.AuditService) *ChartHandler {
	return &ChartHandler{charts: charts, export: exportSvc, audit: audit}
}

type chartPayload struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	DatasetID   uint            `json:"datasetId"`
	Type        string          `json:"type"`
	Config      json.RawMessage `json:"config"`
}

func (h *ChartHandler) List(c *gin.Context) {
	list, err := h.charts.List(c.Query("keyword"))
	if err != nil {
		web.ServerError(c, "查询图表失败")
		return
	}
	web.OK(c, list)
}

func (h *ChartHandler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	ch, err := h.charts.Get(id)
	if err != nil {
		web.NotFound(c, "图表不存在")
		return
	}
	web.OK(c, ch)
}

func (h *ChartHandler) Create(c *gin.Context) {
	var p chartPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	u := web.CurrentUser(c)
	ch, err := h.charts.Create(service.ChartInput{
		Name: p.Name, Description: p.Description, DatasetID: p.DatasetID, Type: p.Type, Config: p.Config,
	}, u.ID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	web.OK(c, ch)
}

func (h *ChartHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	var p chartPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	ch, err := h.charts.Update(id, service.ChartInput{
		Name: p.Name, Description: p.Description, DatasetID: p.DatasetID, Type: p.Type, Config: p.Config,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	web.OK(c, ch)
}

func (h *ChartHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if err := h.charts.Delete(id); err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	h.audit.Log(c, "chart_delete", "chart", id, nil)
	web.OK(c, gin.H{"message": "已删除"})
}

type chartDataRequest struct {
	Params map[string]string `json:"params"`
}

func (h *ChartHandler) Duplicate(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	ch, err := h.charts.Duplicate(id, web.CurrentUser(c).ID)
	if err != nil {
		web.NotFound(c, "图表不存在")
		return
	}
	web.OK(c, ch)
}

func (h *ChartHandler) Data(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	var req chartDataRequest
	_ = c.ShouldBindJSON(&req)
	ch, err := h.charts.Get(id)
	if err != nil {
		web.NotFound(c, "图表不存在")
		return
	}
	rs, err := h.charts.Execute(c.Request.Context(), ch, req.Params)
	if err != nil {
		web.Unprocessable(c, err.Error())
		return
	}
	web.OK(c, rs)
}

// PreviewData executes an unsaved chart config against a dataset.
type chartPreviewRequest struct {
	DatasetID uint            `json:"datasetId"`
	Type      string          `json:"type"`
	Config    json.RawMessage `json:"config"`
	Params    map[string]string `json:"params"`
}

func (h *ChartHandler) PreviewData(c *gin.Context) {
	var req chartPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	cfg := model.ChartConfig{}
	if err := json.Unmarshal(req.Config, &cfg); err != nil {
		web.BadRequest(c, "图表配置不合法")
		return
	}
	if cfg.ChartType == "" {
		cfg.ChartType = req.Type
	}
	tmp := &model.Chart{
		Name: "preview", DatasetID: req.DatasetID, Type: cfg.ChartType,
		ConfigJSON: string(req.Config), Version: 0,
	}
	rs, err := h.charts.Execute(c.Request.Context(), tmp, req.Params)
	if err != nil {
		web.Unprocessable(c, err.Error())
		return
	}
	web.OK(c, rs)
}

func (h *ChartHandler) Export(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	var req chartDataRequest
	_ = c.ShouldBindJSON(&req)
	data, filename, err := h.export.ChartXLSX(c.Request.Context(), id, req.Params)
	if err != nil {
		web.Unprocessable(c, err.Error())
		return
	}
	setAttachment(c, filename)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

// ExportCSV streams a chart result as CSV.
func (h *ChartHandler) ExportCSV(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	var req chartDataRequest
	_ = c.ShouldBindJSON(&req)
	data, filename, err := h.export.ChartCSV(c.Request.Context(), id, req.Params)
	if err != nil {
		web.Unprocessable(c, err.Error())
		return
	}
	setAttachment(c, filename)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", data)
}
