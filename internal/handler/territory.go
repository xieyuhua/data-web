package handler

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/export"
	"biaoge-bi/internal/service"
	"biaoge-bi/internal/web"
)

type TerritoryHandler struct {
	svc   *service.TerritoryService
	org   *service.OrgImportService
	audit *service.AuditService
}

func NewTerritoryHandler(svc *service.TerritoryService, org *service.OrgImportService, audit *service.AuditService) *TerritoryHandler {
	return &TerritoryHandler{svc: svc, org: org, audit: audit}
}

// ---------- Big regions ----------

func (h *TerritoryHandler) ListBigRegions(c *gin.Context) {
	list, err := h.svc.ListBigRegions(c.Query("keyword"))
	if err != nil {
		web.ServerError(c, "查询大区失败")
		return
	}
	web.OK(c, list)
}

func (h *TerritoryHandler) SaveBigRegion(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	var in service.BigRegionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	row, err := h.svc.SaveBigRegion(uint(id), in)
	if err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	h.audit.Log(c, "big_region_save", "big_region", row.ID, gin.H{"code": row.Code})
	web.OK(c, row)
}

func (h *TerritoryHandler) DeleteBigRegion(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if err := h.svc.DeleteBigRegion(id); err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	web.OK(c, gin.H{"message": "已删除"})
}

// ---------- Regions ----------

func (h *TerritoryHandler) ListRegions(c *gin.Context) {
	bigID, _ := strconv.ParseUint(c.Query("bigRegionId"), 10, 32)
	list, err := h.svc.ListRegions(c.Query("keyword"), uint(bigID))
	if err != nil {
		web.ServerError(c, "查询区域失败")
		return
	}
	web.OK(c, list)
}

func (h *TerritoryHandler) SaveRegion(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	var in service.RegionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	row, err := h.svc.SaveRegion(uint(id), in)
	if err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	h.audit.Log(c, "region_save", "region", row.ID, gin.H{"code": row.Code})
	web.OK(c, row)
}

func (h *TerritoryHandler) DeleteRegion(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if err := h.svc.DeleteRegion(id); err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	web.OK(c, gin.H{"message": "已删除"})
}

// ---------- Stores ----------

func (h *TerritoryHandler) ListStores(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	list, _, err := h.svc.ListStores(
		c.Query("keyword"), c.Query("bigRegionCode"), c.Query("regionCode"), c.Query("status"),
		page, pageSize,
	)
	if err != nil {
		web.ServerError(c, "查询门店失败")
		return
	}
	// Total must reflect the unfiltered-by-page count, so re-query.
	_, total, _ := h.svc.ListStores(
		c.Query("keyword"), c.Query("bigRegionCode"), c.Query("regionCode"), c.Query("status"),
		1, 200,
	)
	if total == 200 {
		total = 200 // cap; UI shows "200+"
	}
	web.Page(c, list, int64(total), page, pageSize)
}

func (h *TerritoryHandler) GetStore(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	row, err := h.svc.GetStore(id)
	if err != nil {
		web.NotFound(c, err.Error())
		return
	}
	web.OK(c, row)
}

func (h *TerritoryHandler) SaveStore(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	var in service.StoreInput
	if err := c.ShouldBindJSON(&in); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	row, err := h.svc.SaveStore(uint(id), in)
	if err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	h.audit.Log(c, "store_save", "store", row.ID, gin.H{"code": row.Code, "region": row.RegionCode})
	web.OK(c, row)
}

func (h *TerritoryHandler) DeleteStore(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if err := h.svc.DeleteStore(id); err != nil {
		web.ServerError(c, "删除门店失败")
		return
	}
	web.OK(c, gin.H{"message": "已删除"})
}

func (h *TerritoryHandler) ExportStores(c *gin.Context) {
	data, err := h.svc.ExportStores()
	if err != nil {
		web.ServerError(c, "导出门店失败")
		return
	}
	setAttachment(c, "门店列表.xlsx")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

func (h *TerritoryHandler) ImportStores(c *gin.Context) {
	data, err := readUpload(c, "file")
	if err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	res, ierr := h.svc.ImportStores(data)
	if ierr != nil {
		web.BadRequest(c, ierr.Error())
		return
	}
	h.audit.Log(c, "store_import", "store", 0, gin.H{
		"total": res.Total, "created": res.Created, "updated": res.Updated, "failed": res.Failed,
	})
	c.Header("X-Import-Total", strconv.Itoa(res.Total))
	c.Header("X-Import-Created", strconv.Itoa(res.Created))
	c.Header("X-Import-Updated", strconv.Itoa(res.Updated))
	c.Header("X-Import-Failed", strconv.Itoa(res.Failed))
	web.OK(c, gin.H{
		"total": res.Total, "created": res.Created, "updated": res.Updated, "failed": res.Failed,
		"issues": res.Issues,
	})
}

// StoreTemplate returns an empty workbook with the expected headers.
func (h *TerritoryHandler) StoreTemplate(c *gin.Context) {
	data, err := export.WriteSheet("门店", []string{
		"门店编码", "门店名称", "大区编码", "区域编码", "地址", "联系人", "电话", "状态(active/closed)", "开业日期(YYYY-MM-DD)", "备注",
	}, []export.Cell{{"SH001", "上海南京东路店", "HD", "HD-SH", "上海市黄浦区南京东路 100 号", "张三", "13800000000", "active", "2026-01-01", ""}})
	if err != nil {
		web.ServerError(c, "生成模板失败")
		return
	}
	setAttachment(c, "门店导入模板.xlsx")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

func (h *TerritoryHandler) StoreReport(c *gin.Context) {
	data, err := readUpload(c, "file")
	if err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	res, ierr := h.svc.ImportStores(data)
	if ierr != nil {
		web.BadRequest(c, ierr.Error())
		return
	}
	setAttachment(c, "门店导入核对报告.xlsx")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", res.ReportXLSX)
}

// ---------- Organization import / export ----------

func (h *TerritoryHandler) ExportOrg(c *gin.Context) {
	data, err := h.org.ExportOrg()
	if err != nil {
		web.ServerError(c, "导出组织架构失败")
		return
	}
	setAttachment(c, "组织架构.xlsx")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

func (h *TerritoryHandler) ImportOrg(c *gin.Context) {
	data, err := readUpload(c, "file")
	if err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	res, ierr := h.org.ImportOrg(data)
	if ierr != nil {
		web.BadRequest(c, ierr.Error())
		return
	}
	h.audit.Log(c, "org_import", "org", 0, gin.H{
		"total": res.Total, "created": res.Created, "updated": res.Updated, "failed": res.Failed,
	})
	web.OK(c, gin.H{
		"total": res.Total, "created": res.Created, "updated": res.Updated,
		"failed": res.Failed, "issues": res.Issues,
	})
}

func (h *TerritoryHandler) OrgReport(c *gin.Context) {
	data, err := readUpload(c, "file")
	if err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	res, ierr := h.org.ImportOrg(data)
	if ierr != nil {
		web.BadRequest(c, ierr.Error())
		return
	}
	setAttachment(c, "组织架构导入核对报告.xlsx")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", res.ReportXLSX)
}

// ---------- helpers ----------

func readUpload(c *gin.Context, field string) ([]byte, error) {
	fh, err := c.FormFile(field)
	if err != nil {
		return nil, fmt.Errorf("请选择要上传的 Excel 文件")
	}
	if fh.Size > 10<<20 {
		return nil, fmt.Errorf("文件不能超过 10MB")
	}
	name := strings.ToLower(fh.Filename)
	if !strings.HasSuffix(name, ".xlsx") && !strings.HasSuffix(name, ".xlsm") {
		return nil, fmt.Errorf("仅支持 .xlsx 文件")
	}
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 10<<20))
}
