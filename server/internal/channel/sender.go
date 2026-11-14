package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"bi-platform/internal/model"
)

// Recipient 推送接收人：携带企业微信 UserID 或微信 OpenID。
type Recipient struct {
	UserID       uint64 `json:"user_id"`
	Name         string `json:"name"`
	WecomUserID  string `json:"wecom_user_id"`
	WechatOpenID string `json:"wechat_open_id"`
}

// Payload 推送内容：标题、正文、跳转链接与卡片样式。
type Payload struct {
	TemplateChannel string `json:"template_channel"`
	Title           string `json:"title"`
	Content         string `json:"content"`
	URL             string `json:"url"`
	ButtonText      string `json:"button_text"`
	AccentColor     string `json:"accent_color"`
}

// Result 单次发送结果：sent / simulated / skipped / failed。
type Result struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Sender 推送发送器，负责调用企业微信 / 微信接口并缓存 access_token。
type Sender struct {
	client    *http.Client
	allowMock bool
	mu        sync.Mutex
	tokens    map[string]cachedToken
}

// cachedToken access_token 内存缓存。
type cachedToken struct {
	Value     string
	ExpiresAt time.Time
}

// NewSender 创建发送器；allowMock 为 true 时未配置凭证也返回模拟成功。
func NewSender(allowMock bool) *Sender {
	return &Sender{
		client:    &http.Client{Timeout: 12 * time.Second},
		allowMock: allowMock,
		tokens:    make(map[string]cachedToken),
	}
}

// WeComConfig 企业微信应用配置（AgentID 用于应用消息，WebhookURL 用于群机器人）。
type WeComConfig struct {
	CorpID     string `json:"corp_id"`
	AgentID    int    `json:"agent_id"`
	Secret     string `json:"secret"`
	WebhookURL string `json:"webhook_url"`
	DryRun     bool   `json:"dry_run"`
}

// WeChatConfig 微信公众号模板消息配置。
type WeChatConfig struct {
	AppID      string `json:"app_id"`
	Secret     string `json:"secret"`
	TemplateID string `json:"template_id"`
	DryRun     bool   `json:"dry_run"`
}

// Send 按模板渠道分发到企业微信或微信发送器。
func (s *Sender) Send(ctx context.Context, config model.ChannelConfig, recipient Recipient, payload Payload) Result {
	if payload.TemplateChannel == "wecom_text" || payload.TemplateChannel == "wecom_textcard" {
		return s.sendWeCom(ctx, config, recipient, payload)
	}
	if payload.TemplateChannel == "wechat" {
		return s.sendWeChat(ctx, config, recipient, payload)
	}
	return Result{Status: "failed", Message: "不支持的推送通道类型"}
}

func (s *Sender) sendWeCom(ctx context.Context, config model.ChannelConfig, recipient Recipient, payload Payload) Result {
	var cfg WeComConfig
	if err := json.Unmarshal([]byte(config.ConfigJSON), &cfg); err != nil {
		return Result{Status: "failed", Message: "企业微信配置格式错误"}
	}
	if !config.Enabled || cfg.DryRun {
		if s.allowMock {
			return Result{Status: "simulated", Message: "企业微信未启用或处于演练模式，已记录模拟推送"}
		}
		return Result{Status: "failed", Message: "企业微信通道未启用"}
	}
	if cfg.WebhookURL != "" && (cfg.CorpID == "" || cfg.Secret == "") {
		return s.sendWeComWebhook(ctx, cfg.WebhookURL, recipient, payload)
	}
	if cfg.CorpID == "" || cfg.Secret == "" || cfg.AgentID == 0 {
		if s.allowMock {
			return Result{Status: "simulated", Message: "企业微信凭证未配置，已记录模拟推送"}
		}
		return Result{Status: "failed", Message: "企业微信凭证未配置"}
	}
	if recipient.WecomUserID == "" {
		return Result{Status: "skipped", Message: "接收人未配置企业微信 UserID"}
	}
	token, err := s.wecomToken(ctx, cfg.CorpID, cfg.Secret)
	if err != nil {
		return Result{Status: "failed", Message: err.Error()}
	}
	body := map[string]any{
		"touser":  recipient.WecomUserID,
		"agentid": cfg.AgentID,
	}
	if payload.TemplateChannel == "wecom_text" {
		body["msgtype"] = "text"
		body["text"] = map[string]string{
			"content": payload.Title + "\n" + payload.Content + "\n查看详情：" + payload.URL,
		}
	} else {
		button := payload.ButtonText
		if button == "" {
			button = "查看数据"
		}
		description := payload.Content
		if len([]rune(description)) > 480 {
			description = string([]rune(description)[:480]) + "…"
		}
		body["msgtype"] = "textcard"
		body["textcard"] = map[string]any{
			"title":       payload.Title,
			"description": description,
			"url":         payload.URL,
			"btntxt":      button,
		}
	}
	endpoint := "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=" + url.QueryEscape(token)
	var response wecomResponse
	if err := s.postJSON(ctx, endpoint, body, &response); err != nil {
		return Result{Status: "failed", Message: err.Error()}
	}
	if response.ErrCode != 0 {
		if response.ErrCode == 42001 || response.ErrCode == 40014 {
			s.clearToken("wecom:" + cfg.CorpID)
		}
		return Result{Status: "failed", Message: fmt.Sprintf("企业微信返回错误 %d: %s", response.ErrCode, response.ErrMsg)}
	}
	return Result{Status: "sent", Message: "企业微信推送成功"}
}

func (s *Sender) sendWeComWebhook(ctx context.Context, webhook string, recipient Recipient, payload Payload) Result {
	content := fmt.Sprintf("### %s\n> %s\n\n[%s](%s)", payload.Title, strings.ReplaceAll(payload.Content, "\n", "\n> "), payload.ButtonText, payload.URL)
	body := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": content,
		},
	}
	var response wecomResponse
	if err := s.postJSON(ctx, webhook, body, &response); err != nil {
		return Result{Status: "failed", Message: err.Error()}
	}
	if response.ErrCode != 0 {
		return Result{Status: "failed", Message: fmt.Sprintf("企业微信返回错误 %d: %s", response.ErrCode, response.ErrMsg)}
	}
	return Result{Status: "sent", Message: "企业微信机器人推送成功"}
}

func (s *Sender) sendWeChat(ctx context.Context, config model.ChannelConfig, recipient Recipient, payload Payload) Result {
	var cfg WeChatConfig
	if err := json.Unmarshal([]byte(config.ConfigJSON), &cfg); err != nil {
		return Result{Status: "failed", Message: "微信公众号配置格式错误"}
	}
	if !config.Enabled || cfg.DryRun {
		if s.allowMock {
			return Result{Status: "simulated", Message: "微信公众号未启用或处于演练模式，已记录模拟推送"}
		}
		return Result{Status: "failed", Message: "微信公众号通道未启用"}
	}
	if cfg.AppID == "" || cfg.Secret == "" || cfg.TemplateID == "" {
		if s.allowMock {
			return Result{Status: "simulated", Message: "微信公众号凭证未配置，已记录模拟推送"}
		}
		return Result{Status: "failed", Message: "微信公众号凭证未配置"}
	}
	if recipient.WechatOpenID == "" {
		return Result{Status: "skipped", Message: "接收人未配置微信 OpenID"}
	}
	token, err := s.wechatToken(ctx, cfg.AppID, cfg.Secret)
	if err != nil {
		return Result{Status: "failed", Message: err.Error()}
	}
	body := map[string]any{
		"touser":      recipient.WechatOpenID,
		"template_id": cfg.TemplateID,
		"url":         payload.URL,
		"data": map[string]any{
			"first":    map[string]string{"value": payload.Title},
			"keyword1": map[string]string{"value": truncate(payload.Content, 60)},
			"keyword2": map[string]string{"value": time.Now().Format("2006-01-02 15:04")},
			"remark":   map[string]string{"value": "点击卡片查看完整销售数据"},
		},
	}
	endpoint := "https://api.weixin.qq.com/cgi-bin/message/template/send?access_token=" + url.QueryEscape(token)
	var response wechatResponse
	if err := s.postJSON(ctx, endpoint, body, &response); err != nil {
		return Result{Status: "failed", Message: err.Error()}
	}
	if response.ErrCode != 0 {
		if response.ErrCode == 40001 || response.ErrCode == 42001 {
			s.clearToken("wechat:" + cfg.AppID)
		}
		return Result{Status: "failed", Message: fmt.Sprintf("微信公众号返回错误 %d: %s", response.ErrCode, response.ErrMsg)}
	}
	return Result{Status: "sent", Message: "微信公众号推送成功"}
}

func (s *Sender) wecomToken(ctx context.Context, corpID, secret string) (string, error) {
	key := "wecom:" + corpID
	if token := s.getToken(key); token != "" {
		return token, nil
	}
	endpoint := "https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=" + url.QueryEscape(corpID) + "&corpsecret=" + url.QueryEscape(secret)
	var response struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := s.getJSON(ctx, endpoint, &response); err != nil {
		return "", err
	}
	if response.ErrCode != 0 || response.AccessToken == "" {
		return "", fmt.Errorf("获取企业微信 access_token 失败: %s", response.ErrMsg)
	}
	s.setToken(key, response.AccessToken, response.ExpiresIn)
	return response.AccessToken, nil
}

func (s *Sender) wechatToken(ctx context.Context, appID, secret string) (string, error) {
	key := "wechat:" + appID
	if token := s.getToken(key); token != "" {
		return token, nil
	}
	endpoint := "https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=" + url.QueryEscape(appID) + "&secret=" + url.QueryEscape(secret)
	var response struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := s.getJSON(ctx, endpoint, &response); err != nil {
		return "", err
	}
	if response.ErrCode != 0 || response.AccessToken == "" {
		return "", fmt.Errorf("获取微信公众号 access_token 失败: %s", response.ErrMsg)
	}
	s.setToken(key, response.AccessToken, response.ExpiresIn)
	return response.AccessToken, nil
}

func (s *Sender) getToken(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.tokens[key]
	if !ok || time.Now().After(item.ExpiresAt) {
		return ""
	}
	return item.Value
}

func (s *Sender) setToken(key, value string, expiresIn int) {
	if expiresIn <= 0 {
		expiresIn = 7000
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[key] = cachedToken{Value: value, ExpiresAt: time.Now().Add(time.Duration(expiresIn-120) * time.Second)}
}

func (s *Sender) clearToken(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, key)
}

func (s *Sender) getJSON(ctx context.Context, endpoint string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s", response.Status)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(target)
}

func (s *Sender) postJSON(ctx context.Context, endpoint string, body any, target any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s", response.Status)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(target)
}

type wecomResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

type wechatResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func truncate(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}

var _ = strconv.Itoa
