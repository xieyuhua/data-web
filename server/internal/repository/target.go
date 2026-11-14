package repository

import (
	"bi-platform/internal/model"

	"gorm.io/gorm"
)

// TargetRepository 目标任务数据访问层。
// 任务按「组织 + 周期 + 指标 + 任务类型（基础/挑战）」唯一确定一条记录。
type TargetRepository struct{ db *gorm.DB }

// NewTargetRepository 创建目标任务仓储实例。
func NewTargetRepository(db *gorm.DB) *TargetRepository { return &TargetRepository{db: db} }

// List 按周期、指标、任务类型和组织范围查询目标任务。
// orgIDs 为 nil 表示不限制组织；为空切片表示无可见组织，直接返回空结果。
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
			// 调用方已明确「无可见组织」，无需再查库
			return []model.Target{}, nil
		}
		query = query.Where("org_id IN ?", orgIDs)
	}
	var items []model.Target
	err := query.Order("period desc, org_id asc, id asc").Find(&items).Error
	return items, err
}

// FindByID 按主键查询目标任务。
func (r *TargetRepository) FindByID(id uint64) (*model.Target, error) {
	var item model.Target
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// FindUnique 按业务唯一键查询目标任务，用于新增/更新时的重复校验。
func (r *TargetRepository) FindUnique(orgID uint64, periodType, period, metric, taskType string) (*model.Target, error) {
	var item model.Target
	err := r.db.Where("org_id = ? AND period_type = ? AND period = ? AND metric = ? AND task_type = ?", orgID, periodType, period, metric, taskType).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// Create 新增目标任务。
func (r *TargetRepository) Create(item *model.Target) error { return r.db.Create(item).Error }

// Update 保存目标任务（全字段更新）。
func (r *TargetRepository) Update(item *model.Target) error { return r.db.Save(item).Error }

// Delete 按主键删除目标任务。
func (r *TargetRepository) Delete(id uint64) error {
	return r.db.Delete(&model.Target{}, id).Error
}

// DistinctPeriods 查询某周期类型下已存在的周期列表（如 2026-11），用于前端下拉。
func (r *TargetRepository) DistinctPeriods(periodType string) ([]string, error) {
	var periods []string
	query := r.db.Model(&model.Target{}).Distinct("period")
	if periodType != "" {
		query = query.Where("period_type = ?", periodType)
	}
	err := query.Order("period desc").Pluck("period", &periods).Error
	return periods, err
}

// DB 暴露底层连接，供需要组合查询的调用方使用。
func (r *TargetRepository) DB() *gorm.DB { return r.db }
