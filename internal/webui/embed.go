//go:build !dev

package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var distFS embed.FS

// Register serves the embedded SPA build and falls back to index.html for
// client-side routes.
func Register(r *gin.Engine) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return
	}
	fileServer := http.FileServer(http.FS(sub))
	r.NoRoute(func(c *gin.Context) {
		path := strings.TrimPrefix(c.Request.URL.Path, "/")
		if strings.HasPrefix(path, "api/") {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "接口不存在"})
			return
		}
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err != nil {
			// SPA fallback
			c.Request.URL.Path = "/"
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
