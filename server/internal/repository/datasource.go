package repository

import (
	"bi-platform/internal/model"

	"gorm.io/gorm"
)

type DatasourceRepository struct{ db *gorm.DB }

func NewDatasourceRepository(db *gorm.DB) *DatasourceRepository {
	return &DatasourceRepository{db: db}
}

func (r *DatasourceRepository) List() ([]model.Datasource, error) {
	var items []model.Datasource
	err := r.db.Order("id asc").Find(&items).Error
	return items, err
}

func (r *DatasourceRepository) FindByID(id uint64) (*model.Datasource, error) {
	var item model.Datasource
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *DatasourceRepository) FindByCode(code string) (*model.Datasource, error) {
	var item model.Datasource
	if err := r.db.Where("code = ?", code).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *DatasourceRepository) Create(item *model.Datasource) error { return r.db.Create(item).Error }
func (r *DatasourceRepository) Update(item *model.Datasource) error { return r.db.Save(item).Error }
func (r *DatasourceRepository) Delete(id uint64) error {
	return r.db.Delete(&model.Datasource{}, id).Error
}
func (r *DatasourceRepository) DB() *gorm.DB { return r.db }
