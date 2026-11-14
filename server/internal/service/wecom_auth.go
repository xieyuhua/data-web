package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"bi-platform/internal/config"
	"bi-platform/internal/pkg/auth"
	"bi-platform/internal/pkg/secretbox"
	"bi-platform/internal/repository"
	"bi-platform/internal/wecom"

	"gorm.io/gorm"
)

const (
	wecomStateLogin  = "wecom_login"
	wecomStateBind   = "wecom_bind"
	wecomTicketKind  = "wecom_ticket"
	wecomCallbackAPI = "/api/v1/auth/wecom/callback"
	wecomStateTTL    = 10 * time.Minute
	wecomTicketTTL   = 3 * time.Minute
)

// WecomLoginConfigView 登录页可安全获取的企业微信登录配置（不含 Secret）。
type WecomLoginConfigView struct {
	Enabled   bool   `json:"enabled"`
	CorpID    string `json:"corp_id"`
	AgentID   string `json:"agent_id"`
	Mode      string `json:"mode"`
	AutoLogin bool   `json:"auto_login"`
}

// WecomAuthService 企业微信网页授权登录：支持企微客户端内静默授权与浏览器扫码登录。
type WecomAuthService struct {
	repo        *repository.WecomRepository
	userRepo    *repository.UserRepository
	box         *secretbox.Box
	jwt         *auth.Manager
	authService *AuthService
	cfg         config.Config

	newClient func(corpID, secret string) wecom.AuthClient
}

// NewWecomAuthService 创建企业微信授权登录服务。
func NewWecomAuthService(
	repo *repository.WecomRepository,
	userRepo *repository.UserRepository,
	box *secretbox.Box,
	jwt *auth.Manager,
	authService *AuthService,
	cfg config.Config,
) *WecomAuthService {
	return &WecomAuthService{
		repo: repo, userRepo: userRepo, box: box, jwt: jwt, authService: authService, cfg: cfg,
		newClient: func(corpID, secret string) wecom.AuthClient {
			return wecom.NewAPIClient(corpID, secret)
		},
	}
}

// PublicConfig 返回登录页展示所需信息；未启用时不返回企业信息。
func (s *WecomAuthService) PublicConfig() WecomLoginConfigView {
	cfg, err := s.repo.Config()
	if err != nil || !cfg.LoginEnabled {
		return WecomLoginConfigView{Enabled: false}
	}
	return WecomLoginConfigView{
		Enabled: true, CorpID: cfg.CorpID, AgentID: cfg.AgentID,
		Mode: firstNonEmpty(cfg.LoginMode, "auto"), AutoLogin: cfg.AutoLogin,
	}
}

// AuthorizeURL 生成企业微信授权跳转地址。
// mode: auto（企微内静默 / 浏览器扫码）、oauth（静默授权）、qr（扫码登录）。
func (s *WecomAuthService) AuthorizeURL(mode, redirect string, inWecom bool, userID uint64, kind, baseURL string) (string, error) {
	cfg, err := s.repo.Config()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("企业微信登录尚未配置")
		}
		return "", err
	}
	if !cfg.LoginEnabled {
		return "", errors.New("企业微信登录未启用")
	}
	if strings.TrimSpace(cfg.CorpID) == "" {
		return "", errors.New("企业微信 CorpID 未配置")
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "qr" && mode != "oauth" {
		mode = "auto"
	}
	if mode == "auto" {
		if inWecom {
			mode = "oauth"
		} else {
			mode = "qr"
		}
	}
	state, err := s.jwt.SignEphemeral(auth.EphemeralClaims{
		Kind: kind, Redirect: SafeRedirect(redirect), Mode: mode, UserID: userID,
	}, wecomStateTTL)
	if err != nil {
		return "", err
	}
	callback := strings.TrimRight(s.baseURL(baseURL), "/") + wecomCallbackAPI
	params := url.Values{}
	params.Set("appid", cfg.CorpID)
	params.Set("redirect_uri", callback)
	params.Set("state", state)
	if mode == "oauth" {
		params.Set("response_type", "code")
		params.Set("scope", "snsapi_base")
		return "https://open.weixin.qq.com/connect/oauth2/authorize?" + params.Encode() + "#wechat_redirect", nil
	}
	if strings.TrimSpace(cfg.AgentID) == "" {
		return "", errors.New("扫码登录需要在企业微信配置中填写应用 AgentID")
	}
	params.Set("login_type", "CorpApp")
	params.Set("agentid", cfg.AgentID)
	return "https://login.work.weixin.qq.com/wwlogin/sso/login?" + params.Encode(), nil
}

// Callback 处理企业微信回调：换取成员身份 → 签发一次性 ticket / 完成绑定。
// 返回需要跳转的前端地址。
func (s *WecomAuthService) Callback(ctx context.Context, code, state, baseURL string) (string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", errors.New("缺少企业微信授权 code")
	}
	claims, err := s.jwt.ParseEphemeral(strings.TrimSpace(state))
	if err != nil {
		return "", errors.New("授权状态已失效，请重新发起登录")
	}
	if claims.Kind != wecomStateLogin && claims.Kind != wecomStateBind {
		return "", errors.New("授权状态不合法")
	}
	cfg, err := s.repo.Config()
	if err != nil {
		return "", errors.New("企业微信配置不存在")
	}
	if !cfg.LoginEnabled {
		return "", errors.New("企业微信登录未启用")
	}
	secret, err := s.box.Decrypt(cfg.SecretEnc)
	if err != nil {
		return "", err
	}
	if cfg.CorpID == "" || secret == "" {
		return "", errors.New("企业微信 CorpID / Secret 配置不完整")
	}
	client := s.newClient(cfg.CorpID, secret)
	loginUser, err := client.UserInfoByCode(ctx, code)
	if err != nil {
		return "", err
	}
	web := strings.TrimRight(s.webBase(baseURL), "/")
	if claims.Kind == wecomStateBind {
		if err := s.bind(claims.UserID, loginUser.UserID); err != nil {
			return "", err
		}
		return web + "/wecom/callback?bound=1", nil
	}
	user, err := s.userRepo.FindByWecomUserID(loginUser.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("企业微信账号未绑定平台账号，请联系管理员在「用户与角色」中填写你的企业微信 UserID")
		}
		return "", err
	}
	ticket, err := s.jwt.SignEphemeral(auth.EphemeralClaims{Kind: wecomTicketKind, UserID: user.ID}, wecomTicketTTL)
	if err != nil {
		return "", err
	}
	return web + "/wecom/callback?ticket=" + url.QueryEscape(ticket) +
		"&redirect=" + url.QueryEscape(SafeRedirect(claims.Redirect)), nil
}

// Exchange 用一次性 ticket 换取正式登录态，避免长期令牌出现在 URL 中。
func (s *WecomAuthService) Exchange(ticket string) (*LoginResult, error) {
	claims, err := s.jwt.ParseEphemeral(strings.TrimSpace(ticket))
	if err != nil || claims.Kind != wecomTicketKind {
		return nil, errors.New("登录凭证已失效，请重新登录")
	}
	return s.authService.LoginByUserID(claims.UserID)
}

func (s *WecomAuthService) bind(userID uint64, wecomUserID string) error {
	if userID == 0 {
		return errors.New("绑定信息不完整，请重新发起")
	}
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return errors.New("账号不存在")
	}
	existing, err := s.userRepo.FindByWecomUserID(wecomUserID)
	if err == nil && existing.ID != userID {
		return errors.New("该企业微信账号已绑定其他平台账号")
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	user.WecomUserID = wecomUserID
	return s.userRepo.Update(user)
}

// Unbind 解除当前账号与企业微信的绑定。
func (s *WecomAuthService) Unbind(userID uint64) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return errors.New("账号不存在")
	}
	if user.WecomUserID == "" {
		return errors.New("当前账号未绑定企业微信")
	}
	user.WecomUserID = ""
	return s.userRepo.Update(user)
}

func (s *WecomAuthService) baseURL(requestBase string) string {
	if value := strings.TrimRight(strings.TrimSpace(s.cfg.WebBaseURL), "/"); value != "" {
		return value
	}
	if value := strings.TrimRight(strings.TrimSpace(requestBase), "/"); value != "" {
		return value
	}
	if value := strings.TrimRight(strings.TrimSpace(s.cfg.APIBaseURL), "/"); value != "" {
		return value
	}
	return "http://localhost:" + s.cfg.Port
}

func (s *WecomAuthService) webBase(requestBase string) string {
	if value := strings.TrimRight(strings.TrimSpace(s.cfg.WebBaseURL), "/"); value != "" {
		return value
	}
	return strings.TrimRight(strings.TrimSpace(requestBase), "/")
}

// WebURL 供回调失败时跳转到前端登录页。
func (s *WecomAuthService) WebURL(requestBase string) string {
	base := s.webBase(requestBase)
	if base == "" {
		base = "http://localhost:" + s.cfg.Port
	}
	return base
}

// SafeRedirect 只允许站内相对路径，避免开放重定向。
func SafeRedirect(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.Contains(value, "\\") {
		return "/dashboards"
	}
	if strings.Contains(value, "://") {
		return "/dashboards"
	}
	return value
}
