// Package wecom implements the subset of 企业微信 APIs needed for OAuth login
// and organization synchronization.
package wecom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"biaoge-bi/internal/config"
)

// Client calls the WeCom (企业微信) API. Access tokens are cached per secret
// until shortly before expiry.
type Client struct {
	cfg  config.WeComConfig
	http *http.Client

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

func NewClient(cfg config.WeComConfig) *Client {
	base := cfg.APIBase
	if base == "" {
		base = "https://qyapi.weixin.qq.com"
	}
	cfg.APIBase = base
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c.cfg.Enabled && c.cfg.CorpID != "" && c.cfg.Secret != ""
}

func (c *Client) CorpID() string   { return c.cfg.CorpID }
func (c *Client) AgentID() int     { return c.cfg.AgentID }
func (c *Client) RedirectURI() string { return c.cfg.RedirectURI }

// AuthorizeURL builds the QR-code login URL that employees scan.
func (c *Client) AuthorizeURL(state, redirectURI string) string {
	if redirectURI == "" {
		redirectURI = c.cfg.RedirectURI
	}
	return fmt.Sprintf(
		"https://open.work.weixin.qq.com/wwopen/sso/qrConnect?appid=%s&agentid=%d&redirect_uri=%s&state=%s",
		url.QueryEscape(c.cfg.CorpID), c.cfg.AgentID, url.QueryEscape(redirectURI), url.QueryEscape(state),
	)
}

// webCredentials is the envelope returned by most WeCom API calls.
type webCredentials struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func check(resp webCredentials) error {
	if resp.ErrCode != 0 {
		return fmt.Errorf("企业微信接口错误 %d: %s", resp.ErrCode, resp.ErrMsg)
	}
	return nil
}

// accessToken returns a cached token, refreshing it when necessary.
func (c *Client) accessToken(ctx context.Context, secret string) (string, error) {
	if secret == "" {
		secret = c.cfg.Secret
	}
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.tokenExpiry) {
		tok := c.token
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()

	endpoint := fmt.Sprintf("%s/cgi-bin/gettoken?corpid=%s&corpsecret=%s",
		c.cfg.APIBase, url.QueryEscape(c.cfg.CorpID), url.QueryEscape(secret))
	var out struct {
		webCredentials
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := c.getJSON(ctx, endpoint, &out); err != nil {
		return "", err
	}
	if err := check(out.webCredentials); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", errors.New("企业微信未返回 access_token")
	}
	c.mu.Lock()
	c.token = out.AccessToken
	// Refresh two minutes before expiry to avoid races.
	c.tokenExpiry = time.Now().Add(time.Duration(out.ExpiresIn-120) * time.Second)
	c.mu.Unlock()
	return out.AccessToken, nil
}

// UserInfo resolves an OAuth code into the employee's userid.
func (c *Client) UserInfo(ctx context.Context, code string) (userID string, err error) {
	token, err := c.accessToken(ctx, "")
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("%s/cgi-bin/auth/getuserinfo?access_token=%s&code=%s",
		c.cfg.APIBase, url.QueryEscape(token), url.QueryEscape(code))
	var out struct {
		webCredentials
		UserID string `json:"userid"`
		// 非企业成员扫码时返回 openid，视为未授权
		OpenID string `json:"openid"`
	}
	if err := c.getJSON(ctx, endpoint, &out); err != nil {
		return "", err
	}
	if err := check(out.webCredentials); err != nil {
		return "", err
	}
	if out.UserID == "" {
		if out.OpenID != "" {
			return "", errors.New("该账号不是企业成员，无法登录")
		}
		return "", errors.New("无法获取企业微信成员信息")
	}
	return out.UserID, nil
}

// Department is one node of the WeCom organization tree.
type Department struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	ParentID int    `json:"parentid"`
	Order    int    `json:"order"`
}

// User is a WeCom contact record.
type User struct {
	UserID     string `json:"userid"`
	Name       string `json:"name"`
	Mobile     string `json:"mobile"`
	Email      string `json:"email"`
	Department []int  `json:"department"`
	Position   string `json:"position"`
	Status     int    `json:"status"`
	Enabled    bool   `json:"enabled"`
}

// ListDepartments returns the full department tree of the corp.
func (c *Client) ListDepartments(ctx context.Context) ([]Department, error) {
	token, err := c.accessToken(ctx, c.contactSecret())
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/cgi-bin/department/list?access_token=%s", c.cfg.APIBase, url.QueryEscape(token))
	var out struct {
		webCredentials
		Department []Department `json:"department"`
	}
	if err := c.getJSON(ctx, endpoint, &out); err != nil {
		return nil, err
	}
	if err := check(out.webCredentials); err != nil {
		return nil, err
	}
	return out.Department, nil
}

// ListUsers returns every member of the corp.
func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	token, err := c.accessToken(ctx, c.contactSecret())
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/cgi-bin/user/list?access_token=%s&department_id=1&fetch_child=1",
		c.cfg.APIBase, url.QueryEscape(token))
	var out struct {
		webCredentials
		UserList []User `json:"userlist"`
	}
	if err := c.getJSON(ctx, endpoint, &out); err != nil {
		return nil, err
	}
	if err := check(out.webCredentials); err != nil {
		return nil, err
	}
	return out.UserList, nil
}

func (c *Client) contactSecret() string {
	if c.cfg.ContactSecret != "" {
		return c.cfg.ContactSecret
	}
	return c.cfg.Secret
}

// SuperAdminIDs reports whether a WeCom userid is configured as a BI admin.
func (c *Client) SuperAdminIDs() map[string]bool {
	out := make(map[string]bool, len(c.cfg.SuperAdminIDs))
	for _, id := range c.cfg.SuperAdminIDs {
		out[id] = true
	}
	return out
}

func (c *Client) getJSON(ctx context.Context, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("请求企业微信失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("企业微信返回状态码 %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("解析企业微信响应失败: %w", err)
	}
	return nil
}

// TestConnection validates credentials by fetching an access token.
func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.accessToken(ctx, c.contactSecret())
	return err
}


// ---------- Outgoing application messages ----------

// SendText sends a plain text application message to the given users.
func (c *Client) SendText(ctx context.Context, toUsers []string, content string) error {
	if !c.Enabled() {
		return errors.New("企业微信未启用")
	}
	if len(toUsers) == 0 {
		return errors.New("收件人不能为空")
	}
	token, err := c.accessToken(ctx, "")
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/cgi-bin/message/send?access_token=%s", c.cfg.APIBase, url.QueryEscape(token))
	body := map[string]any{
		"touser":  strings.Join(toUsers, "|"),
		"msgtype": "text",
		"agentid": c.cfg.AgentID,
		"text":    map[string]string{"content": content},
		"safe":    0,
	}
	return c.postJSON(ctx, endpoint, body)
}

// TemplateCard is the subset of the WeCom textcard message we render.
type TemplateCard struct {
	Title       string
	Description string
	URL         string
	BtnText     string
}

// SendTextCard sends a textcard message (title + description + action button).
func (c *Client) SendTextCard(ctx context.Context, toUsers []string, card TemplateCard) error {
	if !c.Enabled() {
		return errors.New("企业微信未启用")
	}
	if len(toUsers) == 0 {
		return errors.New("收件人不能为空")
	}
	if card.URL == "" {
		return errors.New("卡片消息必须提供跳转链接")
	}
	token, err := c.accessToken(ctx, "")
	if err != nil {
		return err
	}
	btn := card.BtnText
	if btn == "" {
		btn = "查看报表"
	}
	endpoint := fmt.Sprintf("%s/cgi-bin/message/send?access_token=%s", c.cfg.APIBase, url.QueryEscape(token))
	body := map[string]any{
		"touser":  strings.Join(toUsers, "|"),
		"msgtype": "textcard",
		"agentid": c.cfg.AgentID,
		"textcard": map[string]string{
			"title":       card.Title,
			"description": card.Description,
			"url":         card.URL,
			"btntxt":      btn,
		},
		"safe": 0,
	}
	return c.postJSON(ctx, endpoint, body)
}

func (c *Client) postJSON(ctx context.Context, endpoint string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("请求企业微信失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var out struct {
		webCredentials
		InvalidUser string `json:"invaliduser"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("解析企业微信响应失败: %w", err)
	}
	if err := check(out.webCredentials); err != nil {
		return err
	}
	if out.InvalidUser != "" {
		return fmt.Errorf("无效的接收人: %s", out.InvalidUser)
	}
	return nil
}
