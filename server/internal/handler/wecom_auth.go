package handler

import (
	"net/http"
	"net/url"
	"strings"

	"bi-platform/internal/config"
	"bi-platform/internal/middleware"
	"bi-platform/internal/pkg/response"
	"bi-platform/internal/service"

	"github.com/gin-gonic/gin"
)

// WecomAuthHandler 企业微信网页授权登录接口。
type WecomAuthHandler struct {
	service *service.WecomAuthService
	cfg     config.Config
}

// NewWecomAuthHandler 创建企业微信授权登录处理器。
func NewWecomAuthHandler(svc *service.WecomAuthService, cfg config.Config) *WecomAuthHandler {
	return &WecomAuthHandler{service: svc, cfg: cfg}
}

// PublicConfig 登录页获取企业微信登录开关（不暴露 Secret）。
// PublicConfig 登录页获取企业微信登录开关。
func (h *WecomAuthHandler) PublicConfig(c *gin.Context) {
	response.OK(c, h.service.PublicConfig())
}

// Authorize 根据访问环境返回静默授权或扫码登录地址。
// Authorize 返回企业微信授权跳转地址。
func (h *WecomAuthHandler) Authorize(c *gin.Context) {
	target, err := h.service.AuthorizeURL(
		c.Query("mode"), c.Query("redirect"), inWecom(c.Request.UserAgent()), 0,
		"wecom_login", requestBase(c),
	)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"url": target})
}

// Callback 接收企业微信授权回调，成功时 302 到前端回调页。
// Callback 企业微信授权回调，成功后 302 回前端。
func (h *WecomAuthHandler) Callback(c *gin.Context) {
	target, err := h.service.Callback(c.Request.Context(), c.Query("code"), c.Query("state"), requestBase(c))
	if err != nil {
		c.Set("audit_action", "WECOM_LOGIN_FAILED")
		c.Set("audit_username", "wecom")
		redirect := h.service.WebURL(requestBase(c)) + "/login?wecom_error=" + url.QueryEscape(err.Error())
		c.Redirect(http.StatusFound, redirect)
		return
	}
	c.Set("audit_action", "WECOM_LOGIN")
	c.Set("audit_username", "wecom")
	c.Redirect(http.StatusFound, target)
}

// Exchange 使用一次性 ticket 换取正式登录态。
// Exchange 用一次性 ticket 换取正式登录态。
func (h *WecomAuthHandler) Exchange(c *gin.Context) {
	var input struct {
		Ticket string `json:"ticket" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.Set("audit_action", "WECOM_LOGIN_FAILED")
		response.BadRequest(c, "缺少登录凭证")
		return
	}
	result, err := h.service.Exchange(input.Ticket)
	if err != nil {
		c.Set("audit_action", "WECOM_LOGIN_FAILED")
		c.Set("audit_username", "wecom")
		response.Unauthorized(c, err.Error())
		return
	}
	c.Set("audit_action", "WECOM_LOGIN")
	c.Set("audit_username", result.User.Username)
	response.OK(c, result)
}

// BindURL 为已登录用户生成企业微信绑定授权地址。
// BindURL 已登录用户发起企业微信绑定授权。
func (h *WecomAuthHandler) BindURL(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		response.Unauthorized(c, "请先登录")
		return
	}
	target, err := h.service.AuthorizeURL(
		c.DefaultQuery("mode", "auto"), c.DefaultQuery("redirect", "/access"),
		inWecom(c.Request.UserAgent()), user.ID, "wecom_bind", requestBase(c),
	)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"url": target})
}

// Unbind 解除当前账号与企业微信的绑定。
// Unbind 解除企业微信绑定。
func (h *WecomAuthHandler) Unbind(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		response.Unauthorized(c, "请先登录")
		return
	}
	if err := h.service.Unbind(user.ID); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"unbound": true})
}

// inWecom 判断请求是否来自企业微信内置浏览器。
func inWecom(userAgent string) bool {
	return strings.Contains(strings.ToLower(userAgent), "wxwork")
}

// requestBase 推导回调使用的站点地址，优先显式配置，其次按请求推断。
func requestBase(c *gin.Context) string {
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}
	if host == "" {
		return ""
	}
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + host
}
