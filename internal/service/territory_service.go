package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"biaoge-bi/internal/export"
	"biaoge-bi/internal/model"
	"biaoge-bi/internal/pinyin"
	"biaoge-bi/internal/repository"
)

// TerritoryService manages big regions, regions and stores.
type TerritoryService struct {
	repo *repository.Repo
}

func NewTerritoryService(repo *repository.Repo) *TerritoryService {
	return &TerritoryService{repo: repo}
}

// ---------- Big regions ----------

type BigRegionInput struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	SortOrder int    `json:"sortOrder"`
	Remarks   string `json:"remarks"`
}

func (s *TerritoryService) ListBigRegions(keyword string) ([]model.BigRegion, error) {
	q := s.repo.DB.Order("sort_order, code")
	if keyword != "" {
		kw := "%" + strings.ToLower(keyword) + "%"
		q = q.Where("LOWER(code) LIKE ? OR LOWER(name) LIKE ? OR pinyin LIKE ? OR initials LIKE ?",
			kw, kw, kw, kw)
	}
	var list []model.BigRegion
	err := q.Find(&list).Error
	return list, err
}

func (s *TerritoryService) SaveBigRegion(id uint, in BigRegionInput) (*model.BigRegion, error) {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	if in.Code == "" || in.Name == "" {
		return nil, errors.New("大区编码和名称不能为空")
	}
	full, initials := pinyin.Forms(in.Name)

	var row model.BigRegion
	if id > 0 {
		if err := s.repo.DB.First(&row, id).Error; err != nil {
			return nil, errors.New("大区不存在")
		}
	} else {
		var n int64
		s.repo.DB.Model(&model.BigRegion{}).Where("code = ?", in.Code).Count(&n)
		if n > 0 {
			return nil, errors.New("大区编码已存在")
		}
	}
	row.Code = in.Code
	row.Name = in.Name
	row.Pinyin = full
	row.Initials = initials
	row.SortOrder = in.SortOrder
	row.Remarks = in.Remarks
	if err := s.repo.DB.Save(&row).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, errors.New("大区编码已存在")
		}
		return nil, err
	}
	return &row, nil
}

func (s *TerritoryService) DeleteBigRegion(id uint) error {
	var regions int64
	if err := s.repo.DB.Model(&model.Region{}).Where("big_region_id = ?", id).Count(&regions).Error; err != nil {
		return err
	}
	if regions > 0 {
		return fmt.Errorf("该大区下还有 %d 个区域，请先删除或迁移", regions)
	}
	var stores int64
	if err := s.repo.DB.Model(&model.Store{}).Where("big_region_id = ?", id).Count(&stores).Error; err != nil {
		return err
	}
	if stores > 0 {
		return fmt.Errorf("该大区下还有 %d 家门店", stores)
	}
	return s.repo.DB.Delete(&model.BigRegion{}, id).Error
}

// ---------- Regions ----------

type RegionInput struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	BigRegionID uint   `json:"bigRegionId"`
	SortOrder   int    `json:"sortOrder"`
	Remarks     string `json:"remarks"`
}

func (s *TerritoryService) ListRegions(keyword string, bigRegionID uint) ([]model.Region, error) {
	q := s.repo.DB.Order("sort_order, code")
	if bigRegionID > 0 {
		q = q.Where("big_region_id = ?", bigRegionID)
	}
	if keyword != "" {
		kw := "%" + strings.ToLower(keyword) + "%"
		q = q.Where("LOWER(code) LIKE ? OR LOWER(name) LIKE ? OR pinyin LIKE ? OR initials LIKE ?",
			kw, kw, kw, kw)
	}
	var list []model.Region
	err := q.Find(&list).Error
	return list, err
}

func (s *TerritoryService) SaveRegion(id uint, in RegionInput) (*model.Region, error) {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	if in.Code == "" || in.Name == "" {
		return nil, errors.New("区域编码和名称不能为空")
	}
	if in.BigRegionID == 0 {
		return nil, errors.New("请选择所属大区")
	}
	var big model.BigRegion
	if err := s.repo.DB.First(&big, in.BigRegionID).Error; err != nil {
		return nil, errors.New("所属大区不存在")
	}
	full, initials := pinyin.Forms(in.Name)

	var row model.Region
	if id > 0 {
		if err := s.repo.DB.First(&row, id).Error; err != nil {
			return nil, errors.New("区域不存在")
		}
		if row.Code != in.Code {
			var n int64
			s.repo.DB.Model(&model.Store{}).Where("region_id = ?", id).Count(&n)
			if n > 0 {
				return nil, fmt.Errorf("该区域下还有 %d 家门店，不能修改区域编码", n)
			}
		}
	} else {
		var n int64
		s.repo.DB.Model(&model.Region{}).Where("code = ?", in.Code).Count(&n)
		if n > 0 {
			return nil, errors.New("区域编码已存在")
		}
	}
	row.Code = in.Code
	row.Name = in.Name
	row.BigRegionID = in.BigRegionID
	row.Pinyin = full
	row.Initials = initials
	row.SortOrder = in.SortOrder
	row.Remarks = in.Remarks
	if err := s.repo.DB.Save(&row).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, errors.New("区域编码已存在")
		}
		return nil, err
	}
	// Keep denormalized codes on stores in sync.
	if err := s.repo.DB.Model(&model.Store{}).Where("region_id = ?", row.ID).Updates(map[string]any{
		"region_code":     row.Code,
		"big_region_code": big.Code,
		"big_region_id":   big.ID,
	}).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *TerritoryService) DeleteRegion(id uint) error {
	var stores int64
	if err := s.repo.DB.Model(&model.Store{}).Where("region_id = ?", id).Count(&stores).Error; err != nil {
		return err
	}
	if stores > 0 {
		return fmt.Errorf("该区域下还有 %d 家门店，请先删除或迁移", stores)
	}
	return s.repo.DB.Delete(&model.Region{}, id).Error
}

// ---------- Stores ----------

type StoreInput struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	RegionID    uint   `json:"regionId"`
	Address     string `json:"address"`
	Contact     string `json:"contact"`
	Phone       string `json:"phone"`
	Status      string `json:"status"`
	OpenedAt    string `json:"openedAt"`
	Remarks     string `json:"remarks"`
}

type StoreView struct {
	model.Store
	BigRegionName string `json:"bigRegionName"`
	RegionName    string `json:"regionName"`
}

func (s *TerritoryService) ListStores(keyword, bigRegionCode, regionCode, status string, page, pageSize int) ([]StoreView, int64, error) {
	q := s.repo.DB.Model(&model.Store{})
	if keyword != "" {
		kw := "%" + strings.ToLower(keyword) + "%"
		q = q.Where("LOWER(code) LIKE ? OR LOWER(name) LIKE ? OR LOWER(address) LIKE ?", kw, kw, kw)
	}
	if bigRegionCode != "" {
		q = q.Where("big_region_code = ?", bigRegionCode)
	}
	if regionCode != "" {
		q = q.Where("region_code = ?", regionCode)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	var stores []model.Store
	if err := q.Order("big_region_code, region_code, code").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&stores).Error; err != nil {
		return nil, 0, err
	}
	return s.decorate(stores)
}

func (s *TerritoryService) decorate(stores []model.Store) ([]StoreView, int64, error) {
	var bigs []model.BigRegion
	var regions []model.Region
	_ = s.repo.DB.Find(&bigs).Error
	_ = s.repo.DB.Find(&regions).Error
	bigName := map[uint]string{}
	for _, b := range bigs {
		bigName[b.ID] = b.Name
	}
	regionName := map[uint]string{}
	for _, r := range regions {
		regionName[r.ID] = r.Name
	}
	out := make([]StoreView, 0, len(stores))
	for _, st := range stores {
		out = append(out, StoreView{
			Store: st, BigRegionName: bigName[st.BigRegionID], RegionName: regionName[st.RegionID],
		})
	}
	return out, int64(len(out)), nil
}

func (s *TerritoryService) SaveStore(id uint, in StoreInput) (*model.Store, error) {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	if in.Code == "" || in.Name == "" {
		return nil, errors.New("门店编码和名称不能为空")
	}
	if in.RegionID == 0 {
		return nil, errors.New("请选择所属区域")
	}
	var region model.Region
	if err := s.repo.DB.First(&region, in.RegionID).Error; err != nil {
		return nil, errors.New("所属区域不存在")
	}
	var big model.BigRegion
	if err := s.repo.DB.First(&big, region.BigRegionID).Error; err != nil {
		return nil, errors.New("区域所属大区不存在")
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if in.Status != "active" && in.Status != "closed" {
		return nil, errors.New("门店状态只能是 active 或 closed")
	}

	var row model.Store
	if id > 0 {
		if err := s.repo.DB.First(&row, id).Error; err != nil {
			return nil, errors.New("门店不存在")
		}
	} else {
		var n int64
		s.repo.DB.Model(&model.Store{}).Where("code = ?", in.Code).Count(&n)
		if n > 0 {
			return nil, errors.New("门店编码已存在")
		}
	}
	row.Code = in.Code
	row.Name = in.Name
	row.RegionID = region.ID
	row.RegionCode = region.Code
	row.BigRegionID = big.ID
	row.BigRegionCode = big.Code
	row.Address = in.Address
	row.Contact = in.Contact
	row.Phone = in.Phone
	row.Status = in.Status
	row.Remarks = in.Remarks
	if in.OpenedAt != "" {
		t, err := parseDateOnly(in.OpenedAt)
		if err != nil {
			return nil, errors.New("开业日期格式应为 YYYY-MM-DD")
		}
		row.OpenedAt = &t
	} else {
		row.OpenedAt = nil
	}
	if err := s.repo.DB.Save(&row).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, errors.New("门店编码已存在")
		}
		return nil, err
	}
	return &row, nil
}

func (s *TerritoryService) DeleteStore(id uint) error {
	return s.repo.DB.Delete(&model.Store{}, id).Error
}

func (s *TerritoryService) GetStore(id uint) (*StoreView, error) {
	var row model.Store
	if err := s.repo.DB.First(&row, id).Error; err != nil {
		return nil, errors.New("门店不存在")
	}
	views, _, err := s.decorate([]model.Store{row})
	if err != nil || len(views) == 0 {
		return nil, err
	}
	return &views[0], nil
}

// Ensure big/region code pairs used by importers.
func (s *TerritoryService) FindBigRegionByCode(code string) (*model.BigRegion, error) {
	var row model.BigRegion
	if err := s.repo.DB.Where("code = ?", code).First(&row).Error; err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, nil
}

func (s *TerritoryService) FindRegionByCode(code string) (*model.Region, error) {
	var row model.Region
	if err := s.repo.DB.Where("code = ?", code).First(&row).Error; err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, nil
}


// parseDateOnly accepts YYYY-MM-DD (the format used by imports and exports).
func parseDateOnly(v string) (time.Time, error) {
	return time.Parse("2006-01-02", strings.TrimSpace(v))
}

// ---------- Store import / export ----------

var storeHeader = []string{
	"门店编码", "门店名称", "大区编码", "区域编码", "地址", "联系人", "电话", "状态(active/closed)", "开业日期(YYYY-MM-DD)", "备注",
}

// ExportStores renders every store for reconciliation with the ERP/source system.
func (s *TerritoryService) ExportStores() ([]byte, error) {
	var stores []model.Store
	if err := s.repo.DB.Order("big_region_code, region_code, code").Find(&stores).Error; err != nil {
		return nil, err
	}
	rows := make([]export.Cell, 0, len(stores))
	for _, st := range stores {
		opened := ""
		if st.OpenedAt != nil {
			opened = st.OpenedAt.Format("2006-01-02")
		}
		rows = append(rows, export.Cell{
			st.Code, st.Name, st.BigRegionCode, st.RegionCode,
			st.Address, st.Contact, st.Phone, st.Status, opened, st.Remarks,
		})
	}

	var bigs []model.BigRegion
	var regions []model.Region
	_ = s.repo.DB.Order("sort_order, code").Find(&bigs).Error
	_ = s.repo.DB.Order("sort_order, code").Find(&regions).Error
	bigRows := make([]export.Cell, 0, len(bigs))
	for _, b := range bigs {
		bigRows = append(bigRows, export.Cell{b.Code, b.Name, b.Pinyin, b.Initials})
	}
	regionRows := make([]export.Cell, 0, len(regions))
	for _, r := range regions {
		regionRows = append(regionRows, export.Cell{r.Code, r.Name, r.Pinyin, r.Initials})
	}
	return export.WriteMultiSheet([]export.SheetSpec{
		{Name: "门店", Header: storeHeader, Rows: rows},
		{Name: "大区", Header: []string{"大区编码", "大区名称", "拼音", "首字母"}, Rows: bigRows},
		{Name: "区域", Header: []string{"区域编码", "区域名称", "拼音", "首字母"}, Rows: regionRows},
	})
}

// StoreImportResult summarizes one import run.
type StoreImportResult struct {
	Total      int        `json:"total"`
	Created    int        `json:"created"`
	Updated    int        `json:"updated"`
	Failed     int        `json:"failed"`
	Issues     []RowIssue `json:"issues"`
	ReportXLSX []byte     `json:"-"`
}

// ImportStores upserts stores by 门店编码, resolving regions by code.
func (s *TerritoryService) ImportStores(data []byte) (*StoreImportResult, error) {
	rows, err := export.ParseWorkbook(data)
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, errors.New("文件中没有数据行")
	}
	header := rows[0]
	iCode := colIndex(header, "门店编码", "code")
	iName := colIndex(header, "门店名称", "名称", "name")
	iBig := colIndex(header, "大区编码", "bigRegionCode")
	iRegion := colIndex(header, "区域编码", "regionCode")
	iAddr := colIndex(header, "地址", "address")
	iContact := colIndex(header, "联系人", "contact")
	iPhone := colIndex(header, "电话", "phone")
	iStatus := colIndex(header, "状态(active/closed)", "状态", "status")
	iOpened := colIndex(header, "开业日期(YYYY-MM-DD)", "开业日期", "openedAt")
	iRemarks := colIndex(header, "备注", "remarks")
	if iCode < 0 || iRegion < 0 {
		return nil, errors.New("缺少「门店编码」或「区域编码」列")
	}

	regionByCode := map[string]model.Region{}
	bigByID := map[uint]model.BigRegion{}
	var regions []model.Region
	var bigs []model.BigRegion
	_ = s.repo.DB.Find(&regions).Error
	_ = s.repo.DB.Find(&bigs).Error
	for _, b := range bigs {
		bigByID[b.ID] = b
	}
	for _, r := range regions {
		regionByCode[r.Code] = r
	}

	res := &StoreImportResult{}
	for r := 1; r < len(rows); r++ {
		row := rows[r]
		code := cell(row, iCode)
		if code == "" {
			continue
		}
		res.Total++
		rowNo := r + 1
		regionCode := cell(row, iRegion)
		region, ok := regionByCode[regionCode]
		if !ok {
			res.Failed++
			res.Issues = append(res.Issues, RowIssue{
				Row: rowNo, Key: code, Level: "error",
				Reason: "区域编码不存在: " + regionCode,
			})
			continue
		}
		big := bigByID[region.BigRegionID]
		if want := cell(row, iBig); want != "" && want != big.Code {
			res.Issues = append(res.Issues, RowIssue{
				Row: rowNo, Key: code, Level: "warning",
				Reason: fmt.Sprintf("大区编码 %s 与区域归属(%s)不一致，已按区域归属修正", want, big.Code),
			})
		}

		status := cell(row, iStatus)
		if status == "" {
			status = "active"
		}
		if status != "active" && status != "closed" {
			res.Issues = append(res.Issues, RowIssue{
				Row: rowNo, Key: code, Level: "warning",
				Reason: "未知状态 " + status + "，已按 active 处理",
			})
			status = "active"
		}

		var existing model.Store
		err := s.repo.DB.Where("code = ?", code).First(&existing).Error
		opened := cell(row, iOpened)
		var openedAt *time.Time
		if opened != "" {
			if t, perr := parseDateOnly(opened); perr == nil {
				openedAt = &t
			} else {
				res.Issues = append(res.Issues, RowIssue{
					Row: rowNo, Key: code, Level: "warning",
					Reason: "开业日期格式无效，已忽略: " + opened,
				})
			}
		}

		values := map[string]any{
			"name": firstNonEmpty(cell(row, iName), existing.Name),
			"big_region_code": big.Code, "big_region_id": big.ID,
			"region_code": region.Code, "region_id": region.ID,
			"address": cell(row, iAddr), "contact": cell(row, iContact),
			"phone": cell(row, iPhone), "status": status, "remarks": cell(row, iRemarks),
		}
		if openedAt != nil {
			values["opened_at"] = openedAt
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if values["name"] == "" {
				res.Failed++
				res.Issues = append(res.Issues, RowIssue{Row: rowNo, Key: code, Level: "error", Reason: "门店名称不能为空"})
				continue
			}
			st := model.Store{
				Code: code, Name: values["name"].(string),
				BigRegionCode: big.Code, BigRegionID: big.ID,
				RegionCode: region.Code, RegionID: region.ID,
				Address: cell(row, iAddr), Contact: cell(row, iContact), Phone: cell(row, iPhone),
				Status: status, OpenedAt: openedAt, Remarks: cell(row, iRemarks),
			}
			if err := s.repo.DB.Create(&st).Error; err != nil {
				res.Failed++
				res.Issues = append(res.Issues, RowIssue{Row: rowNo, Key: code, Level: "error", Reason: err.Error()})
				continue
			}
			res.Created++
			continue
		}
		if err != nil {
			res.Failed++
			res.Issues = append(res.Issues, RowIssue{Row: rowNo, Key: code, Level: "error", Reason: err.Error()})
			continue
		}
		if err := s.repo.DB.Model(&model.Store{}).Where("id = ?", existing.ID).Updates(values).Error; err != nil {
			res.Failed++
			res.Issues = append(res.Issues, RowIssue{Row: rowNo, Key: code, Level: "error", Reason: err.Error()})
			continue
		}
		res.Updated++
	}

	report, _ := export.WriteMultiSheet([]export.SheetSpec{
		{
			Name:   "导入结果",
			Header: []string{"总行数", "新增", "更新", "失败", "提示"},
			Rows:   []export.Cell{{res.Total, res.Created, res.Updated, res.Failed, formatIssueSummary(res.Issues)}},
		},
		{
			Name:   "问题明细",
			Header: []string{"行号", "门店编码", "级别", "原因"},
			Rows:   issuesToCells(res.Issues),
		},
	})
	res.ReportXLSX = report
	return res, nil
}
