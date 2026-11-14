package service

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"bi-platform/internal/config"
	"bi-platform/internal/database"
	"bi-platform/internal/model"
	"bi-platform/internal/repository"

	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	db, err := database.Open(config.Config{
		DBDriver:    "sqlite",
		DSN:         dsn,
		Timezone:    "Asia/Shanghai",
		AutoMigrate: true,
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedDrillFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	now := time.Now()

	roles := []model.Role{
		{Code: "super_admin", Name: "平台管理员", DataScope: "all", Permissions: `["*"]`},
		{Code: "big_region_manager", Name: "大区经理", DataScope: "own_org_tree", Permissions: `["dashboard:view"]`},
		{Code: "store_manager", Name: "店长", DataScope: "own_org", Permissions: `["dashboard:view"]`},
	}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatalf("seed roles: %v", err)
	}

	hq := uint64(1)
	br := uint64(2)
	rg1 := uint64(3)
	rg2 := uint64(6)
	orgs := []model.Organization{
		{ID: 1, ParentID: nil, Name: "集团", Code: "HQ", Type: "company", Level: 1, Status: "active"},
		{ID: 2, ParentID: &hq, Name: "华东大区", Code: "BR_EAST", Type: "big_region", Level: 2, Status: "active"},
		{ID: 3, ParentID: &br, Name: "浙江区域", Code: "RG_ZJ", Type: "region", Level: 3, Status: "active"},
		{ID: 4, ParentID: &rg1, Name: "杭州店", Code: "ST_HZ", Type: "store", Level: 4, Status: "active"},
		{ID: 5, ParentID: &rg1, Name: "宁波店", Code: "ST_NB", Type: "store", Level: 4, Status: "active"},
		{ID: 6, ParentID: &br, Name: "江苏区域", Code: "RG_JS", Type: "region", Level: 3, Status: "active"},
		{ID: 7, ParentID: &rg2, Name: "南京店", Code: "ST_NJ", Type: "store", Level: 4, Status: "active"},
	}
	if err := db.Create(&orgs).Error; err != nil {
		t.Fatalf("seed orgs: %v", err)
	}

	facts := []model.SalesFact{
		{OrgID: 4, StoreID: 4, StatDate: "2026-11-05", Revenue: 500, Orders: 5, GrossProfit: 100, Customers: 10},
		{OrgID: 5, StoreID: 5, StatDate: "2026-11-05", Revenue: 1500, Orders: 15, GrossProfit: 300, Customers: 30},
		{OrgID: 7, StoreID: 7, StatDate: "2026-11-05", Revenue: 800, Orders: 8, GrossProfit: 160, Customers: 16},
	}
	if err := db.Create(&facts).Error; err != nil {
		t.Fatalf("seed sales facts: %v", err)
	}

	employees := []model.Employee{
		{ID: 11, OrgID: 4, Code: "E11", Name: "张三", Position: "销售", Status: "active"},
		{ID: 12, OrgID: 4, Code: "E12", Name: "李四", Position: "导购", Status: "active"},
		{ID: 13, OrgID: 5, Code: "E13", Name: "王五", Position: "销售", Status: "active"},
		{ID: 14, OrgID: 7, Code: "E14", Name: "赵六", Position: "销售", Status: "active"},
	}
	if err := db.Create(&employees).Error; err != nil {
		t.Fatalf("seed employees: %v", err)
	}

	employeeSales := []model.EmployeeSale{
		{EmployeeID: 11, OrgID: 4, StatDate: "2026-11-05", Revenue: 300, Orders: 3, GrossProfit: 60, Customers: 6},
		{EmployeeID: 12, OrgID: 4, StatDate: "2026-11-05", Revenue: 200, Orders: 2, GrossProfit: 40, Customers: 4},
		{EmployeeID: 13, OrgID: 5, StatDate: "2026-11-05", Revenue: 1500, Orders: 15, GrossProfit: 300, Customers: 30},
		{EmployeeID: 14, OrgID: 7, StatDate: "2026-11-05", Revenue: 800, Orders: 8, GrossProfit: 160, Customers: 16},
	}
	if err := db.Create(&employeeSales).Error; err != nil {
		t.Fatalf("seed employee sales: %v", err)
	}

	targets := []model.Target{
		{Name: "杭州店月度销售目标", OrgID: 4, PeriodType: "month", Period: "2026-11", Metric: "revenue", TargetValue: 1000, Unit: "元", CreatedAt: now},
		{Name: "宁波店月度销售目标", OrgID: 5, PeriodType: "month", Period: "2026-11", Metric: "revenue", TargetValue: 2000, Unit: "元", CreatedAt: now},
		{Name: "南京店月度销售目标", OrgID: 7, PeriodType: "month", Period: "2026-11", Metric: "revenue", TargetValue: 1600, Unit: "元", CreatedAt: now},
	}
	if err := db.Create(&targets).Error; err != nil {
		t.Fatalf("seed targets: %v", err)
	}
}

func newDrillFixture(t *testing.T) (*gorm.DB, *DrillService, *TargetService) {
	t.Helper()
	db := newTestDB(t)
	seedDrillFixture(t, db)
	orgRepo := repository.NewOrganizationRepository(db)
	roleRepo := repository.NewRoleRepository(db)
	scope := NewScopeService(orgRepo, roleRepo)
	drill := NewDrillService(orgRepo, repository.NewTargetRepository(db), repository.NewDrillRepository(db), scope)
	target := NewTargetService(repository.NewTargetRepository(db), orgRepo)
	return db, drill, target
}

func mustQuery(t *testing.T, drill *DrillService, user *model.User, orgID uint64, mode string) *DrillResult {
	t.Helper()
	result, err := drill.Query(user, orgID, "month", "2026-11", "revenue", mode)
	if err != nil {
		t.Fatalf("drill query failed: %v", err)
	}
	return result
}

func findNode(t *testing.T, result *DrillResult, name string) DrillNode {
	t.Helper()
	for _, node := range result.Nodes {
		if node.Name == name {
			return node
		}
	}
	t.Fatalf("node %s not found in %d nodes", name, len(result.Nodes))
	return DrillNode{}
}

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 0.001 }

func TestDrillLevelAggregatesChildTargets(t *testing.T) {
	_, drill, _ := newDrillFixture(t)
	user := &model.User{ID: 1, Name: "大区经理", RoleCode: "big_region_manager", OrganizationID: 2}

	result := mustQuery(t, drill, user, 0, "level")
	if result.Current.Name != "华东大区" {
		t.Fatalf("默认定位应为本人组织，实际 %s", result.Current.Name)
	}
	if result.NextLevelType != "region" {
		t.Fatalf("下一层类型应为 region，实际 %s", result.NextLevelType)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("华东大区应有 2 个区域，实际 %d", len(result.Nodes))
	}

	zhejiang := findNode(t, result, "浙江区域")
	if !almostEqual(zhejiang.ActualValue, 2000) {
		t.Fatalf("浙江区域实际值应为 2000，实际 %v", zhejiang.ActualValue)
	}
	// 区域本身没有目标时，应向下汇总门店目标 1000 + 2000 = 3000
	if !almostEqual(zhejiang.TargetValue, 3000) {
		t.Fatalf("浙江区域目标应汇总为 3000，实际 %v", zhejiang.TargetValue)
	}
	if !zhejiang.HasTarget || zhejiang.Rate == nil || !almostEqual(*zhejiang.Rate, 66.7) {
		t.Fatalf("浙江区域达成率应为 66.7%%，实际 %v", zhejiang.Rate)
	}

	if !almostEqual(result.Summary.ActualValue, 2800) || !almostEqual(result.Summary.TargetValue, 4600) {
		t.Fatalf("大区汇总实际/目标应为 2800/4600，实际 %v/%v", result.Summary.ActualValue, result.Summary.TargetValue)
	}
	if !almostEqual(result.Summary.Rate, 60.9) {
		t.Fatalf("大区达成率应为 60.9%%，实际 %v", result.Summary.Rate)
	}
}

func TestDrillFlatAndEmployeeModes(t *testing.T) {
	_, drill, _ := newDrillFixture(t)
	user := &model.User{ID: 1, Name: "大区经理", RoleCode: "big_region_manager", OrganizationID: 2}

	flat := mustQuery(t, drill, user, 0, "all")
	if flat.NextLevelType != "store" || len(flat.Nodes) != 3 {
		t.Fatalf("全部门店模式应平铺 3 家门店，实际 %d（%s）", len(flat.Nodes), flat.NextLevelType)
	}

	employees := mustQuery(t, drill, user, 0, "employees")
	if employees.NextLevelType != "employee" || len(employees.Nodes) != 4 {
		t.Fatalf("全部员工模式应返回 4 名员工，实际 %d", len(employees.Nodes))
	}
	for _, node := range employees.Nodes {
		if node.Type != "employee" || node.ParentName == "" {
			t.Fatalf("员工节点应带所属门店，实际 %+v", node)
		}
	}
}

func TestDrillStoreShowsEmployeesAndEnforcesScope(t *testing.T) {
	_, drill, _ := newDrillFixture(t)
	storeUser := &model.User{ID: 2, Name: "店长", RoleCode: "store_manager", OrganizationID: 4}

	result := mustQuery(t, drill, storeUser, 0, "level")
	if result.Current.Type != "store" || result.NextLevelType != "employee" || len(result.Nodes) != 2 {
		t.Fatalf("店长进入应直接看到本店 2 名员工，实际 %d（%s）", len(result.Nodes), result.NextLevelType)
	}
	if !almostEqual(result.Summary.ActualValue, 500) {
		t.Fatalf("门店销售额应为 500，实际 %v", result.Summary.ActualValue)
	}

	if _, err := drill.Query(storeUser, 2, "month", "2026-11", "revenue", "level"); err == nil {
		t.Fatal("店长查询大区数据应被拒绝")
	}
}

func TestTargetAchievementSummaryAvoidsDoubleCounting(t *testing.T) {
	db, _, targetService := newDrillFixture(t)

	// 只有门店目标时，三家门店都是顶层目标
	result, err := targetService.Achievement(nil, "month", "2026-11", "revenue", "")
	if err != nil {
		t.Fatalf("achievement failed: %v", err)
	}
	if result.Summary.RootCount != 3 {
		t.Fatalf("应识别 3 个顶层目标，实际 %d", result.Summary.RootCount)
	}
	if !almostEqual(result.Summary.TargetValue, 4600) || !almostEqual(result.Summary.ActualValue, 2800) {
		t.Fatalf("汇总目标/实际应为 4600/2800，实际 %v/%v", result.Summary.TargetValue, result.Summary.ActualValue)
	}

	// 追加一条大区目标后，汇总口径切换为大区，不再累加下级目标
	if err := db.Create(&model.Target{
		Name: "华东大区月度销售目标", OrgID: 2, PeriodType: "month", Period: "2026-11",
		Metric: "revenue", TargetValue: 4000, Unit: "元", CreatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("create region target: %v", err)
	}
	result, err = targetService.Achievement(nil, "month", "2026-11", "revenue", "")
	if err != nil {
		t.Fatalf("achievement failed: %v", err)
	}
	if result.Summary.RootCount != 1 {
		t.Fatalf("应只保留 1 个顶层目标，实际 %d", result.Summary.RootCount)
	}
	if !almostEqual(result.Summary.TargetValue, 4000) || !almostEqual(result.Summary.ActualValue, 2800) {
		t.Fatalf("顶层口径目标/实际应为 4000/2800，实际 %v/%v", result.Summary.TargetValue, result.Summary.ActualValue)
	}
	if !almostEqual(result.Summary.Rate, 70) {
		t.Fatalf("达成率应为 70%%，实际 %v", result.Summary.Rate)
	}
	if len(result.Targets) != 4 {
		t.Fatalf("明细仍应保留 4 条目标，实际 %d", len(result.Targets))
	}
}
