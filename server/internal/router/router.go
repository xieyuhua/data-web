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

// Dependencies 路由所需的全部依赖，由 main 统一注入。
// 采用显式依赖注入，便于测试与替换实现。
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
	Pref       *handler.DashboardPreferenceHandler
	Template   *handler.TemplateHandler
	Share      *handler.ShareHandler
	Push       *handler.PushHandler
	Audit      *handler.AuditHandler
	Target     *handler.TargetHandler
	Datasource *handler.DatasourceHandler
	Drill      *handler.DrillHandler
	Wecom      *handler.WecomHandler
	Auth2      *handler.WecomAuthHandler
}

// New 构建 Gin 路由。
// 路由分为三部分：公开接口、登录后接口、管理员接口，并统一挂载鉴权、审计、
// CORS 与安全响应头中间件；构建完成后由后端直接托管前端静态资源。
func New(deps Dependencies) *gin.Engine {
	if os.Getenv("GIN_MODE") == "" {
		// 默认使用发布模式，减少调试输出
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	// 全局中间件：访问日志、panic 恢复、安全头、CORS、审计
	r.Use(gin.Logger(), gin.Recovery(), securityHeaders(), cors(deps.Config.CORSOrigins), middleware.Audit(deps.AuditRepo))
	// 限制上传文件大小为 12MB（通讯录/任务导入）
	r.MaxMultipartMemory = 12 << 20

	// 健康检查接口，供部署探活使用
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().Format(time.RFC3339)})
	})

	api := r.Group("/api/v1")
	// ---------- 公开接口：登录、企业微信授权、分享看板 ----------
	api.POST("/auth/login", deps.Auth.Login)
	api.GET("/auth/wecom/config", deps.Auth2.PublicConfig)
	api.GET("/auth/wecom/authorize", deps.Auth2.Authorize)
	api.GET("/auth/wecom/callback", deps.Auth2.Callback)
	api.POST("/auth/wecom/exchange", deps.Auth2.Exchange)
	api.GET("/public/dashboards/:token", deps.Share.Public)
	api.GET("/public/dashboards/:token/drill", deps.Share.PublicDrill)
	api.GET("/public/dashboards/:token/drill/trend", deps.Share.PublicDrillTrend)
	api.GET("/public/dashboards/:token/widgets/:widgetId/trend", deps.Share.PublicWidgetTrend)

	// ---------- 登录后可访问的接口 ----------
	secured := api.Group("")
	secured.Use(middleware.Auth(deps.JWT, deps.UserRepo, deps.RoleRepo))
	{
		// 当前用户信息、组织树、角色列表
		secured.GET("/auth/me", deps.Auth.Me)
		secured.GET("/orgs/tree", deps.Org.Tree)
		secured.GET("/roles", deps.User.Roles)

		// 企业微信账号绑定
		secured.GET("/auth/wecom/bind-url", deps.Auth2.BindURL)
		secured.DELETE("/auth/wecom/bind", deps.Auth2.Unbind)

		// 用户管理（写操作仅管理员）
		secured.GET("/users", deps.User.List)
		secured.POST("/users", middleware.RequireAdmin(), deps.User.Create)
		secured.PUT("/users/:id", middleware.RequireAdmin(), deps.User.Update)
		secured.DELETE("/users/:id", middleware.RequireAdmin(), deps.User.Delete)
		secured.GET("/users/export", middleware.RequireAdmin(), deps.User.Export)
		secured.GET("/users/template", middleware.RequireAdmin(), deps.User.Template)
		secured.POST("/users/import", middleware.RequireAdmin(), deps.User.Import)

		// 组织下钻与趋势查询
		secured.GET("/drill", deps.Drill.Query)
		secured.GET("/drill/trend", deps.Drill.Trend)
		// 目标任务：查看所有人可见，维护仅管理员
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

		// 数据集：预览需要建看板权限，维护仅管理员
		secured.GET("/datasets", deps.Dataset.List)
		secured.GET("/datasets/:id/preview", middleware.RequirePermission("dashboard:create"), deps.Dataset.Preview)
		secured.POST("/datasets/preview", middleware.RequirePermission("dashboard:create"), deps.Dataset.PreviewDraft)
		secured.POST("/datasets", middleware.RequireAdmin(), deps.Dataset.Create)
		secured.PUT("/datasets/:id", middleware.RequireAdmin(), deps.Dataset.Update)
		secured.DELETE("/datasets/:id", middleware.RequireAdmin(), deps.Dataset.Delete)
		secured.POST("/datasets/:id/to-dashboard", middleware.RequirePermission("dashboard:create"), deps.Dashboard.ToDashboard)

		// 看板：查看按数据权限过滤，维护需要建看板权限
		secured.GET("/dashboards", deps.Dashboard.List)
		secured.GET("/dashboards/:id", deps.Dashboard.Get)
		secured.GET("/dashboards/:id/data", deps.Dashboard.Data)
		// 单组件指标趋势：默认最近 7 天，时间区间可自定义
		secured.GET("/dashboards/:id/widgets/:widgetId/trend", deps.Dashboard.WidgetTrend)
		secured.GET("/dashboards/:id/export", deps.Dashboard.Export)
		secured.POST("/dashboards", middleware.RequirePermission("dashboard:create"), deps.Dashboard.Create)
		secured.PUT("/dashboards/:id", middleware.RequirePermission("dashboard:create"), deps.Dashboard.Update)
		secured.DELETE("/dashboards/:id", middleware.RequirePermission("dashboard:create"), deps.Dashboard.Delete)
		secured.POST("/dashboards/:id/duplicate", middleware.RequirePermission("dashboard:create"), deps.Dashboard.Duplicate)

		// 看板个人偏好：收藏 / 置顶 / 最近浏览（仅影响当前账号自己的首页排序）
		secured.GET("/dashboard-prefs", deps.Pref.List)
		secured.PUT("/dashboard-prefs/:id", deps.Pref.Update)
		secured.POST("/dashboard-prefs/:id/view", deps.Pref.Touch)

		// 模板
		secured.GET("/templates", deps.Template.List)
		secured.POST("/templates", middleware.RequireAdmin(), deps.Template.Create)
		secured.DELETE("/templates/:id", middleware.RequireAdmin(), deps.Template.Delete)
		secured.POST("/templates/:id/use", middleware.RequirePermission("dashboard:create"), deps.Template.Use)

		// 分享链接
		secured.GET("/shares", deps.Share.List)
		secured.POST("/shares", middleware.RequirePermission("share:create"), deps.Share.Create)
		secured.DELETE("/shares/:id", deps.Share.Revoke)

		// ---------- 管理员接口 ----------
		admin := secured.Group("/admin")
		admin.Use(middleware.RequireAdmin())
		{
			// 组织架构维护与通讯录导入导出
			admin.POST("/orgs", deps.Org.Create)
			admin.PUT("/orgs/:id", deps.Org.Update)
			admin.DELETE("/orgs/:id", deps.Org.Delete)
			admin.GET("/orgs/export", deps.Org.Export)
			admin.POST("/orgs/import", deps.Org.Import)

			// 推送模板、订阅、记录与渠道配置
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
			// 审计日志
			admin.GET("/audit-logs", deps.Audit.List)

			// 外部数据源管理
			admin.GET("/datasources", deps.Datasource.List)
			admin.POST("/datasources", deps.Datasource.Create)
			admin.PUT("/datasources/:id", deps.Datasource.Update)
			admin.DELETE("/datasources/:id", deps.Datasource.Delete)
			admin.POST("/datasources/:id/test", deps.Datasource.Test)

			// 企业微信配置、连接测试与异步同步任务
			admin.GET("/wecom/config", deps.Wecom.Config)
			admin.PUT("/wecom/config", deps.Wecom.SaveConfig)
			admin.POST("/wecom/test", deps.Wecom.Test)
			admin.POST("/wecom/sync", deps.Wecom.Sync)
			admin.GET("/wecom/jobs", deps.Wecom.Jobs)
			admin.GET("/wecom/jobs/:id", deps.Wecom.Job)
		}
	}

	// 前端静态资源托管：/assets 走静态文件，其余路径回退到 index.html（前端路由）
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

// cors 跨域中间件：仅放行配置白名单中的来源。
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

// securityHeaders 设置基础安全响应头，降低 XSS、点击劫持风险。
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

// dirExists 判断目录是否存在。
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// env 读取环境变量，为空时使用默认值。
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
