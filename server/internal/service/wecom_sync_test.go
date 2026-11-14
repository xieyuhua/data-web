package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"bi-platform/internal/config"
	"bi-platform/internal/model"
	"bi-platform/internal/pkg/auth"
	"bi-platform/internal/pkg/secretbox"
	"bi-platform/internal/repository"
	"bi-platform/internal/wecom"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fakeWecomClient struct {
	departments    []wecom.Department
	users          map[int64][]wecom.User
	departmentErr  error
	userErr        error
	loginUser      *wecom.LoginUser
	loginErr       error
	blockDeparture chan struct{}
}

func (f *fakeWecomClient) Departments(ctx context.Context) ([]wecom.Department, error) {
	if f.blockDeparture != nil {
		<-f.blockDeparture
	}
	if f.departmentErr != nil {
		return nil, f.departmentErr
	}
	return f.departments, nil
}

func (f *fakeWecomClient) Users(ctx context.Context, departmentID int64, fetchChild bool) ([]wecom.User, error) {
	if f.userErr != nil {
		return nil, f.userErr
	}
	if fetchChild {
		result := make([]wecom.User, 0)
		for _, list := range f.users {
			result = append(result, list...)
		}
		return result, nil
	}
	return f.users[departmentID], nil
}

func (f *fakeWecomClient) UserInfoByCode(ctx context.Context, code string) (*wecom.LoginUser, error) {
	if f.loginErr != nil {
		return nil, f.loginErr
	}
	return f.loginUser, nil
}

type wecomTestEnv struct {
	db       *gorm.DB
	repo     *repository.WecomRepository
	box      *secretbox.Box
	userRepo *repository.UserRepository
	roleRepo *repository.RoleRepository
	jwt      *auth.Manager
}

func newWecomTestEnv(t *testing.T) *wecomTestEnv {
	t.Helper()
	dsn := fmt.Sprintf("file:wecomtest_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取数据库句柄失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.Organization{}, &model.Role{}, &model.User{}, &model.WecomSyncConfig{}, &model.WecomSyncJob{}); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	roles := []model.Role{
		{Code: "boss", Name: "集团老总", DataScope: "all", Level: 1},
		{Code: "regional_manager", Name: "区域经理", DataScope: "own_org_tree", Level: 3},
		{Code: "store_staff", Name: "门店员工", DataScope: "own_org", Level: 5},
	}
	for i := range roles {
		if err := db.Create(&roles[i]).Error; err != nil {
			t.Fatalf("创建角色失败: %v", err)
		}
	}
	box, err := secretbox.New("test-secret-for-wecom-sync-2026")
	if err != nil {
		t.Fatalf("初始化加密失败: %v", err)
	}
	return &wecomTestEnv{
		db: db, repo: repository.NewWecomRepository(db), box: box,
		userRepo: repository.NewUserRepository(db), roleRepo: repository.NewRoleRepository(db),
		jwt: auth.NewManager("test-secret-for-wecom-sync-2026", time.Hour),
	}
}

func (e *wecomTestEnv) saveConfig(t *testing.T, enabled, loginEnabled bool) {
	t.Helper()
	svc := NewWecomSyncService(e.repo, e.box)
	yes := enabled
	login := loginEnabled
	_, err := svc.SaveConfig(WecomConfigInput{
		CorpID: "ww-test-corp", AgentID: "1000002", Secret: "test-secret",
		Enabled: &yes, LoginEnabled: &login, SyncUsers: boolPtr(true),
		DefaultRoleCode: "store_staff",
	})
	if err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}
}

func boolPtr(v bool) *bool { return &v }

func newSampleDepartments() []wecom.Department {
	return []wecom.Department{
		{ID: 1, Name: "众智集团", ParentID: 0, Order: 1},
		{ID: 2, Name: "华东大区", ParentID: 1, Order: 2},
		{ID: 3, Name: "上海区域", ParentID: 2, Order: 1},
		{ID: 4, Name: "静安旗舰店", ParentID: 3, Order: 1},
	}
}

func newSampleUsers() map[int64][]wecom.User {
	return map[int64][]wecom.User{
		4: {
			{UserID: "zhangsan", Name: "张三", Mobile: "13800000001", Department: []int64{4}, MainDepartment: 4, Status: 1},
			{UserID: "lisi", Name: "李四", Department: []int64{3}, MainDepartment: 3, Status: 2},
			{UserID: "left", Name: "已离职", Department: []int64{4}, MainDepartment: 4, Status: 5},
		},
	}
}

func waitJob(t *testing.T, repo *repository.WecomRepository, id uint64) *model.WecomSyncJob {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		job, err := repo.Job(id)
		if err == nil && job.Status != "pending" && job.Status != "running" {
			return job
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("同步任务 %d 未在超时时间内完成", id)
	return nil
}

func TestWecomSyncCreatesHierarchyAndUsers(t *testing.T) {
	env := newWecomTestEnv(t)
	env.saveConfig(t, true, true)
	svc := NewWecomSyncService(env.repo, env.box)
	fake := &fakeWecomClient{departments: newSampleDepartments(), users: newSampleUsers()}
	svc.newClient = func(corpID, secret string) wecom.Client { return fake }

	job, err := svc.Start(1, WecomSyncInput{})
	if err != nil {
		t.Fatalf("发起同步失败: %v", err)
	}
	done := waitJob(t, env.repo, job.ID)
	if done.Status != "success" {
		t.Fatalf("期望同步成功，实际 %s: %s", done.Status, done.Error)
	}
	if done.Departments != 4 {
		t.Fatalf("期望 4 个部门，实际 %d", done.Departments)
	}
	var orgs []model.Organization
	if err := env.db.Order("id asc").Find(&orgs).Error; err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 4 {
		t.Fatalf("期望创建 4 个组织，实际 %d", len(orgs))
	}
	byExternal := make(map[string]model.Organization)
	for _, org := range orgs {
		if org.ExternalSource != "wecom" || org.ExternalID == "" {
			t.Fatalf("组织缺少企业微信来源标记: %+v", org)
		}
		byExternal[org.ExternalID] = org
	}
	if byExternal["1"].Type != "company" || byExternal["2"].Type != "big_region" ||
		byExternal["3"].Type != "region" || byExternal["4"].Type != "store" {
		t.Fatalf("组织类型映射不正确: %+v", byExternal)
	}
	if byExternal["4"].ParentID == nil || *byExternal["4"].ParentID != byExternal["3"].ID {
		t.Fatalf("门店未挂到区域下: %+v", byExternal["4"])
	}
	if byExternal["4"].Code != "WECOM_DEPT_4" {
		t.Fatalf("组织编码不符合约定: %s", byExternal["4"].Code)
	}

	var users []model.User
	if err := env.db.Order("id asc").Find(&users).Error; err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 {
		t.Fatalf("期望创建 2 个成员（离职跳过），实际 %d", len(users))
	}
	byUsername := make(map[string]model.User, len(users))
	for _, item := range users {
		byUsername[item.Username] = item
	}
	zhangsan, ok := byUsername["zhangsan"]
	if !ok || zhangsan.OrganizationID != byExternal["4"].ID {
		t.Fatalf("store manager sync mismatch: %+v", users)
	}
	lisi, ok := byUsername["lisi"]
	if !ok || lisi.Status != "disabled" {
		t.Fatalf("disabled member status not synced: %+v", users)
	}
	if zhangsan.RoleCode != "store_staff" || zhangsan.PasswordHash == "" {
		t.Fatalf("default role or initial password invalid: %+v", zhangsan)
	}
}

func TestWecomSyncIsIdempotent(t *testing.T) {
	env := newWecomTestEnv(t)
	env.saveConfig(t, true, true)
	svc := NewWecomSyncService(env.repo, env.box)
	fake := &fakeWecomClient{departments: newSampleDepartments(), users: newSampleUsers()}
	svc.newClient = func(corpID, secret string) wecom.Client { return fake }

	first, err := svc.Start(1, WecomSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	waitJob(t, env.repo, first.ID)

	second, err := svc.Start(1, WecomSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	done := waitJob(t, env.repo, second.ID)
	if done.Status != "success" {
		t.Fatalf("二次同步失败: %s %s", done.Status, done.Error)
	}
	if done.Created != 0 || done.Updated < 6 {
		t.Fatalf("二次同步应全部命中更新，实际 created=%d updated=%d", done.Created, done.Updated)
	}
	var orgCount, userCount int64
	env.db.Model(&model.Organization{}).Count(&orgCount)
	env.db.Model(&model.User{}).Count(&userCount)
	if orgCount != 4 || userCount != 2 {
		t.Fatalf("二次同步产生了重复数据: orgs=%d users=%d", orgCount, userCount)
	}
}

func TestWecomSyncDisablesRemovedDepartments(t *testing.T) {
	env := newWecomTestEnv(t)
	env.saveConfig(t, true, true)
	svc := NewWecomSyncService(env.repo, env.box)
	fake := &fakeWecomClient{departments: newSampleDepartments(), users: newSampleUsers()}
	svc.newClient = func(corpID, secret string) wecom.Client { return fake }
	first, err := svc.Start(1, WecomSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	waitJob(t, env.repo, first.ID)

	fake.departments = newSampleDepartments()[:3]
	fake.users = map[int64][]wecom.User{}
	second, err := svc.Start(1, WecomSyncInput{SyncUsers: boolPtr(false), DisableMissing: boolPtr(true)})
	if err != nil {
		t.Fatal(err)
	}
	done := waitJob(t, env.repo, second.ID)
	if done.Status != "success" {
		t.Fatalf("同步失败: %s %s", done.Status, done.Error)
	}
	var store model.Organization
	if err := env.db.Where("external_id = ?", "4").First(&store).Error; err != nil {
		t.Fatal(err)
	}
	if store.Status != "disabled" {
		t.Fatalf("已移除部门应被停用，实际 %s", store.Status)
	}
}

func TestWecomSyncPartialWhenDepartmentFails(t *testing.T) {
	env := newWecomTestEnv(t)
	env.saveConfig(t, true, true)
	svc := NewWecomSyncService(env.repo, env.box)
	fake := &fakeWecomClient{
		departments: []wecom.Department{{ID: 1, Name: "总部"}},
		userErr:     errors.New("通讯录读取权限不足"),
	}
	svc.newClient = func(corpID, secret string) wecom.Client { return fake }
	job, err := svc.Start(1, WecomSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	done := waitJob(t, env.repo, job.ID)
	if done.Status != "partial" {
		t.Fatalf("部门成功、成员失败应为 partial，实际 %s", done.Status)
	}
	if done.Failed == 0 || !strings.Contains(done.Error, "读取部门") {
		t.Fatalf("失败明细未记录: %+v", done)
	}
}

func TestWecomSyncRejectsConcurrentJobs(t *testing.T) {
	env := newWecomTestEnv(t)
	env.saveConfig(t, true, true)
	svc := NewWecomSyncService(env.repo, env.box)
	block := make(chan struct{})
	fake := &fakeWecomClient{departments: newSampleDepartments(), users: newSampleUsers(), blockDeparture: block}
	svc.newClient = func(corpID, secret string) wecom.Client { return fake }
	if _, err := svc.Start(1, WecomSyncInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Start(1, WecomSyncInput{}); !errors.Is(err, ErrWecomSyncRunning) {
		t.Fatalf("并发任务应被拒绝，实际错误: %v", err)
	}
	close(block)
}

func TestWecomConfigNeverReturnsPlainSecret(t *testing.T) {
	env := newWecomTestEnv(t)
	env.saveConfig(t, true, true)
	svc := NewWecomSyncService(env.repo, env.box)
	view, err := svc.Config()
	if err != nil {
		t.Fatal(err)
	}
	if !view.SecretConfigured || strings.Contains(view.SecretMasked, "test-secret") {
		t.Fatalf("配置视图泄露了 Secret: %+v", view)
	}
	// 用掩码再次保存不应覆盖真实 Secret。
	if _, err := svc.SaveConfig(WecomConfigInput{
		CorpID: view.CorpID, Secret: view.SecretMasked, SyncUsers: boolPtr(true),
	}); err != nil {
		t.Fatalf("保存掩码配置失败: %v", err)
	}
	item, err := env.repo.Config()
	if err != nil {
		t.Fatal(err)
	}
	plain, err := env.box.Decrypt(item.SecretEnc)
	if err != nil || plain != "test-secret" {
		t.Fatalf("Secret 被掩码覆盖: %q %v", plain, err)
	}
}

func TestSortDepartmentsDepthOrder(t *testing.T) {
	sorted, depths, roots := sortDepartments([]wecom.Department{
		{ID: 4, Name: "门店", ParentID: 3},
		{ID: 1, Name: "集团", ParentID: 0},
		{ID: 3, Name: "区域", ParentID: 2},
		{ID: 2, Name: "大区", ParentID: 1},
	})
	want := []int64{1, 2, 3, 4}
	for i, item := range sorted {
		if item.ID != want[i] {
			t.Fatalf("部门排序不正确: %+v", sorted)
		}
	}
	if depths[1] != 1 || depths[2] != 2 || depths[3] != 3 || depths[4] != 4 {
		t.Fatalf("部门深度计算不正确: %+v", depths)
	}
	if len(roots) != 1 || roots[0].ID != 1 {
		t.Fatalf("根部门识别不正确: %+v", roots)
	}
}

func TestWecomTypeRules(t *testing.T) {
	rules := defaultTypeRules()
	if rules.typeForDepth(1) != "company" || rules.typeForDepth(3) != "region" || rules.typeForDepth(6) != "store" {
		t.Fatalf("默认类型映射不正确: %+v", rules)
	}
	custom, err := normalizeTypeRules(WecomTypeRules{DepthTypes: []string{"company", "region"}, LastType: "store"})
	if err != nil {
		t.Fatal(err)
	}
	if custom.typeForDepth(2) != "region" || custom.typeForDepth(3) != "store" {
		t.Fatalf("自定义类型映射不正确: %+v", custom)
	}
	if _, err := normalizeTypeRules(WecomTypeRules{DepthTypes: []string{"invalid"}}); err == nil {
		t.Fatal("非法组织类型应报错")
	}
}

func TestSafeRedirect(t *testing.T) {
	cases := map[string]string{
		"/dashboards":             "/dashboards",
		"https://evil.example":    "/dashboards",
		"//evil.example":          "/dashboards",
		"/a\\b":                   "/dashboards",
		"":                        "/dashboards",
		"/dashboards/1?tab=trend": "/dashboards/1?tab=trend",
	}
	for input, want := range cases {
		if got := SafeRedirect(input); got != want {
			t.Fatalf("SafeRedirect(%q)=%q，期望 %q", input, got, want)
		}
	}
}

func TestWecomAuthAuthorizeAndExchange(t *testing.T) {
	env := newWecomTestEnv(t)
	env.saveConfig(t, true, true)
	org := model.Organization{Name: "总部", Code: "HQ", Type: "company", Level: 1, Status: "active"}
	if err := env.db.Create(&org).Error; err != nil {
		t.Fatal(err)
	}
	user := model.User{
		Name: "张三", Username: "zhangsan", PasswordHash: "x", RoleCode: "store_staff",
		OrganizationID: org.ID, WecomUserID: "zhangsan", Status: "active",
	}
	if err := env.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	authService := NewAuthService(env.userRepo, env.roleRepo, env.jwt)
	svc := NewWecomAuthService(env.repo, env.userRepo, env.box, env.jwt, authService, testConfig())
	fake := &fakeWecomClient{loginUser: &wecom.LoginUser{UserID: "zhangsan"}}
	svc.newClient = func(corpID, secret string) wecom.AuthClient { return fake }

	target, err := svc.AuthorizeURL("auto", "/dashboards/9", false, 0, wecomStateLogin, "http://127.0.0.1:8486")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(target, "login.work.weixin.qq.com") || !strings.Contains(target, "CorpApp") {
		t.Fatalf("浏览器应走扫码登录: %s", target)
	}
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatal("授权地址缺少 state")
	}
	callback, err := svc.Callback(context.Background(), "fake-code", state, "http://127.0.0.1:8486")
	if err != nil {
		t.Fatalf("回调失败: %v", err)
	}
	callbackURL, err := url.Parse(callback)
	if err != nil {
		t.Fatal(err)
	}
	ticket := callbackURL.Query().Get("ticket")
	if ticket == "" || callbackURL.Query().Get("redirect") != "/dashboards/9" {
		t.Fatalf("回调跳转不正确: %s", callback)
	}
	result, err := svc.Exchange(ticket)
	if err != nil {
		t.Fatalf("ticket 兑换失败: %v", err)
	}
	if result.Token == "" || result.User.ID != user.ID {
		t.Fatalf("兑换结果不正确: %+v", result)
	}
	if _, err := svc.Exchange(ticket + "x"); err == nil {
		t.Fatal("篡改的 ticket 应被拒绝")
	}
}

func TestWecomAuthRejectsUnboundUser(t *testing.T) {
	env := newWecomTestEnv(t)
	env.saveConfig(t, true, true)
	authService := NewAuthService(env.userRepo, env.roleRepo, env.jwt)
	svc := NewWecomAuthService(env.repo, env.userRepo, env.box, env.jwt, authService, testConfig())
	fake := &fakeWecomClient{loginUser: &wecom.LoginUser{UserID: "nobody"}}
	svc.newClient = func(corpID, secret string) wecom.AuthClient { return fake }
	target, err := svc.AuthorizeURL("auto", "/dashboards", true, 0, wecomStateLogin, "http://127.0.0.1:8486")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(target, "open.weixin.qq.com") {
		t.Fatalf("企微内应走静默授权: %s", target)
	}
	parsed, _ := url.Parse(target)
	if _, err := svc.Callback(context.Background(), "code", parsed.Query().Get("state"), "http://127.0.0.1:8486"); err == nil {
		t.Fatal("未绑定用户应登录失败")
	}
}

func TestWecomAuthBind(t *testing.T) {
	env := newWecomTestEnv(t)
	env.saveConfig(t, true, true)
	org := model.Organization{Name: "总部", Code: "HQ", Type: "company", Level: 1, Status: "active"}
	if err := env.db.Create(&org).Error; err != nil {
		t.Fatal(err)
	}
	user := model.User{Name: "王五", Username: "wangwu", PasswordHash: "x", RoleCode: "store_staff", OrganizationID: org.ID, Status: "active"}
	if err := env.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	authService := NewAuthService(env.userRepo, env.roleRepo, env.jwt)
	svc := NewWecomAuthService(env.repo, env.userRepo, env.box, env.jwt, authService, testConfig())
	fake := &fakeWecomClient{loginUser: &wecom.LoginUser{UserID: "wangwu-wecom"}}
	svc.newClient = func(corpID, secret string) wecom.AuthClient { return fake }
	target, err := svc.AuthorizeURL("oauth", "/access", true, user.ID, wecomStateBind, "http://127.0.0.1:8486")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(target)
	redirect, err := svc.Callback(context.Background(), "code", parsed.Query().Get("state"), "http://127.0.0.1:8486")
	if err != nil {
		t.Fatalf("绑定失败: %v", err)
	}
	if !strings.Contains(redirect, "/wecom/callback?bound=1") {
		t.Fatalf("绑定回调地址不正确: %s", redirect)
	}
	updated, _ := env.userRepo.FindByID(user.ID)
	if updated.WecomUserID != "wangwu-wecom" {
		t.Fatalf("绑定未写入: %+v", updated)
	}
	if err := svc.Unbind(user.ID); err != nil {
		t.Fatal(err)
	}
	updated, _ = env.userRepo.FindByID(user.ID)
	if updated.WecomUserID != "" {
		t.Fatalf("解绑未生效: %+v", updated)
	}
}

func testConfig() config.Config {
	return config.Config{Port: "8486", APIBaseURL: "http://127.0.0.1:8486"}
}
