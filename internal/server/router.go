package server

import (
	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/auth"
	"biaoge-bi/internal/config"
	"biaoge-bi/internal/handler"
	"biaoge-bi/internal/service"
	"biaoge-bi/internal/web"
	"biaoge-bi/internal/webui"
)

type Handlers struct {
	Auth       *handler.AuthHandler
	DataSource *handler.DataSourceHandler
	Dataset    *handler.DatasetHandler
	Chart      *handler.ChartHandler
	Dashboard  *handler.DashboardHandler
	Public     *handler.PublicHandler
	Schedule   *handler.ScheduleHandler
	Settings   *handler.SettingsHandler
	Audit      *handler.AuditHandler
	WeCom      *handler.WeComHandler
	Territory  *handler.TerritoryHandler
	Setting    *handler.SettingHandler
	AI         *handler.AIHandler
}

type Deps struct {
	Config    *config.Config
	Auth      *service.AuthService
	Handlers  *Handlers
	LoginRate *auth.Limiter
}

func NewRouter(deps Deps) *gin.Engine {
	if deps.Config.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(Recovery(), RequestID())
	if deps.Config.Server.RequestLogging {
		r.Use(RequestLogger())
	}
	r.Use(CORS(deps.Config.Server.CorsOrigins), BodyLimit(deps.Config.Server.BodyLimitMB))

	api := r.Group("/api")
	api.GET("/health", func(c *gin.Context) { web.OK(c, gin.H{"status": "ok"}) })
	api.POST("/auth/login", deps.Handlers.Auth.Login)
	api.GET("/wecom/status", deps.Handlers.WeCom.PublicStatus)
	api.GET("/wecom/authorize-url", deps.Handlers.WeCom.AuthorizeURL)
	api.POST("/wecom/login", deps.Handlers.WeCom.Login)

	// Public share endpoints (token-authenticated, rate-limited).
	pub := api.Group("/public")
	{
		pub.GET("/dashboards/:token", deps.Handlers.Public.View)
		pub.POST("/dashboards/:token/data", deps.Handlers.Public.Data)
		pub.POST("/dashboards/:token/filters/:filterId/options", deps.Handlers.Public.FilterOptions)
	}

	authed := api.Group("")
	authed.Use(AuthRequired(deps.Auth))
	{
		authed.GET("/auth/me", deps.Handlers.Auth.Me)
		authed.PUT("/auth/password", deps.Handlers.Auth.ChangePassword)

		admin := authed.Group("")
		admin.Use(AdminOnly())
		{
			admin.GET("/users", deps.Handlers.Auth.ListUsers)
			admin.POST("/users", deps.Handlers.Auth.CreateUser)
			admin.GET("/users/:id", deps.Handlers.Auth.GetUser)
			admin.PUT("/users/:id", deps.Handlers.Auth.UpdateUser)
			admin.DELETE("/users/:id", deps.Handlers.Auth.DeleteUser)
			admin.PUT("/users/:id/password", deps.Handlers.Auth.ResetPassword)

			admin.GET("/datasources", deps.Handlers.DataSource.List)
			admin.GET("/datasources/:id", deps.Handlers.DataSource.Get)
			admin.POST("/datasources", deps.Handlers.DataSource.Create)
			admin.PUT("/datasources/:id", deps.Handlers.DataSource.Update)
			admin.DELETE("/datasources/:id", deps.Handlers.DataSource.Delete)
			admin.POST("/datasources/test", deps.Handlers.DataSource.TestUnsaved)
			admin.POST("/datasources/:id/test", deps.Handlers.DataSource.TestByID)

			admin.POST("/datasets", deps.Handlers.Dataset.Create)
			admin.PUT("/datasets/:id", deps.Handlers.Dataset.Update)
			admin.DELETE("/datasets/:id", deps.Handlers.Dataset.Delete)

			admin.GET("/admin/settings", deps.Handlers.Settings.Get)
			admin.GET("/admin/audit-logs", deps.Handlers.Audit.List)
			admin.GET("/admin/config", deps.Handlers.Setting.List)
			admin.PUT("/admin/config", deps.Handlers.Setting.Update)
			admin.POST("/admin/config/reset", deps.Handlers.Setting.Reset)
			admin.POST("/admin/ai/ping", deps.Handlers.AI.Ping)
			admin.POST("/admin/wecom/sync", deps.Handlers.WeCom.Sync)
			admin.GET("/admin/org/export", deps.Handlers.Territory.ExportOrg)
			admin.POST("/admin/org/import", deps.Handlers.Territory.ImportOrg)
			admin.POST("/admin/org/import-report", deps.Handlers.Territory.OrgReport)
			admin.GET("/admin/wecom/status", deps.Handlers.WeCom.Status)
		}

		authed.GET("/departments", deps.Handlers.WeCom.Departments)

		authed.GET("/big-regions", deps.Handlers.Territory.ListBigRegions)
		authed.POST("/big-regions", deps.Handlers.Territory.SaveBigRegion)
		authed.PUT("/big-regions/:id", deps.Handlers.Territory.SaveBigRegion)
		authed.DELETE("/big-regions/:id", deps.Handlers.Territory.DeleteBigRegion)

		authed.GET("/regions", deps.Handlers.Territory.ListRegions)
		authed.POST("/regions", deps.Handlers.Territory.SaveRegion)
		authed.PUT("/regions/:id", deps.Handlers.Territory.SaveRegion)
		authed.DELETE("/regions/:id", deps.Handlers.Territory.DeleteRegion)

		authed.GET("/stores", deps.Handlers.Territory.ListStores)
		authed.GET("/stores/:id", deps.Handlers.Territory.GetStore)
		authed.POST("/stores", deps.Handlers.Territory.SaveStore)
		authed.PUT("/stores/:id", deps.Handlers.Territory.SaveStore)
		authed.DELETE("/stores/:id", deps.Handlers.Territory.DeleteStore)
		authed.GET("/stores/export", deps.Handlers.Territory.ExportStores)
		authed.GET("/stores/template", deps.Handlers.Territory.StoreTemplate)
		authed.POST("/stores/import", deps.Handlers.Territory.ImportStores)
		authed.POST("/stores/import-report", deps.Handlers.Territory.StoreReport)
		authed.GET("/dashboards/:id/dept-grants", deps.Handlers.WeCom.ListDeptGrants)
		authed.POST("/dashboards/:id/dept-grants", deps.Handlers.WeCom.AddDeptGrant)
		authed.DELETE("/dashboards/:id/dept-grants/:grantId", deps.Handlers.WeCom.RemoveDeptGrant)

		authed.GET("/ai/status", deps.Handlers.AI.Status)
		authed.POST("/datasets/test-sql", deps.Handlers.AI.TestSQL)
		authed.POST("/datasets/infer-params", deps.Handlers.AI.InferParams)
		authed.POST("/ai/sql", deps.Handlers.AI.GenerateSQL)
		authed.POST("/ai/complete", deps.Handlers.AI.CompleteSQL)
		authed.POST("/ai/params", deps.Handlers.AI.SuggestParams)
		authed.POST("/ai/metadata", deps.Handlers.AI.SuggestMetadata)

		authed.GET("/datasets", deps.Handlers.Dataset.List)
		authed.GET("/datasets/:id", deps.Handlers.Dataset.Get)
		authed.POST("/datasets/:id/preview", deps.Handlers.Dataset.Preview)
		authed.GET("/datasets/:id/columns", deps.Handlers.Dataset.Columns)

		authed.GET("/charts", deps.Handlers.Chart.List)
		authed.GET("/charts/:id", deps.Handlers.Chart.Get)
		authed.POST("/charts", deps.Handlers.Chart.Create)
		authed.PUT("/charts/:id", deps.Handlers.Chart.Update)
		authed.DELETE("/charts/:id", deps.Handlers.Chart.Delete)
		authed.POST("/charts/:id/duplicate", deps.Handlers.Chart.Duplicate)
		authed.POST("/charts/:id/data", deps.Handlers.Chart.Data)
		authed.POST("/charts/preview-data", deps.Handlers.Chart.PreviewData)
		authed.POST("/charts/:id/export", deps.Handlers.Chart.Export)
		authed.POST("/charts/:id/export-csv", deps.Handlers.Chart.ExportCSV)

		authed.GET("/dashboards", deps.Handlers.Dashboard.List)
		authed.GET("/dashboards/:id", deps.Handlers.Dashboard.Get)
		authed.POST("/dashboards", deps.Handlers.Dashboard.Create)
		authed.PUT("/dashboards/:id", deps.Handlers.Dashboard.Update)
		authed.DELETE("/dashboards/:id", deps.Handlers.Dashboard.Delete)
		authed.POST("/dashboards/:id/duplicate", deps.Handlers.Dashboard.Duplicate)
		authed.PUT("/dashboards/:id/layout", deps.Handlers.Dashboard.SaveLayout)
		authed.PUT("/dashboards/:id/bindings", deps.Handlers.Dashboard.SaveBindings)
		authed.POST("/dashboards/:id/data", deps.Handlers.Dashboard.Data)
		authed.POST("/dashboards/:id/filters/:filterId/options", deps.Handlers.Dashboard.FilterOptions)
		authed.POST("/dashboards/:id/export", deps.Handlers.Dashboard.Export)
		authed.GET("/dashboards/:id/grants", deps.Handlers.Dashboard.ListGrants)
		authed.POST("/dashboards/:id/grants", deps.Handlers.Dashboard.AddGrant)
		authed.DELETE("/dashboards/:id/grants/:grantId", deps.Handlers.Dashboard.RemoveGrant)
		authed.GET("/dashboards/:id/shares", deps.Handlers.Dashboard.ListShares)
		authed.POST("/dashboards/:id/shares", deps.Handlers.Dashboard.CreateShare)
		authed.DELETE("/dashboards/:id/shares/:shareId", deps.Handlers.Dashboard.RevokeShare)

		authed.GET("/schedules", deps.Handlers.Schedule.List)
		authed.POST("/schedules", deps.Handlers.Schedule.Create)
		authed.PUT("/schedules/:id", deps.Handlers.Schedule.Update)
		authed.DELETE("/schedules/:id", deps.Handlers.Schedule.Delete)
		authed.POST("/schedules/:id/run", deps.Handlers.Schedule.RunNow)
		authed.GET("/schedules/:id/runs", deps.Handlers.Schedule.Runs)
		authed.GET("/schedule-runs/:id/download", deps.Handlers.Schedule.DownloadRun)
	}

	webui.Register(r)
	return r
}
