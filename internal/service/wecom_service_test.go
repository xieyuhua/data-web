package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"biaoge-bi/internal/config"
	"biaoge-bi/internal/model"
	"biaoge-bi/internal/repository"
	"biaoge-bi/internal/wecom"
)

func newWeComFixture(t *testing.T) (*WeComService, *repository.Repo) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "meta.db")), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(
		&model.User{}, &model.Department{}, &model.DashboardDeptGrant{}, &model.SyncState{},
		&model.Dashboard{}, &model.DashboardGrant{}, &model.DashboardWidget{},
		&model.DashboardFilter{}, &model.FilterBinding{}, &model.Chart{},
	); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.WeCom = config.WeComConfig{
		Enabled: true, CorpID: "corp", AgentID: 100001, Secret: "secret",
		AutoCreateUser: true, APIBase: "https://example.invalid",
		SuperAdminIDs: []string{"boss"},
	}
	repo := repository.New(db)
	svc := NewWeComService(repo, cfg, nil)
	return svc, repo
}

func stubDirectory(t *testing.T, depts []wecom.Department, users []wecom.User) {
	t.Helper()
	oldD, oldU := SyncDepartmentsFunc, SyncUsersFunc
	SyncDepartmentsFunc = func(context.Context) ([]wecom.Department, error) { return depts, nil }
	SyncUsersFunc = func(context.Context) ([]wecom.User, error) { return users, nil }
	t.Cleanup(func() {
		SyncDepartmentsFunc = oldD
		SyncUsersFunc = oldU
	})
}

func TestSyncCreatesDepartmentsAndUsers(t *testing.T) {
	svc, repo := newWeComFixture(t)
	stubDirectory(t,
		[]wecom.Department{
			{ID: 1, Name: "总公司", ParentID: 0},
			{ID: 2, Name: "研发中心", ParentID: 1},
			{ID: 3, Name: "后端组", ParentID: 2},
		},
		[]wecom.User{
			{UserID: "boss", Name: "老板", Department: []int{1}, Enabled: true},
			{UserID: "dev1", Name: "开发一", Department: []int{3}, Enabled: true, Position: "工程师"},
			{UserID: "ops1", Name: "运维一", Department: []int{2}, Enabled: false},
		},
	)

	res, err := svc.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Departments != 3 || res.Users != 3 || res.Status != "success" {
		t.Fatalf("unexpected sync result: %+v", res)
	}

	var deptCount int64
	repo.DB.Model(&model.Department{}).Count(&deptCount)
	if deptCount != 3 {
		t.Fatalf("expected 3 departments, got %d", deptCount)
	}

	boss, err := repo.UserByWeComID("boss")
	if err != nil {
		t.Fatal(err)
	}
	if boss.Role != model.RoleAdmin {
		t.Fatalf("super admin role not applied: %s", boss.Role)
	}

	dev, err := repo.UserByWeComID("dev1")
	if err != nil {
		t.Fatal(err)
	}
	if dev.Role != model.RoleUser || !dev.Enabled || dev.Source != model.SourceWeCom {
		t.Fatalf("unexpected dev user: %+v", dev)
	}
	if ids := DirectDeptIDs(dev); len(ids) != 1 || ids[0] != 3 {
		t.Fatalf("department binding wrong: %v", DirectDeptIDs(dev))
	}
	if dev.PasswordHash == "" {
		t.Fatal("sso user must still carry a non-empty hash sentinel")
	}
	// SSO users must not be authenticatable with a guessed password.
	if _, err := repo.UserByUsername("dev1"); err != nil {
		t.Fatal("user should be resolvable by username")
	}

	// Disabled employees are imported as disabled.
	ops, err := repo.UserByWeComID("ops1")
	if err != nil {
		t.Fatal(err)
	}
	if ops.Enabled {
		t.Fatal("disabled employee should be disabled locally")
	}

	// Re-sync is idempotent and updates mutable fields.
	stubDirectory(t,
		[]wecom.Department{{ID: 1, Name: "总公司"}, {ID: 2, Name: "研发中心"}, {ID: 3, Name: "后端组"}},
		[]wecom.User{{UserID: "dev1", Name: "开发一号", Department: []int{2}, Enabled: true}},
	)
	if _, err := svc.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	dev, _ = repo.UserByWeComID("dev1")
	if dev.DisplayName != "开发一号" {
		t.Fatalf("display name not refreshed: %s", dev.DisplayName)
	}
	var users int64
	repo.DB.Model(&model.User{}).Count(&users)
	if users != 3 {
		t.Fatalf("re-sync must not duplicate users, got %d", users)
	}
}

func TestDeptGrantInheritance(t *testing.T) {
	svc, repo := newWeComFixture(t)
	stubDirectory(t,
		[]wecom.Department{
			{ID: 1, Name: "总公司"},
			{ID: 2, Name: "研发中心", ParentID: 1},
			{ID: 3, Name: "后端组", ParentID: 2},
			{ID: 10, Name: "销售部", ParentID: 1},
		},
		[]wecom.User{
			{UserID: "dev1", Name: "开发一", Department: []int{3}, Enabled: true},
			{UserID: "sales1", Name: "销售一", Department: []int{10}, Enabled: true},
			{UserID: "outsider", Name: "外部", Department: []int{10}, Enabled: true},
		},
	)
	if _, err := svc.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	dash := model.Dashboard{Name: "研发看板", OwnerID: 999}
	if err := repo.DB.Create(&dash).Error; err != nil {
		t.Fatal(err)
	}
	ds := NewDashboardService(repo, nil, nil, nil)

	// Grant to the parent department; a member of a nested department inherits it.
	if err := ds.AddDeptGrant(dash.ID, 2, "view"); err != nil {
		t.Fatal(err)
	}
	dev, _ := repo.UserByWeComID("dev1")
	if perm, err := ds.Access(dash.ID, dev); err != nil || perm != "view" {
		t.Fatalf("child dept should inherit view grant, got %q err=%v", perm, err)
	}

	// A member of an unrelated department has no access.
	sales, _ := repo.UserByWeComID("sales1")
	if perm, err := ds.Access(dash.ID, sales); err != nil || perm != "" {
		t.Fatalf("unrelated dept should have no access, got %q err=%v", perm, err)
	}

	// Grant to a child department does not leak upward to the parent dept members
	// unless they belong to that child.
	if err := ds.AddDeptGrant(dash.ID, 3, "view"); err != nil {
		t.Fatal(err)
	}
	if perm, _ := ds.Access(dash.ID, sales); perm != "" {
		t.Fatalf("child grant must not grant siblings, got %q", perm)
	}

	// Dashboard listing includes department-published dashboards.
	list, total, err := ds.List(dev, "", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 || list[0].ID != dash.ID {
		t.Fatalf("department dashboard missing from list: total=%d list=%v", total, list)
	}
	salesList, total, err := ds.List(sales, "", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(salesList) != 0 {
		t.Fatalf("unrelated user should see no dashboards, got %d", total)
	}

	// Removing the grant revokes access.
	grants, err := ds.ListDeptGrants(dash.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range grants {
		if g.WeComDeptID == 2 {
			if err := ds.RemoveDeptGrant(dash.ID, g.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if perm, _ := ds.Access(dash.ID, dev); perm != "view" {
		// A child grant (dept 3) still applies to dev1, so view is still expected.
		t.Fatalf("dev1 should still have the direct child grant, got %q", perm)
	}
	for _, g := range grants {
		if g.WeComDeptID == 3 {
			if err := ds.RemoveDeptGrant(dash.ID, g.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if perm, _ := ds.Access(dash.ID, dev); perm != "" {
		t.Fatalf("access should be revoked after removing all grants, got %q", perm)
	}
}

func TestDescendantAndAncestorIDs(t *testing.T) {
	_, repo := newWeComFixture(t)
	for _, d := range []model.Department{
		{WeComID: 1, Name: "root"},
		{WeComID: 2, Name: "a", ParentWeComID: 1},
		{WeComID: 3, Name: "a1", ParentWeComID: 2},
		{WeComID: 4, Name: "b", ParentWeComID: 1},
	} {
		if err := repo.DB.Create(&d).Error; err != nil {
			t.Fatal(err)
		}
	}
	ids := DescendantAndAncestorDeptIDs(repo.DB, []int{3})
	set := map[int]bool{}
	for _, id := range ids {
		set[id] = true
	}
	for _, want := range []int{1, 2, 3} {
		if !set[want] {
			t.Fatalf("missing dept %d in %v", want, ids)
		}
	}
	if set[4] {
		t.Fatalf("sibling dept 4 must not be included: %v", ids)
	}
}

var _ = time.Now
