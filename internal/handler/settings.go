package handler

import (
	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/config"
	"biaoge-bi/internal/web"
)

type SettingsHandler struct {
	rt *config.Runtime
}

func NewSettingsHandler(rt *config.Runtime) *SettingsHandler { return &SettingsHandler{rt: rt} }

// Get returns sanitized platform capability flags.
func (h *SettingsHandler) Get(c *gin.Context) {
	cfg := h.rt.Get()
	web.OK(c, gin.H{
		"version":          "1.0.0",
		"smtpEnabled":      cfg.SMTP.Enabled && cfg.SMTP.Host != "",
		"schedulerEnabled": cfg.Scheduler.Enabled,
		"wecomEnabled":     cfg.WeCom.Enabled,
		"queryTimeoutSec":  cfg.DataSource.QueryTimeoutSec,
		"maxResultRows":    cfg.DataSource.MaxResultRows,
	})
}
