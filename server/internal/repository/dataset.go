package repository

import (
	"strings"

	"bi-platform/internal/model"

	"gorm.io/gorm"
)

type DatasetRepository struct{ db *gorm.DB }

func NewDatasetRepository(db *gorm.DB) *DatasetRepository { return &DatasetRepository{db: db} }

func (r *DatasetRepository) List() ([]model.Dataset, error) {
	var items []model.Dataset
	err := r.db.Order("id asc").Find(&items).Error
	return items, err
}

func (r *DatasetRepository) FindByID(id uint64) (*model.Dataset, error) {
	var item model.Dataset
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *DatasetRepository) FindByCode(code string) (*model.Dataset, error) {
	var item model.Dataset
	if err := r.db.Where("code = ?", code).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *DatasetRepository) Create(item *model.Dataset) error { return r.db.Create(item).Error }
func (r *DatasetRepository) Update(item *model.Dataset) error { return r.db.Save(item).Error }
func (r *DatasetRepository) Delete(id uint64) error {
	return r.db.Delete(&model.Dataset{}, id).Error
}

func (r *DatasetRepository) DB() *gorm.DB { return r.db }

type DashboardRepository struct{ db *gorm.DB }

func NewDashboardRepository(db *gorm.DB) *DashboardRepository {
	return &DashboardRepository{db: db}
}

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

func (r *DashboardRepository) FindByID(id uint64) (*model.Dashboard, error) {
	var item model.Dashboard
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *DashboardRepository) FindByCode(code string) (*model.Dashboard, error) {
	var item model.Dashboard
	if err := r.db.Where("code = ?", code).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *DashboardRepository) Create(item *model.Dashboard) error { return r.db.Create(item).Error }
func (r *DashboardRepository) Update(item *model.Dashboard) error { return r.db.Save(item).Error }
func (r *DashboardRepository) Delete(id uint64) error {
	return r.db.Delete(&model.Dashboard{}, id).Error
}
func (r *DashboardRepository) DB() *gorm.DB { return r.db }

type TemplateRepository struct{ db *gorm.DB }

func NewTemplateRepository(db *gorm.DB) *TemplateRepository {
	return &TemplateRepository{db: db}
}

func (r *TemplateRepository) List() ([]model.Template, error) {
	var items []model.Template
	err := r.db.Order("built_in desc, id asc").Find(&items).Error
	return items, err
}

func (r *TemplateRepository) FindByID(id uint64) (*model.Template, error) {
	var item model.Template
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *TemplateRepository) Create(item *model.Template) error { return r.db.Create(item).Error }
func (r *TemplateRepository) Update(item *model.Template) error { return r.db.Save(item).Error }
func (r *TemplateRepository) Delete(id uint64) error {
	return r.db.Delete(&model.Template{}, id).Error
}
