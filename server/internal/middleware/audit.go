package middleware

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"bi-platform/internal/model"
	"bi-platform/internal/repository"

	"github.com/gin-gonic/gin"
)

// shareTokenPattern 用于在审计日志中脱敏分享链接 Token。
var shareTokenPattern = regexp.MustCompile(`(/public/dashboards/)[^/?#]+`)

// Audit 记录所有写操作（POST/PUT/DELETE），以及登录、导出和分享访问等敏感读取，
// 便于追溯关键变更与数据外发行为。
func Audit(repo *repository.AuditRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()

		if repo == nil || !shouldAudit(c) {
			return
		}

		method := c.Request.Method
		var userID uint64
		var username string
		if user := CurrentUser(c); user != nil {
			userID = user.ID
			username = user.Username
		}
		if username == "" {
			username = c.GetString("audit_username")
		}

		action := method
		if custom := c.GetString("audit_action"); custom != "" {
			action = custom
		}

		resource := c.FullPath()
		if resource == "" {
			resource = c.Request.URL.Path
		}
		status := c.Writer.Status()
		path := shareTokenPattern.ReplaceAllString(c.Request.URL.Path, "${1}{token}")
		detail, _ := json.Marshal(map[string]any{
			"method": method,
			"path":   path,
			"query":  c.Request.URL.RawQuery,
			"status": status,
		})

		_ = repo.Create(&model.AuditLog{
			UserID:   userID,
			Username: username,
			Action:   action,
			Resource: resource,
			Method:   method,
			Path:     path,
			Status:   status,
			Duration: time.Since(started).Milliseconds(),
			Detail:   string(detail),
			IP:       c.ClientIP(),
		})
	}
}

// shouldAudit 写操作全量记录；读取只记录登录、导出与分享访问，避免日志被浏览行为淹没。
func shouldAudit(c *gin.Context) bool {
	// 看板浏览上报属于高频只读行为，不写入审计日志，避免淹没真正的变更记录
	if strings.HasSuffix(c.Request.URL.Path, "/view") && strings.Contains(c.Request.URL.Path, "/dashboard-prefs/") {
		return false
	}
	switch c.Request.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	case http.MethodGet, http.MethodHead:
	default:
		return false
	}
	path := c.Request.URL.Path
	if strings.HasSuffix(path, "/auth/login") || strings.HasSuffix(path, "/auth/wecom/callback") {
		return true
	}
	if strings.Contains(path, "/export") {
		return true
	}
	if strings.Contains(path, "/public/dashboards/") {
		return true
	}
	return false
}
