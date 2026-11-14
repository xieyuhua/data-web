package handler

import (
	"bi-platform/internal/middleware"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

// AuthHandler 账号密码登录与当前用户信息接口。
type AuthHandler struct {
	auth *service.AuthService
}

// NewAuthHandler 创建认证处理器。
func NewAuthHandler(auth *service.AuthService) *AuthHandler { return &AuthHandler{auth: auth} }

// Login 账号密码登录，成功后返回 JWT 与用户信息。
// 同时写入审计动作（成功 LOGIN / 失败 LOGIN_FAILED）。
func (h *AuthHandler) Login(c *gin.Context) {
	var input struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "请输入用户名和密码")
		return
	}
	c.Set("audit_username", input.Username)
	result, err := h.auth.Login(input.Username, input.Password)
	if err != nil {
		c.Set("audit_action", "LOGIN_FAILED")
		response.Unauthorized(c, err.Error())
		return
	}
	c.Set("audit_action", "LOGIN")
	response.OK(c, result)
}

// Me 返回当前登录用户、角色、权限点与数据范围。
func (h *AuthHandler) Me(c *gin.Context) {
	user := middleware.CurrentUser(c)
	role := middleware.CurrentRole(c)
	response.OK(c, gin.H{
		"user":        user,
		"role":        role,
		"permissions": service.Permissions(role),
		"data_scope":  role.DataScope,
		"is_admin":    service.IsAdmin(role),
	})
}
