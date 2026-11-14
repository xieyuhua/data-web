package repository

import (
	"strings"

	"bi-platform/internal/model"

	"gorm.io/gorm"
)

// DatasetRepository 数据集数据访问层。
// 数据集保存 SQL、参数与可视化配置，是看板组件的数据来源。
type DatasetRepository struct{ db *gorm.DB }

// NewDatasetRepository 创建数据集仓储实例。
func NewDatasetRepository(db *gorm.DB) *DatasetRepository { return &DatasetRepository{db: db} }

// List 查询全部数据集。
func (r *DatasetRepository) List() ([]model.Dataset, error) {
	var items []model.Dataset
	err := r.db.Order("id asc").Find(&items).Error
	return items, err
}

// FindByID 按主键查询数据集。
func (r *DatasetRepository) FindByID(id uint64) (*model.Dataset, error) {
	var item model.Dataset
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// FindByCode 按编码查询数据集（编码唯一）。
func (r *DatasetRepository) FindByCode(code string) (*model.Dataset, error) {
	var item model.Dataset
	if err := r.db.Where("code = ?", code).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// Create 新增数据集。
func (r *DatasetRepository) Create(item *model.Dataset) error { return r.db.Create(item).Error }

// Update 保存数据集。
func (r *DatasetRepository) Update(item *model.Dataset) error { return r.db.Save(item).Error }

// Delete 按主键删除数据集。
func (r *DatasetRepository) Delete(id uint64) error {
	return r.db.Delete(&model.Dataset{}, id).Error
}

// DB 暴露底层连接。
func (r *DatasetRepository) DB() *gorm.DB { return r.db }

// DashboardRepository 看板数据访问层。
type DashboardRepository struct{ db *gorm.DB }

// NewDashboardRepository 创建看板仓储实例。
func NewDashboardRepository(db *gorm.DB) *DashboardRepository {
	return &DashboardRepository{db: db}
}

// List 按关键字（名称/描述）与分类查询看板，最近更新的排在前面。
func (r *DashboardRepository) List(keyword, category string) ([]model.Dashboard, error) {
	query := r.db.Model(&model.Dashboard{})
	if keyword != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	if category != "" {
		query = query.Where("category = ?", category)
	}
	var items []model.Dashboard
	err := query.Order("updated_at desc").Find(&items).Error
	return items, err
}

// FindByID 按主键查询看板。
func (r *DashboardRepository) FindByID(id uint64) (*model.Dashboard, error) {
	var item model.Dashboard
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// FindByCode 按编码查询看板（分享链接、前端路由使用）。
func (r *DashboardRepository) FindByCode(code string) (*model.Dashboard, error) {
	var item model.Dashboard
	if err := r.db.Where("code = ?", code).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// Create 新增看板。
func (r *DashboardRepository) Create(item *model.Dashboard) error { return r.db.Create(item).Error }

// Update 保存看板。
func (r *DashboardRepository) Update(item *model.Dashboard) error { return r.db.Save(item).Error }

// Delete 按主键删除看板。
func (r *DashboardRepository) Delete(id uint64) error {
	return r.db.Delete(&model.Dashboard{}, id).Error
}

// DB 暴露底层连接。
func (r *DashboardRepository) DB() *gorm.DB { return r.db }

// TemplateRepository 看板模板数据访问层。
type TemplateRepository struct{ db *gorm.DB }

// NewTemplateRepository 创建模板仓储实例。
func NewTemplateRepository(db *gorm.DB) *TemplateRepository {
	return &TemplateRepository{db: db}
}

// List 查询模板列表，内置模板排在最前。
func (r *TemplateRepository) List() ([]model.Template, error) {
	var items []model.Template
	err := r.db.Order("built_in desc, id asc").Find(&items).Error
	return items, err
}

// FindByID 按主键查询模板。
func (r *TemplateRepository) FindByID(id uint64) (*model.Template, error) {
	var item model.Template
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// Create 新增模板。
func (r *TemplateRepository) Create(item *model.Template) error { return r.db.Create(item).Error }

// Update 保存模板。
func (r *TemplateRepository) Update(item *model.Template) error { return r.db.Save(item).Error }

// Delete 按主键删除模板。
func (r *TemplateRepository) Delete(id uint64) error {
	return r.db.Delete(&model.Template{}, id).Error
}
