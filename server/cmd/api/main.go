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

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	if location, err := time.LoadLocation(cfg.Timezone); err == nil {
		time.Local = location
	}
	db, err := database.Open(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if cfg.SeedDemoData {
		if err := seed.Run(db); err != nil {
			log.Fatalf("初始化演示数据失败: %v", err)
		}
	}

	jwtManager := auth.NewManager(cfg.JWTSecret, cfg.JWTExpire)
	userRepo := repository.NewUserRepository(db)
	roleRepo := repository.NewRoleRepository(db)
	orgRepo := repository.NewOrganizationRepository(db)
	datasetRepo := repository.NewDatasetRepository(db)
	dashboardRepo := repository.NewDashboardRepository(db)
	templateRepo := repository.NewTemplateRepository(db)
	pushRepo := repository.NewPushRepository(db)
	shareRepo := repository.NewShareRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	datasourceRepo := repository.NewDatasourceRepository(db)
	targetRepo := repository.NewTargetRepository(db)
	drillRepo := repository.NewDrillRepository(db)

	scopeService := service.NewScopeService(orgRepo, roleRepo)
	authService := service.NewAuthService(userRepo, roleRepo, jwtManager)
	orgService := service.NewOrganizationService(orgRepo)
	userService := service.NewUserService(userRepo, roleRepo, orgRepo)
	secretBox, err := secretbox.New(cfg.JWTSecret)
	if err != nil {
		log.Fatalf("初始化敏感配置加密失败: %v", err)
	}
	datasourceService := service.NewDatasourceService(datasourceRepo, secretBox, cfg.SQLQueryTimeout)
	defer datasourceService.CloseAll()
	datasetService := service.NewDatasetService(datasetRepo, datasourceService, cfg.SQLQueryTimeout)
	dashboardService := service.NewDashboardService(dashboardRepo, datasetRepo, datasetService)
	templateService := service.NewTemplateService(templateRepo, dashboardRepo)
	shareService := service.NewShareService(shareRepo, dashboardRepo, cfg)
	sender := channel.NewSender(cfg.AllowMockPush)
	pushService := service.NewPushService(pushRepo, userRepo, roleRepo, orgRepo, dashboardRepo, scopeService, shareService, sender)
	targetService := service.NewTargetService(targetRepo, orgRepo)
	drillService := service.NewDrillService(orgRepo, targetRepo, drillRepo, scopeService)

	authHandler := handler.NewAuthHandler(authService)
	orgHandler := handler.NewOrganizationHandler(orgService, scopeService)
	userHandler := handler.NewUserHandler(userService, roleRepo, scopeService)
	datasetHandler := handler.NewDatasetHandler(datasetService, scopeService)
	dashboardHandler := handler.NewDashboardHandler(dashboardService, dashboardRepo, scopeService)
	templateHandler := handler.NewTemplateHandler(templateService)
	shareHandler := handler.NewShareHandler(shareService, dashboardService, scopeService, drillService)
	pushHandler := handler.NewPushHandler(pushService)
	auditHandler := handler.NewAuditHandler(auditRepo)
	targetHandler := handler.NewTargetHandler(targetService, scopeService)
	datasourceHandler := handler.NewDatasourceHandler(datasourceService)
	drillHandler := handler.NewDrillHandler(drillService)

	pushScheduler := scheduler.New(pushRepo, pushService, cfg.Timezone)
	pushHandler.Reload = pushScheduler.Reload
	pushScheduler.Start()
	defer pushScheduler.Stop()

	engine := router.New(router.Dependencies{
		Config: cfg, JWT: jwtManager, UserRepo: userRepo, RoleRepo: roleRepo, AuditRepo: auditRepo,
		Auth: authHandler, Org: orgHandler, User: userHandler, Dataset: datasetHandler,
		Dashboard: dashboardHandler, Template: templateHandler, Share: shareHandler,
		Push: pushHandler, Audit: auditHandler, Target: targetHandler,
		Datasource: datasourceHandler, Drill: drillHandler,
	})

	server := &http.Server{
		Addr:              cfg.Address(),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		log.Printf("BI platform API listening on %s", cfg.Address())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		fmt.Printf("shutdown error: %v\n", err)
	}
}
