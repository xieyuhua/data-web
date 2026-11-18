package server

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"biaoge-bi/internal/service"
	"biaoge-bi/internal/web"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if id == "" {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Header("X-Request-Id", id)
		c.Next()
	}
}

func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
			"request_id", c.GetString("request_id"),
		)
	}
}

func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, err any) {
		slog.Error("panic recovered", "error", err, "path", c.Request.URL.Path)
		web.Fail(c, http.StatusInternalServerError, "服务器内部错误")
	})
}

// AuthRequired validates the bearer token and loads a live user.
func AuthRequired(authSvc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			web.Unauthorized(c, "请先登录")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		_, user, err := authSvc.ParseToken(token)
		if err != nil {
			web.Unauthorized(c, err.Error())
			return
		}
		web.SetUser(c, user)
		c.Next()
	}
}

func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := web.CurrentUser(c)
		if u == nil || !u.IsAdmin() {
			web.Forbidden(c, "需要管理员权限")
			return
		}
		c.Next()
	}
}

func CORS(origins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[strings.TrimRight(o, "/")] = true
	}
	return func(c *gin.Context) {
		origin := strings.TrimRight(c.GetHeader("Origin"), "/")
		if origin != "" && allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "false")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Expose-Headers", "Content-Disposition, X-Request-Id")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func BodyLimit(mb int) gin.HandlerFunc {
	if mb <= 0 {
		mb = 5
	}
	return func(c *gin.Context) {
		if c.Request.ContentLength > int64(mb)<<20 {
		web.Fail(c, http.StatusRequestEntityTooLarge, "请求体过大")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(mb)<<20)
		c.Next()
	}
}
