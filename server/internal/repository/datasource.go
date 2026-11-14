package repository

import (
	"bi-platform/internal/model"

	"gorm.io/gorm"
)

// DatasourceRepository 数据源数据访问层。
// 数据源保存外部数据库连接信息，密码字段在服务层加密后再落库。
type DatasourceRepository struct{ db *gorm.DB }

// NewDatasourceRepository 创建数据源仓储实例。
func NewDatasourceRepository(db *gorm.DB) *DatasourceRepository {
	return &DatasourceRepository{db: db}
}

// List 查询全部数据源。
func (r *DatasourceRepository) List() ([]model.Datasource, error) {
	var items []model.Datasource
	err := r.db.Order("id asc").Find(&items).Error
	return items, err
}

// FindByID 按主键查询数据源。
func (r *DatasourceRepository) FindByID(id uint64) (*model.Datasource, error) {
	var item model.Datasource
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// FindByCode 按编码查询数据源（编码唯一）。
func (r *DatasourceRepository) FindByCode(code string) (*model.Datasource, error) {
	var item model.Datasource
	if err := r.db.Where("code = ?", code).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// Create 新增数据源。
func (r *DatasourceRepository) Create(item *model.Datasource) error { return r.db.Create(item).Error }

// Update 保存数据源。
func (r *DatasourceRepository) Update(item *model.Datasource) error { return r.db.Save(item).Error }

// Delete 按主键删除数据源。
func (r *DatasourceRepository) Delete(id uint64) error {
	return r.db.Delete(&model.Datasource{}, id).Error
}

// DB 暴露底层连接。
func (r *DatasourceRepository) DB() *gorm.DB { return r.db }
