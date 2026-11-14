package service

import (
	"errors"
	"time"

	"bi-platform/internal/repository"
)

// DashboardPreferenceView 看板偏好的对外结构。
type DashboardPreferenceView struct {
	DashboardID  uint64     `json:"dashboard_id"`
	Favorite     bool       `json:"favorite"`
	Pinned       bool       `json:"pinned"`
	ViewCount    int        `json:"view_count"`
	LastViewedAt *time.Time `json:"last_viewed_at"`
}

// DashboardPreferenceService 处理"我的收藏 / 置顶 / 最近浏览"相关业务。
// 偏好只影响个人使用体验，不参与数据权限判断。
type DashboardPreferenceService struct {
	repo          *repository.DashboardPreferenceRepository
	dashboardRepo *repository.DashboardRepository
}

// NewDashboardPreferenceService 创建看板偏好服务。
func NewDashboardPreferenceService(repo *repository.DashboardPreferenceRepository, dashboardRepo *repository.DashboardRepository) *DashboardPreferenceService {
	return &DashboardPreferenceService{repo: repo, dashboardRepo: dashboardRepo}
}

// Map 返回当前用户的全部偏好，按看板 ID 建索引，供列表页一次性合并。
func (s *DashboardPreferenceService) Map(userID uint64) (map[uint64]DashboardPreferenceView, error) {
	items, err := s.repo.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	result := make(map[uint64]DashboardPreferenceView, len(items))
	for _, item := range items {
		result[item.DashboardID] = DashboardPreferenceView{
			DashboardID:  item.DashboardID,
			Favorite:     item.Favorite,
			Pinned:       item.Pinned,
			ViewCount:    item.ViewCount,
			LastViewedAt: item.LastViewedAt,
		}
	}
	return result, nil
}

// Set 更新收藏 / 置顶状态；两个参数都为 nil 时只做存在性校验。
func (s *DashboardPreferenceService) Set(userID, dashboardID uint64, favorite, pinned *bool) (*DashboardPreferenceView, error) {
	if dashboardID == 0 {
		return nil, errors.New("看板参数不正确")
	}
	// 校验看板存在，避免为已删除看板写入孤立偏好
	if _, err := s.dashboardRepo.FindByID(dashboardID); err != nil {
		return nil, errors.New("看板不存在")
	}
	item, err := s.repo.SetFlags(userID, dashboardID, favorite, pinned)
	if err != nil {
		return nil, err
	}
	return &DashboardPreferenceView{
		DashboardID:  item.DashboardID,
		Favorite:     item.Favorite,
		Pinned:       item.Pinned,
		ViewCount:    item.ViewCount,
		LastViewedAt: item.LastViewedAt,
	}, nil
}

// Touch 记录一次浏览，用于"最近浏览"排序。失败不影响看板正常展示。
func (s *DashboardPreferenceService) Touch(userID, dashboardID uint64) error {
	if dashboardID == 0 {
		return errors.New("看板参数不正确")
	}
	return s.repo.Touch(userID, dashboardID)
}

// DeleteByDashboard 看板删除后清理所有用户的偏好记录。
func (s *DashboardPreferenceService) DeleteByDashboard(dashboardID uint64) error {
	return s.repo.DeleteByDashboard(dashboardID)
}
