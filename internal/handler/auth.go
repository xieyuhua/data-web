package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/auth"
	"biaoge-bi/internal/web"
	"biaoge-bi/internal/service"
)

type AuthHandler struct {
	auth      *service.AuthService
	users     *service.UserService
	loginRate *auth.Limiter
	audit     *service.AuditService
}

func NewAuthHandler(authSvc *service.AuthService, users *service.UserService, loginRate *auth.Limiter, audit *service.AuditService) *AuthHandler {
	return &AuthHandler{auth: authSvc, users: users, loginRate: loginRate, audit: audit}
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		web.BadRequest(c, "请输入用户名和密码")
		return
	}
	key := c.ClientIP() + "|" + req.Username
	if !h.loginRate.Allow(key) {
		web.Fail(c, http.StatusTooManyRequests, "尝试过于频繁，请稍后再试")
		return
	}
	res, err := h.auth.Login(req.Username, req.Password)
	if err != nil {
		h.audit.Log(c, "login_failed", "user", 0, gin.H{"username": req.Username})
		web.Unauthorized(c, err.Error())
		return
	}
	h.audit.Log(c, "login", "user", res.User.ID, nil)
	web.OK(c, res)
}

func (h *AuthHandler) Me(c *gin.Context) {
	u := web.CurrentUser(c)
	if u == nil {
		web.Unauthorized(c, "请先登录")
		return
	}
	web.OK(c, u)
}

type changePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required"`
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	u := web.CurrentUser(c)
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	if err := h.auth.ChangePassword(u.ID, req.OldPassword, req.NewPassword); err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	web.OK(c, gin.H{"message": "密码已更新"})
}

type createUserRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	Role        string `json:"role"`
}

func (h *AuthHandler) CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	u, err := h.users.Create(service.CreateUserInput{
		Username: req.Username, Password: req.Password, DisplayName: req.DisplayName,
		Email: req.Email, Role: req.Role,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	web.OK(c, u)
}

func (h *AuthHandler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	users, total, err := h.users.List(c.Query("keyword"), page, pageSize)
	if err != nil {
		web.ServerError(c, "查询用户失败")
		return
	}
	web.Page(c, users, total, page, pageSize)
}

func (h *AuthHandler) GetUser(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	u, err := h.users.Get(id)
	if err != nil {
		web.NotFound(c, "用户不存在")
		return
	}
	web.OK(c, u)
}

type updateUserRequest struct {
	DisplayName *string `json:"displayName"`
	Email       *string `json:"email"`
	Role        *string `json:"role"`
	Enabled     *bool   `json:"enabled"`
}

func (h *AuthHandler) UpdateUser(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	cur := web.CurrentUser(c)
	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	if cur.ID == id && req.Enabled != nil && !*req.Enabled {
		web.BadRequest(c, "不能禁用当前登录账号")
		return
	}
	if cur.ID == id && req.Role != nil && *req.Role != "admin" {
		web.BadRequest(c, "不能降级当前登录的管理员账号")
		return
	}
	u, err := h.users.Update(id, service.UpdateUserInput{
		DisplayName: req.DisplayName, Email: req.Email, Role: req.Role, Enabled: req.Enabled,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	web.OK(c, u)
}

type resetPasswordRequest struct {
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	if err := h.users.ResetPassword(id, req.Password); err != nil {
		writeServiceError(c, err)
		return
	}
	h.audit.Log(c, "user_password_reset", "user", id, nil)
	web.OK(c, gin.H{"message": "密码已重置"})
}

func (h *AuthHandler) DeleteUser(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	cur := web.CurrentUser(c)
	if cur.ID == id {
		web.BadRequest(c, "不能删除当前登录账号")
		return
	}
	if err := h.users.Delete(id); err != nil {
		web.ServerError(c, "删除用户失败")
		return
	}
	web.OK(c, gin.H{"message": "已删除"})
}

func parseID(c *gin.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		web.BadRequest(c, "无效的 ID")
		return 0, errors.New("invalid id")
	}
	return uint(id), nil
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrDuplicateUsername),
		errors.Is(err, service.ErrWeakPassword),
		errors.Is(err, service.ErrInvalidCredentials):
		web.BadRequest(c, err.Error())
	default:
		if err.Error() != "" && len(err.Error()) < 300 {
			web.BadRequest(c, err.Error())
			return
		}
		web.ServerError(c, "操作失败")
	}
}
