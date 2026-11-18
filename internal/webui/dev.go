//go:build dev

package webui

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Register in dev mode does nothing: the Vite dev server hosts the UI and
// proxies /api to this process.
func Register(r *gin.Engine) {
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "开发模式下请访问 Vite 开发服务器 http://localhost:5173",
		})
	})
}
