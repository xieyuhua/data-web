package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
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
	"biaoge-bi/internal/scheduler"
	"biaoge-bi/internal/server"
	"biaoge-bi/internal/service"

	// Pure-Go SQLite dialector (modernc), no CGO required.
	"github.com/glebarez/sqlite"
)

func main() {
	configFile := flag.String("config", "", "path to YAML config file")
	flag.Parse()

	cfg, err := config.Load(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	setupLogger(cfg.Logging.Level)
	slog.Info("启动 BI 平台", "version", "1.0.0")
	service.SetConfigPath(*configFile)

	// ---------- Metadata DB ----------
	if err := config.EnsureDir(cfg.Metadata.DSN); err != nil {
		slog.Error("创建元数据库目录失败", "error", err)
		os.Exit(1)
	}
	meta, err := openMetadata(cfg)
	if err != nil {
		slog.Error("连接元数据库失败", "error", err)
		os.Exit(1)
	}
	if err := meta.AutoMigrate(
		&model.User{}, &model.DataSource{}, &model.Dataset{}, &model.DatasetParam{},
		&model.Chart{}, &model.Dashboard{}, &model.DashboardWidget{}, &model.DashboardFilter{},
		&model.FilterBinding{}, &model.DashboardGrant{}, &model.ShareLink{},
		&model.ScheduleTask{}, &model.ScheduleRun{}, &model.AuditLog{},
		&model.Department{}, &model.DashboardDeptGrant{}, &model.SyncState{},
		&model.BigRegion{}, &model.Region{}, &model.Store{}, &model.Setting{},
	); err != nil {
		slog.Error("元数据库迁移失败", "error", err)
		os.Exit(1)
	}

	// ---------- Core wiring ----------
	cipher, err := crypto.NewCipher(cfg.Security.SecretKey)
	if err != nil {
		slog.Error("初始化加密组件失败", "error", err)
		os.Exit(1)
	}
	jwtMgr, err := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.TokenTTL())
	if err != nil {
		slog.Error("JWT 配置无效", "error", err)
		os.Exit(1)
	}

	repo := repository.New(meta)

	// Load stored overrides so admin changes survive restarts, then publish a
	// live snapshot that every service reads instead of the boot-time config.
	settingSvc := service.NewSettingService(repo, config.NewRuntime(cfg))
	if merged, err := settingSvc.LoadWithOverrides(); err != nil {
		slog.Warn("加载运行时配置覆盖失败，使用文件配置", "error", err)
	} else if merged != nil {
		cfg = merged
		setupLogger(cfg.Logging.Level)
	}
	rt := config.NewRuntime(cfg)
	settingSvc = service.NewSettingService(repo, rt)

	resultCache := query.NewResultCache(time.Duration(cfg.Cache.TTLSeconds)*time.Second, cfg.Cache.MaxEntries)

	// Hot-reload hooks: rebuild caches / adjust logger when settings change.
	settingSvc.OnApply("cache", func(c *config.Config) {
		resultCache.Reconfigure(time.Duration(c.Cache.TTLSeconds)*time.Second, c.Cache.MaxEntries)
	})
	settingSvc.OnApply("logging", func(c *config.Config) {
		setupLogger(c.Logging.Level)
		slog.Info("日志级别已热更新", "level", c.Logging.Level)
	})
	dsManager := datasource.NewManager(meta, cipher, cfg)
	defer dsManager.Close()
	executor := &query.Executor{Manager: dsManager}

	authSvc := service.NewAuthService(repo, jwtMgr, cfg)
	userSvc := service.NewUserService(repo)
	dsSvc := service.NewDataSourceService(repo, dsManager, cipher)
	datasetSvc := service.NewDatasetService(repo, executor)
	chartSvc := service.NewChartService(repo, executor, resultCache, service.ChartLimits{
		Timeout: cfg.QueryTimeout(),
		MaxRows: cfg.DataSource.MaxResultRows,
	})
	chartSvc.SetRuntime(rt)
	dashboardSvc := service.NewDashboardService(repo, chartSvc, datasetSvc, executor)
	shareSvc := service.NewShareService(repo)
	exportSvc := service.NewExportService(repo, chartSvc, dashboardSvc)
	auditSvc := service.NewAuditService(repo)
	aiSvc := service.NewAIService(repo, dsManager, rt, cfg)
	territorySvc := service.NewTerritoryService(repo)
	orgImportSvc := service.NewOrgImportService(repo)
	wecomSvc := service.NewWeComService(repo, cfg, authSvc)
	wecomSvc.SetRuntime(rt)

	if err := authSvc.BootstrapAdmin(); err != nil {
		slog.Error("初始化管理员失败", "error", err)
		os.Exit(1)
	}

	// ---------- Scheduler ----------
	sched := scheduler.New(cfg, meta, scheduler.NewBuilder(exportSvc))
	sched.SetRuntime(rt)
	handler.SchedulerHooks.RunNow = func(taskID uint) (*model.ScheduleRun, error) {
		return sched.RunNow(taskID)
	}
	handler.SchedulerHooks.Reload = sched.Reload
	// WeCom push hooks: build a time-limited share URL and a KPI summary line.
	scheduler.SetWeComHooks(
		func(dashboardID uint, expireHours int) (string, error) {
			token, err := shareSvc.CreateForPush(dashboardID, 0, expireHours)
			if err != nil {
				return "", err
			}
			base := cfg.Server.PublicURL
			if base == "" {
				base = fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port)
			}
			return strings.TrimRight(base, "/") + "/share/" + token, nil
		},
		func(ctx context.Context, dashboardID uint) ([]string, error) {
			return dashboardSvc.KeyMetricsSummary(ctx, dashboardID)
		},
	)

	sched.Start()
	defer sched.Stop()

	// Periodic 企业微信 contacts sync (disabled when WeCom is not configured).
	if wecomSvc.Enabled() && cfg.WeCom.SyncIntervalHours > 0 {
		go wecomSyncLoop(wecomSvc, time.Duration(cfg.WeCom.SyncIntervalHours)*time.Hour)
	}

	// ---------- HTTP ----------
	loginRate := auth.NewLimiter(5, 5)
	defer loginRate.Close()
	publicRate := auth.NewLimiter(120, 60)
	defer publicRate.Close()

	handlers := &server.Handlers{
		Auth:       handler.NewAuthHandler(authSvc, userSvc, loginRate, auditSvc),
		DataSource: handler.NewDataSourceHandler(dsSvc, auditSvc),
		Dataset:    handler.NewDatasetHandler(datasetSvc, auditSvc),
		Chart:      handler.NewChartHandler(chartSvc, exportSvc, auditSvc),
		Dashboard:  handler.NewDashboardHandler(dashboardSvc, shareSvc, exportSvc, auditSvc),
		Public:     handler.NewPublicHandler(shareSvc, dashboardSvc, publicRate),
		Schedule:   handler.NewScheduleHandler(repo, dashboardSvc),
		Settings:   handler.NewSettingsHandler(rt),
		Setting:    handler.NewSettingHandler(settingSvc, auditSvc),
		AI:         handler.NewAIHandler(aiSvc, datasetSvc, auditSvc),
		Audit:      handler.NewAuditHandler(auditSvc),
		WeCom:      handler.NewWeComHandler(wecomSvc, dashboardSvc, auditSvc),
		Territory:  handler.NewTerritoryHandler(territorySvc, orgImportSvc, auditSvc),
	}
	router := server.NewRouter(server.Deps{
		Config: cfg, Auth: authSvc, Handlers: handlers, LoginRate: loginRate,
	})

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: 5 * time.Minute, // exports can be large
		IdleTimeout:  120 * time.Second,
	}
	go func() {
		slog.Info("HTTP 服务已启动", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP 服务异常退出", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("正在关闭服务...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	slog.Info("服务已停止")
}

func wecomSyncLoop(svc *service.WeComService, interval time.Duration) {
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		res, err := svc.Sync(ctx)
		if err != nil {
			slog.Warn("企业微信通讯录同步失败", "error", err)
			return
		}
		slog.Info("企业微信通讯录同步完成", "departments", res.Departments, "users", res.Users)
	}
	run()
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		run()
	}
}

func setupLogger(level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
		gin.SetMode(gin.DebugMode)
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})))
}

func openMetadata(cfg *config.Config) (*gorm.DB, error) {
	gcfg := &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Warn)}
	switch strings.ToLower(cfg.Metadata.Driver) {
	case "mysql":
		return gorm.Open(mysql.Open(cfg.Metadata.DSN), gcfg)
	default:
		return gorm.Open(sqlite.Open(cfg.Metadata.DSN), gcfg)
	}
}
