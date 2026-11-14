package middleware

import (
	"strings"

	"bi-platform/internal/model"
	"bi-platform/internal/pkg/auth"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/repository"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	userKey = "bi.current_user"
	roleKey = "bi.current_role"
)

func Auth(jwtManager *auth.Manager, userRepo *repository.UserRepository, roleRepo *repository.RoleRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			response.Unauthorized(c, "请先登录")
			return
		}
		claims, err := jwtManager.Parse(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil {
			response.Unauthorized(c, "登录凭证已失效，请重新登录")
			return
		}
		user, err := userRepo.FindByID(claims.UserID)
		if err != nil || user.Status != "active" {
			response.Unauthorized(c, "账号不存在或已停用")
			return
		}
		role, err := roleRepo.FindByCode(user.RoleCode)
		if err != nil {
			response.Forbidden(c, "角色配置异常")
			return
		}
		c.Set(userKey, user)
		c.Set(roleKey, role)
		c.Next()
	}
}

func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := CurrentRole(c)
		if !service.HasPermission(role, permission) {
			response.Forbidden(c, "当前账号没有此操作权限")
			return
		}
		c.Next()
	}
}

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !service.IsAdmin(CurrentRole(c)) {
			response.Forbidden(c, "仅平台管理员可执行此操作")
			return
		}
		c.Next()
	}
}

func CurrentUser(c *gin.Context) *model.User {
	value, ok := c.Get(userKey)
	if !ok {
		return nil
	}
	user, _ := value.(*model.User)
	return user
}

func CurrentRole(c *gin.Context) *model.Role {
	value, ok := c.Get(roleKey)
	if !ok {
		return nil
	}
	role, _ := value.(*model.Role)
	return role
}
