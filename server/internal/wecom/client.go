// Package wecom 封装企业微信通讯录接口（部门列表 + 成员列表）。
// 仅依赖标准库，便于在无真实凭证时用 Fake Client 做单元测试。
package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL 企业微信开放接口地址。
const DefaultBaseURL = "https://qyapi.weixin.qq.com"

// Department 企业微信部门。
type Department struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	ParentID int64  `json:"parentid"`
	Order    int    `json:"order"`
}

// User 企业微信成员。
type User struct {
	UserID         string  `json:"userid"`
	Name           string  `json:"name"`
	Mobile         string  `json:"mobile"`
	Email          string  `json:"email"`
	Position       string  `json:"position"`
	MainDepartment int64   `json:"main_department"`
	Department     []int64 `json:"department"`
	Status         int     `json:"status"`
}

// Client 通讯录读取接口，服务层只依赖该接口。
type Client interface {
	Departments(ctx context.Context) ([]Department, error)
	// Users 读取指定部门的成员；fetchChild 为 true 时包含所有下级部门成员。
	Users(ctx context.Context, departmentID int64, fetchChild bool) ([]User, error)
}

// AuthClient 网页授权登录所需的接口。
type AuthClient interface {
	UserInfoByCode(ctx context.Context, code string) (*LoginUser, error)
}

// LoginUser 企业微信网页授权返回的登录身份。
type LoginUser struct {
	UserID   string `json:"userid"`
	OpenID   string `json:"openid"`
	DeviceID string `json:"device_id"`
}

// APIError 企业微信业务错误（errcode != 0）。
type APIError struct {
	Code    int
	Message string
}

// Error 实现 error 接口，输出「错误码: 错误信息」。
func (e *APIError) Error() string {
	hint := errorHint(e.Code)
	if hint == "" {
		return fmt.Sprintf("企业微信接口错误(%d): %s", e.Code, e.Message)
	}
	return fmt.Sprintf("企业微信接口错误(%d): %s（%s）", e.Code, e.Message, hint)
}

func errorHint(code int) string {
	switch code {
	case 40001, 40014:
		return "Secret 无效或已过期，请检查企业微信应用 Secret"
	case 40013:
		return "CorpID 不正确"
	case 41001:
		return "缺少 access_token，请检查配置"
	case 42001:
		return "access_token 已过期，稍后重试"
	case 45009:
		return "接口调用频率超限，请稍后重试"
	case 60011:
		return "应用没有通讯录读取权限，请在企微后台为应用授权"
	case 60111:
		return "成员不存在"
	}
	return ""
}

// APIClient 企业微信 HTTP 客户端，内部缓存 access_token。
type APIClient struct {
	CorpID  string
	Secret  string
	BaseURL string
	HTTP    *http.Client

	mu          sync.Mutex
	token       string
	tokenExpire time.Time
}

// NewAPIClient 创建客户端；secret 不应出现在任何日志或错误信息中。
func NewAPIClient(corpID, secret string) *APIClient {
	return &APIClient{
		CorpID:  strings.TrimSpace(corpID),
		Secret:  strings.TrimSpace(secret),
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *APIClient) base() string {
	if strings.TrimSpace(c.BaseURL) == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(c.BaseURL, "/")
}

func (c *APIClient) httpClient() *http.Client {
	if c.HTTP == nil {
		return &http.Client{Timeout: 15 * time.Second}
	}
	return c.HTTP
}

// AccessToken 获取并缓存 access_token，提前 5 分钟刷新。
func (c *APIClient) AccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExpire) {
		return c.token, nil
	}
	query := url.Values{}
	query.Set("corpid", c.CorpID)
	query.Set("corpsecret", c.Secret)
	var resp struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := c.get(ctx, "/cgi-bin/gettoken", query, &resp); err != nil {
		return "", err
	}
	if resp.ErrCode != 0 {
		return "", &APIError{Code: resp.ErrCode, Message: resp.ErrMsg}
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("企业微信未返回 access_token")
	}
	ttl := time.Duration(resp.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	if ttl > 5*time.Minute {
		ttl -= 5 * time.Minute
	}
	c.token = resp.AccessToken
	c.tokenExpire = time.Now().Add(ttl)
	return c.token, nil
}

// Departments 拉取全量部门列表。
func (c *APIClient) Departments(ctx context.Context) ([]Department, error) {
	token, err := c.AccessToken(ctx)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("access_token", token)
	var resp struct {
		ErrCode    int          `json:"errcode"`
		ErrMsg     string       `json:"errmsg"`
		Department []Department `json:"department"`
	}
	if err := c.get(ctx, "/cgi-bin/department/list", query, &resp); err != nil {
		return nil, err
	}
	if resp.ErrCode != 0 {
		return nil, &APIError{Code: resp.ErrCode, Message: resp.ErrMsg}
	}
	return resp.Department, nil
}

// Users 拉取部门成员列表。
func (c *APIClient) Users(ctx context.Context, departmentID int64, fetchChild bool) ([]User, error) {
	token, err := c.AccessToken(ctx)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("access_token", token)
	query.Set("department_id", strconv.FormatInt(departmentID, 10))
	if fetchChild {
		query.Set("fetch_child", "1")
	} else {
		query.Set("fetch_child", "0")
	}
	var resp struct {
		ErrCode  int    `json:"errcode"`
		ErrMsg   string `json:"errmsg"`
		UserList []User `json:"userlist"`
	}
	if err := c.get(ctx, "/cgi-bin/user/list", query, &resp); err != nil {
		return nil, err
	}
	if resp.ErrCode != 0 {
		return nil, &APIError{Code: resp.ErrCode, Message: resp.ErrMsg}
	}
	return resp.UserList, nil
}

// UserInfoByCode 用网页授权 code 换取企业成员身份。
func (c *APIClient) UserInfoByCode(ctx context.Context, code string) (*LoginUser, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("企业微信授权 code 为空")
	}
	token, err := c.AccessToken(ctx)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("access_token", token)
	query.Set("code", code)
	var resp struct {
		ErrCode  int    `json:"errcode"`
		ErrMsg   string `json:"errmsg"`
		UserID   string `json:"userid"`
		OpenID   string `json:"openid"`
		DeviceID string `json:"device_id"`
	}
	if err := c.get(ctx, "/cgi-bin/auth/getuserinfo", query, &resp); err != nil {
		return nil, err
	}
	if resp.ErrCode != 0 {
		return nil, &APIError{Code: resp.ErrCode, Message: resp.ErrMsg}
	}
	if resp.UserID == "" {
		return nil, errors.New("当前企业微信账号不是企业成员，无法登录")
	}
	return &LoginUser{UserID: resp.UserID, OpenID: resp.OpenID, DeviceID: resp.DeviceID}, nil
}

func (c *APIClient) get(ctx context.Context, path string, query url.Values, out any) error {
	endpoint := c.base() + path
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("请求企业微信失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("读取企业微信响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("企业微信接口返回 HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("解析企业微信响应失败: %w", err)
	}
	return nil
}
