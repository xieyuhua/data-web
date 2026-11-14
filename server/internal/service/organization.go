package service

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"bi-platform/internal/model"
	"bi-platform/internal/repository"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// OrganizationService 组织架构业务服务。
type OrganizationService struct {
	repo *repository.OrganizationRepository
}

// NewOrganizationService 创建组织服务。
func NewOrganizationService(repo *repository.OrganizationRepository) *OrganizationService {
	return &OrganizationService{repo: repo}
}

// Tree 查询组织树。
func (s *OrganizationService) Tree() ([]model.Organization, error) { return s.repo.Tree() }

// All 查询全部组织（平铺）。
func (s *OrganizationService) All() ([]model.Organization, error) { return s.repo.All() }

// Create 新增组织（校验层级与编码）。
func (s *OrganizationService) Create(item *model.Organization) error {
	return s.validateAndSave(item, false)
}

// Update 更新组织。
func (s *OrganizationService) Update(item *model.Organization) error {
	if item.ID == 0 {
		return errors.New("组织 ID 不能为空")
	}
	return s.validateAndSave(item, true)
}

func (s *OrganizationService) validateAndSave(item *model.Organization, update bool) error {
	item.Name = strings.TrimSpace(item.Name)
	item.Code = strings.TrimSpace(item.Code)
	if item.Name == "" || item.Code == "" {
		return errors.New("组织名称和编码不能为空")
	}
	validTypes := map[string]bool{"company": true, "big_region": true, "region": true, "store": true}
	if !validTypes[item.Type] {
		return errors.New("组织类型不正确")
	}
	item.Level = 1
	if item.ParentID != nil && *item.ParentID > 0 {
		parent, err := s.repo.FindByID(*item.ParentID)
		if err != nil {
			return errors.New("上级组织不存在")
		}
		item.Level = parent.Level + 1
	}
	if item.Status == "" {
		item.Status = "active"
	}
	if update {
		existing, err := s.repo.FindByCode(item.Code)
		if err != nil {
			return err
		}
		if existing.ID != item.ID {
			return errors.New("组织编码已存在")
		}
		if item.ParentID != nil && *item.ParentID == item.ID {
			return errors.New("上级组织不能是自己")
		}
		return s.repo.Update(item)
	}
	return s.repo.Create(item)
}

// Delete 删除组织（存在下级或用户时拒绝）。
func (s *OrganizationService) Delete(id uint64) error { return s.repo.Delete(id) }

// Export 导出组织架构为 Excel。
func (s *OrganizationService) Export() ([]byte, error) {
	items, err := s.repo.All()
	if err != nil {
		return nil, err
	}
	byID := make(map[uint64]model.Organization)
	for _, item := range items {
		byID[item.ID] = item
	}
	file := excelize.NewFile()
	defer file.Close()
	sheet := "组织架构"
	index, _ := file.NewSheet(sheet)
	file.SetActiveSheet(index)
	// 移除 excelize 默认创建的空 Sheet1，保证导出文件可直接再次导入。
	_ = file.DeleteSheet("Sheet1")
	headers := []string{"类型", "名称", "编码", "上级编码", "负责人", "电话", "地址", "状态"}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = file.SetCellValue(sheet, cell, header)
	}
	style, _ := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"173A33"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	_ = file.SetCellStyle(sheet, "A1", "H1", style)
	for rowIndex, item := range items {
		row := rowIndex + 2
		parentCode := ""
		if item.ParentID != nil {
			parentCode = byID[*item.ParentID].Code
		}
		values := []any{item.Type, item.Name, item.Code, parentCode, item.ManagerName, item.Phone, item.Address, item.Status}
		for col, value := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			_ = file.SetCellValue(sheet, cell, value)
		}
	}
	_ = file.SetColWidth(sheet, "A", "H", 16)
	_ = file.SetColWidth(sheet, "C", "E", 22)
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// ImportOrganization 通讯录导入时的一行组织数据。
type ImportOrganization struct {
	Type       string
	Name       string
	Code       string
	ParentCode string
	Manager    string
	Phone      string
	Address    string
	Status     string
}

// Import 导入组织架构 Excel，按编码新增或更新。
func (s *OrganizationService) Import(filename string, reader io.Reader) (int, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".xlsx" && ext != ".xls" {
		return 0, errors.New("请上传 Excel 文件（.xlsx）")
	}
	file, err := excelize.OpenReader(reader)
	if err != nil {
		return 0, fmt.Errorf("无法读取 Excel: %w", err)
	}
	defer file.Close()
	sheets := file.GetSheetList()
	if len(sheets) == 0 {
		return 0, errors.New("Excel 中没有工作表")
	}
	var rows [][]string
	for _, sheet := range sheets {
		candidate, readErr := file.GetRows(sheet)
		if readErr != nil || len(candidate) < 2 {
			continue
		}
		hasName, hasCode := false, false
		for _, title := range candidate[0] {
			switch strings.TrimSpace(title) {
			case "名称":
				hasName = true
			case "编码":
				hasCode = true
			}
		}
		if hasName && hasCode {
			rows = candidate
			break
		}
	}
	if len(rows) < 2 {
		return 0, errors.New("Excel 中没有可导入的数据，请确认表头包含“名称”和“编码”")
	}
	headers := make(map[string]int)
	for i, title := range rows[0] {
		headers[strings.TrimSpace(title)] = i
	}
	get := func(row []string, name string) string {
		index, ok := headers[name]
		if !ok || index >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[index])
	}
	typeAliases := map[string]string{
		"公司": "company", "集团": "company", "company": "company",
		"大区": "big_region", "大区经理": "big_region", "big_region": "big_region", "big region": "big_region",
		"区域": "region", "region": "region",
		"门店": "store", "store": "store",
	}
	records := make([]ImportOrganization, 0)
	for _, row := range rows[1:] {
		name := get(row, "名称")
		code := get(row, "编码")
		if name == "" && code == "" {
			continue
		}
		orgType := typeAliases[strings.ToLower(get(row, "类型"))]
		if orgType == "" {
			orgType = get(row, "类型")
		}
		records = append(records, ImportOrganization{
			Type:       orgType,
			Name:       name,
			Code:       code,
			ParentCode: get(row, "上级编码"),
			Manager:    get(row, "负责人"),
			Phone:      get(row, "电话"),
			Address:    get(row, "地址"),
			Status:     get(row, "状态"),
		})
	}
	if len(records) == 0 {
		return 0, errors.New("没有解析到组织数据")
	}

	pending := records
	imported := 0
	for len(pending) > 0 {
		progress := false
		next := make([]ImportOrganization, 0)
		for _, record := range pending {
			if record.Name == "" || record.Code == "" || record.Type == "" {
				return imported, fmt.Errorf("组织 %s 缺少名称、编码或类型", record.Code)
			}
			var parentID *uint64
			if record.ParentCode != "" {
				parent, err := s.repo.FindByCode(record.ParentCode)
				if err != nil {
					next = append(next, record)
					continue
				}
				parentID = &parent.ID
			}
			status := record.Status
			if status == "" {
				status = "active"
			}
			item := &model.Organization{
				Name: record.Name, Code: record.Code, Type: record.Type, ParentID: parentID,
				ManagerName: record.Manager, Phone: record.Phone, Address: record.Address, Status: status,
			}
			existing, findErr := s.repo.FindByCode(record.Code)
			if findErr == nil {
				// Import is an upsert: update the existing organization by code.
				item.ID = existing.ID
				item.CreatedAt = existing.CreatedAt
				if err := s.validateAndSave(item, true); err != nil {
					return imported, err
				}
			} else if errors.Is(findErr, gorm.ErrRecordNotFound) {
				if err := s.validateAndSave(item, false); err != nil {
					return imported, err
				}
			} else {
				return imported, findErr
			}
			imported++
			progress = true
		}
		if !progress {
			missing := make([]string, 0)
			for _, item := range next {
				missing = append(missing, item.Code+"→"+item.ParentCode)
			}
			return imported, fmt.Errorf("无法确定上级组织: %s", strings.Join(missing, ", "))
		}
		pending = next
	}
	return imported, nil
}
