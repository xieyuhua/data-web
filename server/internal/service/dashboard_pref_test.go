package service

import (
	"fmt"
	"testing"
	"time"

	"bi-platform/internal/model"
	"bi-platform/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestDashboardPreferenceLifecycle 覆盖收藏、置顶、浏览计数与看板删除后的清理。
func TestDashboardPreferenceLifecycle(t *testing.T) {
	dsn := fmt.Sprintf("file:preftest_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&model.Dashboard{}, &model.DashboardPreference{}); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}

	dashboardRepo := repository.NewDashboardRepository(db)
	prefRepo := repository.NewDashboardPreferenceRepository(db)
	svc := NewDashboardPreferenceService(prefRepo, dashboardRepo)

	dashboard := &model.Dashboard{Name: "测试看板", Code: "test_dashboard", Status: "published"}
	if err := dashboardRepo.Create(dashboard); err != nil {
		t.Fatalf("创建看板失败: %v", err)
	}
	const userID = uint64(7)

	// 首次收藏：记录自动创建
	yes := true
	if _, err := svc.Set(userID, dashboard.ID, &yes, nil); err != nil {
		t.Fatalf("设置收藏失败: %v", err)
	}
	prefs, err := svc.Map(userID)
	if err != nil {
		t.Fatalf("读取偏好失败: %v", err)
	}
	if !prefs[dashboard.ID].Favorite {
		t.Fatalf("收藏状态应为 true")
	}

	// 两次浏览：计数累加且刷新最近浏览时间
	for i := 0; i < 2; i++ {
		if err := svc.Touch(userID, dashboard.ID); err != nil {
			t.Fatalf("记录浏览失败: %v", err)
		}
	}
	prefs, _ = svc.Map(userID)
	if prefs[dashboard.ID].ViewCount != 2 {
		t.Fatalf("浏览次数 = %d，期望 2", prefs[dashboard.ID].ViewCount)
	}
	if prefs[dashboard.ID].LastViewedAt == nil {
		t.Fatalf("最近浏览时间不应为空")
	}

	// 置顶且取消收藏：未传的字段保持原值
	no := false
	if _, err := svc.Set(userID, dashboard.ID, &no, &yes); err != nil {
		t.Fatalf("设置置顶失败: %v", err)
	}
	prefs, _ = svc.Map(userID)
	if prefs[dashboard.ID].Favorite || !prefs[dashboard.ID].Pinned {
		t.Fatalf("收藏/置顶状态不符合预期: %+v", prefs[dashboard.ID])
	}

	// 看板删除后偏好应被清理
	if err := svc.DeleteByDashboard(dashboard.ID); err != nil {
		t.Fatalf("清理偏好失败: %v", err)
	}
	prefs, _ = svc.Map(userID)
	if _, ok := prefs[dashboard.ID]; ok {
		t.Fatalf("看板删除后不应残留偏好记录")
	}
}
