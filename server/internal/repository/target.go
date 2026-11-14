package repository

import (
	"bi-platform/internal/model"

	"gorm.io/gorm"
)

type TargetRepository struct{ db *gorm.DB }

func NewTargetRepository(db *gorm.DB) *TargetRepository { return &TargetRepository{db: db} }

func (r *TargetRepository) List(periodType, period, metric, taskType string, orgIDs []uint64) ([]model.Target, error) {
	query := r.db.Model(&model.Target{})
	if periodType != "" {
		query = query.Where("period_type = ?", periodType)
	}
	if period != "" {
		query = query.Where("period = ?", period)
	}
	if metric != "" {
		query = query.Where("metric = ?", metric)
	}
	if taskType != "" {
		query = query.Where("task_type = ?", taskType)
	}
	if orgIDs != nil {
		if len(orgIDs) == 0 {
			return []model.Target{}, nil
		}
		query = query.Where("org_id IN ?", orgIDs)
	}
	var items []model.Target
	err := query.Order("period desc, org_id asc, id asc").Find(&items).Error
	return items, err
}

func (r *TargetRepository) FindByID(id uint64) (*model.Target, error) {
	var item model.Target
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *TargetRepository) FindUnique(orgID uint64, periodType, period, metric, taskType string) (*model.Target, error) {
	var item model.Target
	err := r.db.Where("org_id = ? AND period_type = ? AND period = ? AND metric = ? AND task_type = ?", orgID, periodType, period, metric, taskType).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *TargetRepository) Create(item *model.Target) error { return r.db.Create(item).Error }
func (r *TargetRepository) Update(item *model.Target) error { return r.db.Save(item).Error }
func (r *TargetRepository) Delete(id uint64) error {
	return r.db.Delete(&model.Target{}, id).Error
}

func (r *TargetRepository) DistinctPeriods(periodType string) ([]string, error) {
	var periods []string
	query := r.db.Model(&model.Target{}).Distinct("period")
	if periodType != "" {
		query = query.Where("period_type = ?", periodType)
	}
	err := query.Order("period desc").Pluck("period", &periods).Error
	return periods, err
}

func (r *TargetRepository) DB() *gorm.DB { return r.db }
