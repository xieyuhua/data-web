package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bi-platform/internal/config"
	"bi-platform/internal/handler"
	"bi-platform/internal/middleware"
	"bi-platform/internal/pkg/auth"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/repository"

	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	Config     config.Config
	JWT        *auth.Manager
	UserRepo   *repository.UserRepository
	RoleRepo   *repository.RoleRepository
	AuditRepo  *repository.AuditRepository
	Auth       *handler.AuthHandler
	Org        *handler.OrganizationHandler
	User       *handler.UserHandler
	Dataset    *handler.DatasetHandler
	Dashboard  *handler.DashboardHandler
	Template   *handler.TemplateHandler
	Share      *handler.ShareHandler
	Push       *handler.PushHandler
	Audit      *handler.AuditHandler
	Target     *handler.TargetHandler
	Datasource *handler.DatasourceHandler
	Drill      *handler.DrillHandler
}

func New(deps Dependencies) *gin.Engine {
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), securityHeaders(), cors(deps.Config.CORSOrigins), middleware.Audit(deps.AuditRepo))
	r.MaxMultipartMemory = 12 << 20

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().Format(time.RFC3339)})
	})

	api := r.Group("/api/v1")
	api.POST("/auth/login", deps.Auth.Login)
	api.GET("/public/dashboards/:token", deps.Share.Public)
	api.GET("/public/dashboards/:token/drill", deps.Share.PublicDrill)
	api.GET("/public/dashboards/:token/drill/trend", deps.Share.PublicDrillTrend)

	secured := api.Group("")
	secured.Use(middleware.Auth(deps.JWT, deps.UserRepo, deps.RoleRepo))
	{
		secured.GET("/auth/me", deps.Auth.Me)
		secured.GET("/orgs/tree", deps.Org.Tree)
		secured.GET("/roles", deps.User.Roles)

		secured.GET("/users", deps.User.List)
		secured.POST("/users", middleware.RequireAdmin(), deps.User.Create)
		secured.PUT("/users/:id", middleware.RequireAdmin(), deps.User.Update)
		secured.DELETE("/users/:id", middleware.RequireAdmin(), deps.User.Delete)
		secured.GET("/users/export", middleware.RequireAdmin(), deps.User.Export)
		secured.GET("/users/template", middleware.RequireAdmin(), deps.User.Template)
		secured.POST("/users/import", middleware.RequireAdmin(), deps.User.Import)

		secured.GET("/drill", deps.Drill.Query)
		secured.GET("/drill/trend", deps.Drill.Trend)
		secured.GET("/targets", deps.Target.List)
		secured.GET("/targets/achievement", deps.Target.Achievement)
		secured.GET("/targets/periods", deps.Target.Periods)
		secured.GET("/targets/metrics", deps.Target.Metrics)
		secured.POST("/targets", middleware.RequireAdmin(), deps.Target.Create)
		secured.PUT("/targets/:id", middleware.RequireAdmin(), deps.Target.Update)
		secured.DELETE("/targets/:id", middleware.RequireAdmin(), deps.Target.Delete)
		secured.GET("/targets/export", middleware.RequireAdmin(), deps.Target.Export)
		secured.GET("/targets/template", middleware.RequireAdmin(), deps.Target.Template)
		secured.POST("/targets/import", middleware.RequireAdmin(), deps.Target.Import)

		secured.GET("/datasets", deps.Dataset.List)
		secured.GET("/datasets/:id/preview", middleware.RequirePermission("dashboard:create"), deps.Dataset.Preview)
		secured.POST("/datasets/preview", middleware.RequirePermission("dashboard:create"), deps.Dataset.PreviewDraft)
		secured.POST("/datasets", middleware.RequireAdmin(), deps.Dataset.Create)
		secured.PUT("/datasets/:id", middleware.RequireAdmin(), deps.Dataset.Update)
		secured.DELETE("/datasets/:id", middleware.RequireAdmin(), deps.Dataset.Delete)
		secured.POST("/datasets/:id/to-dashboard", middleware.RequirePermission("dashboard:create"), deps.Dashboard.ToDashboard)

		secured.GET("/dashboards", deps.Dashboard.List)
		secured.GET("/dashboards/:id", deps.Dashboard.Get)
		secured.GET("/dashboards/:id/data", deps.Dashboard.Data)
		secured.GET("/dashboards/:id/export", deps.Dashboard.Export)
		secured.POST("/dashboards", middleware.RequirePermission("dashboard:create"), deps.Dashboard.Create)
		secured.PUT("/dashboards/:id", middleware.RequirePermission("dashboard:create"), deps.Dashboard.Update)
		secured.DELETE("/dashboards/:id", middleware.RequirePermission("dashboard:create"), deps.Dashboard.Delete)
		secured.POST("/dashboards/:id/duplicate", middleware.RequirePermission("dashboard:create"), deps.Dashboard.Duplicate)

		secured.GET("/templates", deps.Template.List)
		secured.POST("/templates", middleware.RequireAdmin(), deps.Template.Create)
		secured.DELETE("/templates/:id", middleware.RequireAdmin(), deps.Template.Delete)
		secured.POST("/templates/:id/use", middleware.RequirePermission("dashboard:create"), deps.Template.Use)

		secured.GET("/shares", deps.Share.List)
		secured.POST("/shares", middleware.RequirePermission("share:create"), deps.Share.Create)
		secured.DELETE("/shares/:id", deps.Share.Revoke)

		admin := secured.Group("/admin")
		admin.Use(middleware.RequireAdmin())
		{
			admin.POST("/orgs", deps.Org.Create)
			admin.PUT("/orgs/:id", deps.Org.Update)
			admin.DELETE("/orgs/:id", deps.Org.Delete)
			admin.GET("/orgs/export", deps.Org.Export)
			admin.POST("/orgs/import", deps.Org.Import)

			admin.GET("/push/templates", deps.Push.Templates)
			admin.POST("/push/templates", deps.Push.CreateTemplate)
			admin.PUT("/push/templates/:id", deps.Push.UpdateTemplate)
			admin.DELETE("/push/templates/:id", deps.Push.DeleteTemplate)
			admin.GET("/push/subscriptions", deps.Push.Subscriptions)
			admin.POST("/push/subscriptions", deps.Push.CreateSubscription)
			admin.PUT("/push/subscriptions/:id", deps.Push.UpdateSubscription)
			admin.DELETE("/push/subscriptions/:id", deps.Push.DeleteSubscription)
			admin.POST("/push/subscriptions/:id/send", deps.Push.SendSubscription)
			admin.GET("/push/records", deps.Push.Records)
			admin.POST("/push/records/:id/retry", deps.Push.RetryRecord)
			admin.GET("/push/channels", deps.Push.ChannelConfigs)
			admin.PUT("/push/channels/:channel", deps.Push.SaveChannelConfig)
			admin.GET("/push/preview", deps.Push.Preview)
			admin.GET("/audit-logs", deps.Audit.List)

			admin.GET("/datasources", deps.Datasource.List)
			admin.POST("/datasources", deps.Datasource.Create)
			admin.PUT("/datasources/:id", deps.Datasource.Update)
			admin.DELETE("/datasources/:id", deps.Datasource.Delete)
			admin.POST("/datasources/:id/test", deps.Datasource.Test)
		}
	}

	if staticDir := env("WEB_DIST_DIR", "../web/dist"); dirExists(staticDir) {
		r.Static("/assets", filepath.Join(staticDir, "assets"))
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				response.NotFound(c, "接口不存在")
				return
			}
			c.File(filepath.Join(staticDir, "index.html"))
		})
	}

	return r
}

func cors(origins string) gin.HandlerFunc {
	allowed := make(map[string]bool)
	for _, origin := range strings.Split(origins, ",") {
		allowed[strings.TrimSpace(origin)] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && (allowed[origin] || allowed["*"]) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Requested-With")
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			c.Header("Access-Control-Max-Age", "86400")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
