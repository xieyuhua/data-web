package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bi-platform/internal/channel"
	"bi-platform/internal/config"
	"bi-platform/internal/database"
	"bi-platform/internal/handler"
	"bi-platform/internal/pkg/auth"
	"bi-platform/internal/pkg/secretbox"
	"bi-platform/internal/repository"
	"bi-platform/internal/router"
	"bi-platform/internal/scheduler"
	"bi-platform/internal/seed"
	"bi-platform/internal/service"
)

// main 是 BI 平台 API 服务入口。
// 启动流程：加载配置 → 初始化数据库与演示数据 → 组装仓储/服务/处理器 →
// 启动定时推送调度器 → 启动 HTTP 服务 → 监听退出信号并优雅关闭。
func main() {
	// 1. 加载并校验配置（端口固定 8486，可用 PORT 覆盖）
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	if location, err := time.LoadLocation(cfg.Timezone); err == nil {
		// 统一进程时区，保证周期任务与日期统计一致
		time.Local = location
	}
	// 2. 初始化数据库连接并自动迁移
	db, err := database.Open(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if cfg.SeedDemoData {
		// 首次启动时写入演示用的组织、用户、销售数据
		if err := seed.Run(db); err != nil {
			log.Fatalf("初始化演示数据失败: %v", err)
		}
	}

	// 3. 初始化仓储层
	jwtManager := auth.NewManager(cfg.JWTSecret, cfg.JWTExpire)
	userRepo := repository.NewUserRepository(db)
	roleRepo := repository.NewRoleRepository(db)
	orgRepo := repository.NewOrganizationRepository(db)
	datasetRepo := repository.NewDatasetRepository(db)
	dashboardRepo := repository.NewDashboardRepository(db)
	dashboardPrefRepo := repository.NewDashboardPreferenceRepository(db)
	templateRepo := repository.NewTemplateRepository(db)
	pushRepo := repository.NewPushRepository(db)
	shareRepo := repository.NewShareRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	if err := auditRepo.Backfill(); err != nil {
		// 回填失败不影响启动，仅记录日志
		log.Printf("审计日志回填失败: %v", err)
	}
	datasourceRepo := repository.NewDatasourceRepository(db)
	targetRepo := repository.NewTargetRepository(db)
	drillRepo := repository.NewDrillRepository(db)
	wecomRepo := repository.NewWecomRepository(db)

	// 4. 初始化服务层（依赖注入）
	scopeService := service.NewScopeService(orgRepo, roleRepo)
	authService := service.NewAuthService(userRepo, roleRepo, jwtManager)
	orgService := service.NewOrganizationService(orgRepo)
	userService := service.NewUserService(userRepo, roleRepo, orgRepo)
	secretBox, err := secretbox.New(cfg.JWTSecret)
	if err != nil {
		log.Fatalf("初始化敏感配置加密失败: %v", err)
	}
	wecomSyncService := service.NewWecomSyncService(wecomRepo, secretBox)
	// 恢复上次异常中断的企业微信同步任务状态
	wecomSyncService.Recover()
	wecomAuthService := service.NewWecomAuthService(wecomRepo, userRepo, secretBox, jwtManager, authService, cfg)
	datasourceService := service.NewDatasourceService(datasourceRepo, secretBox, cfg.SQLQueryTimeout)
	// 进程退出时关闭所有外部数据源连接
	defer datasourceService.CloseAll()
	datasetService := service.NewDatasetService(datasetRepo, datasourceService, cfg.SQLQueryTimeout)
	// 下钻服务在看板服务之前创建：看板指标趋势在没有日期列时会回退到组织范围趋势
	drillService := service.NewDrillService(orgRepo, targetRepo, drillRepo, scopeService)
	dashboardService := service.NewDashboardService(dashboardRepo, datasetRepo, datasetService, drillService)
	dashboardPrefService := service.NewDashboardPreferenceService(dashboardPrefRepo, dashboardRepo)
	templateService := service.NewTemplateService(templateRepo, dashboardRepo)
	shareService := service.NewShareService(shareRepo, dashboardRepo, cfg)
	sender := channel.NewSender(cfg.AllowMockPush)
	pushService := service.NewPushService(pushRepo, userRepo, roleRepo, orgRepo, dashboardRepo, scopeService, shareService, sender)
	targetService := service.NewTargetService(targetRepo, orgRepo)

	// 5. 初始化 HTTP 处理器
	authHandler := handler.NewAuthHandler(authService)
	orgHandler := handler.NewOrganizationHandler(orgService, scopeService)
	userHandler := handler.NewUserHandler(userService, roleRepo, scopeService)
	datasetHandler := handler.NewDatasetHandler(datasetService, scopeService)
	dashboardHandler := handler.NewDashboardHandler(dashboardService, dashboardRepo, scopeService, dashboardPrefService)
	dashboardPrefHandler := handler.NewDashboardPreferenceHandler(dashboardPrefService)
	templateHandler := handler.NewTemplateHandler(templateService)
	shareHandler := handler.NewShareHandler(shareService, dashboardService, scopeService, drillService)
	pushHandler := handler.NewPushHandler(pushService)
	auditHandler := handler.NewAuditHandler(auditRepo)
	targetHandler := handler.NewTargetHandler(targetService, scopeService)
	datasourceHandler := handler.NewDatasourceHandler(datasourceService)
	drillHandler := handler.NewDrillHandler(drillService)
	wecomHandler := handler.NewWecomHandler(wecomSyncService)
	wecomAuthHandler := handler.NewWecomAuthHandler(wecomAuthService, cfg)

	// 6. 启动定时推送调度器；订阅变更时通过 pushHandler.Reload 热更新
	pushScheduler := scheduler.New(pushRepo, pushService, cfg.Timezone)
	pushHandler.Reload = pushScheduler.Reload
	pushScheduler.Start()
	defer pushScheduler.Stop()

	// 7. 构建路由
	engine := router.New(router.Dependencies{
		Config: cfg, JWT: jwtManager, UserRepo: userRepo, RoleRepo: roleRepo, AuditRepo: auditRepo,
		Auth: authHandler, Org: orgHandler, User: userHandler, Dataset: datasetHandler,
		Dashboard: dashboardHandler, Pref: dashboardPrefHandler, Template: templateHandler, Share: shareHandler,
		Push: pushHandler, Audit: auditHandler, Target: targetHandler,
		Datasource: datasourceHandler, Drill: drillHandler,
		Wecom: wecomHandler, Auth2: wecomAuthHandler,
	})

	server := &http.Server{
		Addr:    cfg.Address(),
		Handler: engine,
		// 超时设置防止慢连接占用资源
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		log.Printf("BI platform API listening on %s", cfg.Address())
		// 非正常关闭错误直接退出进程
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")
	// 优雅关闭：最多等待 10 秒处理完存量请求
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		fmt.Printf("shutdown error: %v\n", err)
	}
}
