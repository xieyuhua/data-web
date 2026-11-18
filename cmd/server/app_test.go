package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"biaoge-bi/internal/auth"
	"biaoge-bi/internal/config"
	"biaoge-bi/internal/crypto"
	"biaoge-bi/internal/datasource"
	"biaoge-bi/internal/handler"
	"biaoge-bi/internal/model"
	"biaoge-bi/internal/query"
	"biaoge-bi/internal/repository"
	"biaoge-bi/internal/server"
	"biaoge-bi/internal/service"
)

type testApp struct {
	srv   *httptest.Server
	token string
	data  *gorm.DB
}

type apiResp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (a *testApp) call(t *testing.T, method, path string, body any, token string) (int, apiResp, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, a.srv.URL+path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var ar apiResp
	_ = json.Unmarshal(raw, &ar)
	return resp.StatusCode, ar, raw
}

func (a *testApp) mustOK(t *testing.T, method, path string, body any, token string) apiResp {
	t.Helper()
	status, ar, raw := a.call(t, method, path, body, token)
	if status != 200 || ar.Code != 0 {
		t.Fatalf("%s %s -> %d %s", method, path, status, string(raw))
	}
	return ar
}

func setupApp(t *testing.T) *testApp {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()

	meta, err := gorm.Open(sqlite.Open(filepath.Join(dir, "meta.db")), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := meta.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := meta.AutoMigrate(
		&model.User{}, &model.DataSource{}, &model.Dataset{}, &model.DatasetParam{},
		&model.Chart{}, &model.Dashboard{}, &model.DashboardWidget{}, &model.DashboardFilter{},
		&model.FilterBinding{}, &model.DashboardGrant{}, &model.ShareLink{},
		&model.ScheduleTask{}, &model.ScheduleRun{}, &model.AuditLog{},
		&model.Department{}, &model.DashboardDeptGrant{}, &model.SyncState{},
		&model.BigRegion{}, &model.Region{}, &model.Store{}, &model.Setting{},
	); err != nil {
		t.Fatal(err)
	}

	// Data source DB
	dataPath := filepath.Join(dir, "sales.db")
	dataDB, err := gorm.Open(sqlite.Open(dataPath), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := dataDB.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := dataDB.Exec(`CREATE TABLE sales (region TEXT, product TEXT, order_date TEXT, amount REAL, quantity INTEGER)`).Error; err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		region string
		amount float64
	}{
		{"east", 100}, {"east", 200}, {"west", 50}, {"west", 150}, {"north", 75},
	}
	for i, r := range rows {
		if err := dataDB.Exec(`INSERT INTO sales VALUES (?, 'p', '2026-01-0'+?, ?, 1)`,
			r.region, fmt.Sprintf("%d", i+1), r.amount).Error; err != nil {
			t.Fatal(err)
		}
	}

	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Auth.JWTSecret = "0123456789abcdef0123456789abcdef"
	cfg.Auth.AdminPassword = "admin12345"
	cfg.Auth.TokenTTLHours = 1
	cfg.Scheduler.Enabled = false
	cfg.Scheduler.ReportDir = filepath.Join(dir, "reports")

	cipher, err := crypto.NewCipher("test-secret")
	if err != nil {
		t.Fatal(err)
	}
	jwtMgr, err := auth.NewJWTManager(cfg.Auth.JWTSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	repo := repository.New(meta)
	manager := datasource.NewManager(meta, cipher, cfg)
	t.Cleanup(manager.Close)
	executor := &query.Executor{Manager: manager}
	cache := query.NewResultCache(time.Minute, 50)

	authSvc := service.NewAuthService(repo, jwtMgr, cfg)
	userSvc := service.NewUserService(repo)
	dsSvc := service.NewDataSourceService(repo, manager, cipher)
	datasetSvc := service.NewDatasetService(repo, executor)
	chartSvc := service.NewChartService(repo, executor, cache, service.ChartLimits{Timeout: 15 * time.Second, MaxRows: 5000})
	dashSvc := service.NewDashboardService(repo, chartSvc, datasetSvc, executor)
	shareSvc := service.NewShareService(repo)
	exportSvc := service.NewExportService(repo, chartSvc, dashSvc)
	auditSvc := service.NewAuditService(repo)
	wecomSvc := service.NewWeComService(repo, cfg, authSvc)

	if err := authSvc.BootstrapAdmin(); err != nil {
		t.Fatal(err)
	}

	loginRate := auth.NewLimiter(5, 5)
	publicRate := auth.NewLimiter(1000, 1000)
	t.Cleanup(loginRate.Close)
	t.Cleanup(publicRate.Close)

	handlers := &server.Handlers{
		Auth:       handler.NewAuthHandler(authSvc, userSvc, loginRate, auditSvc),
		DataSource: handler.NewDataSourceHandler(dsSvc, auditSvc),
		Dataset:    handler.NewDatasetHandler(datasetSvc, auditSvc),
		Chart:      handler.NewChartHandler(chartSvc, exportSvc, auditSvc),
		Dashboard:  handler.NewDashboardHandler(dashSvc, shareSvc, exportSvc, auditSvc),
		Public:     handler.NewPublicHandler(shareSvc, dashSvc, publicRate),
		Schedule:   handler.NewScheduleHandler(repo, dashSvc),
		Settings:   handler.NewSettingsHandler(config.NewRuntime(cfg)),
		Setting:    handler.NewSettingHandler(service.NewSettingService(repo, config.NewRuntime(cfg)), auditSvc),
		AI:         handler.NewAIHandler(service.NewAIService(repo, manager, config.NewRuntime(cfg), cfg), datasetSvc, auditSvc),
		Audit:      handler.NewAuditHandler(auditSvc),
		WeCom:      handler.NewWeComHandler(wecomSvc, dashSvc, auditSvc),
		Territory:  handler.NewTerritoryHandler(service.NewTerritoryService(repo), service.NewOrgImportService(repo), auditSvc),
	}
	router := server.NewRouter(server.Deps{Config: cfg, Auth: authSvc, Handlers: handlers, LoginRate: loginRate})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return &testApp{srv: srv, data: dataDB}
}

func TestEndToEnd(t *testing.T) {
	app := setupApp(t)

	// Login as bootstrap admin
	ar := app.mustOK(t, "POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin12345"}, "")
	var login struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(ar.Data, &login); err != nil || login.Token == "" {
		t.Fatalf("login failed: %s", string(ar.Data))
	}
	adminToken := login.Token

	// Wrong password -> 401
	if status, _, _ := app.call(t, "POST", "/api/auth/login", map[string]string{"username": "admin", "password": "wrong"}, ""); status != 401 {
		t.Fatalf("wrong password should be 401, got %d", status)
	}

	// External SQLite data source in a temp directory.
	dsPath := filepath.Join(t.TempDir(), "e2e.db")
	seed, err := gorm.Open(sqlite.Open(dsPath), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	seed.Exec(`CREATE TABLE sales (region TEXT, amount REAL)`)
	seed.Exec(`INSERT INTO sales VALUES ('east', 100), ('east', 200), ('west', 50)`)
	sqlDB, _ := seed.DB()
	sqlDB.Close()

	dsResp := app.mustOK(t, "POST", "/api/datasources", map[string]any{
		"name": "e2e-sqlite", "type": "sqlite", "database": dsPath,
	}, adminToken)
	var ds struct {
		ID          uint   `json:"id"`
		HasPassword bool   `json:"hasPassword"`
		PasswordEnc string `json:"passwordEnc"`
	}
	json.Unmarshal(dsResp.Data, &ds)
	if ds.ID == 0 {
		t.Fatal("datasource id missing")
	}
	if ds.PasswordEnc != "" {
		t.Fatal("password must never be exposed")
	}
	app.mustOK(t, "POST", fmt.Sprintf("/api/datasources/%d/test", ds.ID), nil, adminToken)

	// Dataset validation: DML rejected
	if status, _, _ := app.call(t, "POST", "/api/datasets", map[string]any{
		"name": "bad", "dataSourceId": ds.ID, "sqlText": "DELETE FROM sales",
	}, adminToken); status != 400 {
		t.Fatalf("DML should be rejected with 400, got %d", status)
	}
	// Undeclared placeholder rejected
	if status, _, _ := app.call(t, "POST", "/api/datasets", map[string]any{
		"name": "bad2", "dataSourceId": ds.ID, "sqlText": "SELECT * FROM sales WHERE amount > {{min}}",
	}, adminToken); status != 400 {
		t.Fatalf("undeclared param should be 400, got %d", status)
	}

	dsResp = app.mustOK(t, "POST", "/api/datasets", map[string]any{
		"name": "sales", "dataSourceId": ds.ID,
		"sqlText": "SELECT region, amount FROM sales WHERE amount >= {{min_amount}}",
		"params": []map[string]any{
			{"name": "min_amount", "type": "number", "required": true, "defaultValue": "0"},
		},
	}, adminToken)
	var dataset struct {
		ID             uint   `json:"id"`
		ColumnMetaJSON string `json:"columnMetaJson"`
	}
	json.Unmarshal(dsResp.Data, &dataset)
	if dataset.ID == 0 || dataset.ColumnMetaJSON == "" {
		t.Fatalf("dataset create/infer failed: %s", string(dsResp.Data))
	}

	preview := app.mustOK(t, "POST", fmt.Sprintf("/api/datasets/%d/preview", dataset.ID),
		map[string]any{"params": map[string]string{"min_amount": "60"}}, adminToken)
	var rs query.ResultSet
	json.Unmarshal(preview.Data, &rs)
	if rs.RowCount != 2 {
		t.Fatalf("expected 2 rows >= 60 (100, 200), got %d", rs.RowCount)
	}

	// Chart: sum by region
	chartResp := app.mustOK(t, "POST", "/api/charts", map[string]any{
		"name": "by region", "datasetId": dataset.ID, "type": "bar",
		"config": map[string]any{
			"chartType":  "bar",
			"dataMode":   "aggregate",
			"dimensions": []map[string]string{{"field": "region", "alias": "Region"}},
			"metrics":    []map[string]string{{"field": "amount", "agg": "sum", "alias": "Total"}},
			"sort":       []map[string]any{{"alias": "Total", "desc": true}},
			"limit":      100,
		},
	}, adminToken)
	var chart struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(chartResp.Data, &chart)

	dataResp := app.mustOK(t, "POST", fmt.Sprintf("/api/charts/%d/data", chart.ID),
		map[string]any{"params": map[string]string{"min_amount": "0"}}, adminToken)
	var chartData query.ResultSet
	json.Unmarshal(dataResp.Data, &chartData)
	if chartData.RowCount != 2 {
		t.Fatalf("expected 2 regions, got %d", chartData.RowCount)
	}
	// rows sorted desc: east 300, west 50
	if chartData.Rows[0][1].(float64) != 300 {
		t.Fatalf("unexpected first row: %v", chartData.Rows[0])
	}
	if chartData.Columns[1].LogicalType != "number" {
		t.Fatalf("aggregate column should be numeric, got %s", chartData.Columns[1].LogicalType)
	}

	// Dashboard + layout + data
	dashResp := app.mustOK(t, "POST", "/api/dashboards", map[string]any{"name": "E2E"}, adminToken)
	var dash struct {
		ID uint `json:"id"`
	}
	json.Unmarshal(dashResp.Data, &dash)
	layoutResp := app.mustOK(t, "PUT", fmt.Sprintf("/api/dashboards/%d/layout", dash.ID), map[string]any{
		"widgets": []map[string]any{
			{"widgetType": "chart", "chartId": chart.ID, "title": "by region", "x": 0, "y": 0, "w": 6, "h": 8},
		},
		"filters": []map[string]any{
			{"name": "min_amount", "label": "Min", "filterType": "number", "defaultValueJson": "0"},
		},
	}, adminToken)
	var layout struct {
		Widgets []model.DashboardWidget `json:"widgets"`
		Filters []model.DashboardFilter `json:"filters"`
	}
	json.Unmarshal(layoutResp.Data, &layout)
	if len(layout.Widgets) != 1 || len(layout.Filters) != 1 {
		t.Fatalf("layout not saved: %+v", layout)
	}
	app.mustOK(t, "PUT", fmt.Sprintf("/api/dashboards/%d/bindings", dash.ID), map[string]any{
		"bindings": []map[string]any{{
			"filterId": layout.Filters[0].ID, "widgetId": layout.Widgets[0].ID,
			"paramName": "min_amount", "valueIndex": 0,
		}},
	}, adminToken)

	dashData := app.mustOK(t, "POST", fmt.Sprintf("/api/dashboards/%d/data", dash.ID),
		map[string]any{"filterValues": map[string]any{"min_amount": 100}}, adminToken)
	var dd service.DashboardData
	json.Unmarshal(dashData.Data, &dd)
	if len(dd.Widgets) != 1 {
		t.Fatalf("dashboard widgets missing: %+v", dd)
	}
	for _, wr := range dd.Widgets {
		if wr.Error != "" {
			t.Fatalf("widget error: %s", wr.Error)
		}
		// Only 'east' has rows >= 100 after the filter is applied.
		if wr.Result == nil || wr.Result.RowCount != 1 || wr.Result.Rows[0][1].(float64) != 300 {
			t.Fatalf("filter not applied: %+v", wr.Result)
		}
	}

	// XLSX export
	status, _, raw := app.call(t, "POST", fmt.Sprintf("/api/dashboards/%d/export", dash.ID),
		map[string]any{"filterValues": map[string]any{}}, adminToken)
	if status != 200 || len(raw) < 4 || raw[0] != 'P' || raw[1] != 'K' {
		t.Fatalf("xlsx export invalid: status=%d len=%d", status, len(raw))
	}

	// Share link + public access + revoke
	shareResp := app.mustOK(t, "POST", fmt.Sprintf("/api/dashboards/%d/shares", dash.ID), map[string]any{}, adminToken)
	var share struct {
		ID    uint   `json:"id"`
		Token string `json:"token"`
	}
	json.Unmarshal(shareResp.Data, &share)
	if share.Token == "" {
		t.Fatal("share token missing")
	}
	pub := app.mustOK(t, "GET", "/api/public/dashboards/"+share.Token, nil, "")
	if bytes.Contains(pub.Data, []byte("sqlText")) || bytes.Contains(pub.Data, []byte("SELECT")) {
		t.Fatal("public view must not leak SQL")
	}
	app.mustOK(t, "POST", "/api/public/dashboards/"+share.Token+"/data",
		map[string]any{"filterValues": map[string]any{}}, "")
	app.mustOK(t, "DELETE", fmt.Sprintf("/api/dashboards/%d/shares/%d", dash.ID, share.ID), nil, adminToken)
	if status, _, _ := app.call(t, "GET", "/api/public/dashboards/"+share.Token, nil, ""); status != 404 {
		t.Fatalf("revoked share should 404, got %d", status)
	}

	// Normal user permissions
	app.mustOK(t, "POST", "/api/users", map[string]any{
		"username": "bob", "password": "bob1234567", "role": "user",
	}, adminToken)
	ar = app.mustOK(t, "POST", "/api/auth/login", map[string]string{"username": "bob", "password": "bob1234567"}, "")
	json.Unmarshal(ar.Data, &login)
	bobToken := login.Token
	if status, _, _ := app.call(t, "GET", "/api/users", nil, bobToken); status != 403 {
		t.Fatalf("user should not list users, got %d", status)
	}
	if status, _, _ := app.call(t, "GET", fmt.Sprintf("/api/dashboards/%d", dash.ID), nil, bobToken); status != 403 {
		t.Fatalf("ungranted dashboard should 403, got %d", status)
	}
	var bob struct {
		ID uint `json:"id"`
	}
	// grant view to bob
	users := app.mustOK(t, "GET", "/api/users?page=1&pageSize=50", nil, adminToken)
	var page struct {
		Items []model.User `json:"items"`
	}
	json.Unmarshal(users.Data, &page)
	for _, u := range page.Items {
		if u.Username == "bob" {
			bob.ID = u.ID
		}
	}
	app.mustOK(t, "POST", fmt.Sprintf("/api/dashboards/%d/grants", dash.ID), map[string]any{
		"userId": bob.ID, "permission": "view",
	}, adminToken)
	app.mustOK(t, "GET", fmt.Sprintf("/api/dashboards/%d", dash.ID), nil, bobToken)
	if status, _, _ := app.call(t, "PUT", fmt.Sprintf("/api/dashboards/%d/layout", dash.ID),
		map[string]any{"widgets": []any{}, "filters": []any{}}, bobToken); status != 403 {
		t.Fatalf("view-only grant must not edit, got %d", status)
	}

	// Login rate limiting
	for i := 0; i < 8; i++ {
		app.call(t, "POST", "/api/auth/login", map[string]string{"username": "rate", "password": "wrong"}, "")
	}
	if status, _, _ := app.call(t, "POST", "/api/auth/login", map[string]string{"username": "rate", "password": "wrong"}, ""); status != 429 {
		t.Fatalf("expected rate limit 429, got %d", status)
	}
}

func TestHealth(t *testing.T) {
	app := setupApp(t)
	status, ar, _ := app.call(t, "GET", "/api/health", nil, "")
	if status != 200 || ar.Code != 0 {
		t.Fatalf("health failed: %d", status)
	}
}
