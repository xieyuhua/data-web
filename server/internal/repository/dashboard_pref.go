package repository

import (
	"errors"
	"time"

	"bi-platform/internal/model"

	"gorm.io/gorm"
)

// DashboardPreferenceRepository 看板个人偏好数据访问层。
// 偏好属于"个人视图设置"，与看板本身解耦：删除看板时才级联清理。
type DashboardPreferenceRepository struct{ db *gorm.DB }

// NewDashboardPreferenceRepository 创建看板偏好仓储实例。
func NewDashboardPreferenceRepository(db *gorm.DB) *DashboardPreferenceRepository {
	return &DashboardPreferenceRepository{db: db}
}

// ListByUser 查询指定用户的全部看板偏好（数量与可见看板同量级，无需分页）。
func (r *DashboardPreferenceRepository) ListByUser(userID uint64) ([]model.DashboardPreference, error) {
	var items []model.DashboardPreference
	err := r.db.Where("user_id = ?", userID).Find(&items).Error
	return items, err
}

// Find 查询某用户对某个看板的偏好记录。
func (r *DashboardPreferenceRepository) Find(userID, dashboardID uint64) (*model.DashboardPreference, error) {
	var item model.DashboardPreference
	err := r.db.Where("user_id = ? AND dashboard_id = ?", userID, dashboardID).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// SetFlags 更新收藏 / 置顶标记；不存在时先创建记录。
// favorite / pinned 为 nil 表示本次不改动该字段。
func (r *DashboardPreferenceRepository) SetFlags(userID, dashboardID uint64, favorite, pinned *bool) (*model.DashboardPreference, error) {
	item, err := r.ensure(userID, dashboardID)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if favorite != nil {
		updates["favorite"] = *favorite
	}
	if pinned != nil {
		updates["pinned"] = *pinned
	}
	if len(updates) == 0 {
		return item, nil
	}
	if err := r.db.Model(&model.DashboardPreference{}).Where("id = ?", item.ID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return r.Find(userID, dashboardID)
}

// Touch 记录一次打开：打开次数 +1，并刷新最近浏览时间。
// 使用一条 UPDATE 语句，避免高并发下读改写丢更新。
func (r *DashboardPreferenceRepository) Touch(userID, dashboardID uint64) error {
	now := time.Now()
	result := r.db.Model(&model.DashboardPreference{}).
		Where("user_id = ? AND dashboard_id = ?", userID, dashboardID).
		Updates(map[string]any{
			"view_count":     gorm.Expr("view_count + 1"),
			"last_viewed_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	// 首次浏览：插入一条带计数与时间的记录
	return r.db.Create(&model.DashboardPreference{
		UserID:       userID,
		DashboardID:  dashboardID,
		ViewCount:    1,
		LastViewedAt: &now,
	}).Error
}

// DeleteByDashboard 删除某个看板在所有用户下的偏好记录（看板删除时调用）。
func (r *DashboardPreferenceRepository) DeleteByDashboard(dashboardID uint64) error {
	return r.db.Where("dashboard_id = ?", dashboardID).Delete(&model.DashboardPreference{}).Error
}

// ensure 读取偏好记录，不存在时创建一条空记录。
func (r *DashboardPreferenceRepository) ensure(userID, dashboardID uint64) (*model.DashboardPreference, error) {
	item, err := r.Find(userID, dashboardID)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	item = &model.DashboardPreference{UserID: userID, DashboardID: dashboardID}
	if err := r.db.Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}
