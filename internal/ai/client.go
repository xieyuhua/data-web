// Package ai provides a minimal OpenAI-compatible chat client used for SQL
// authoring assistance. Any provider exposing /v1/chat/completions works
// (DeepSeek, Qwen, Moonshot, Ollama, vLLM, Azure-compatible gateways, ...).
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"biaoge-bi/internal/config"
)

// Client calls an OpenAI-compatible chat completions endpoint.
type Client struct {
	cfg  config.AIConfig
	http *http.Client
}

func NewClient(cfg config.AIConfig) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 90 * time.Second}}
}

func (c *Client) Enabled() bool {
	return c.cfg.Enabled && c.cfg.BaseURL != "" && c.cfg.Model != ""
}

func (c *Client) Model() string { return c.cfg.Model }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Chat sends a system+user prompt pair and returns the assistant's text.
func (c *Client) Chat(ctx context.Context, system, user string) (string, error) {
	if !c.Enabled() {
		return "", errors.New("AI 未启用，请先在「系统配置 → AI」中填写接口地址与模型")
	}
	body := chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: c.cfg.Temperature,
		MaxTokens:   c.cfg.MaxTokens,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("调用 AI 接口失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", errors.New("AI 接口鉴权失败，请检查 API Key")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", errors.New("AI 接口限流，请稍后重试")
	}

	var out chatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("解析 AI 响应失败（HTTP %d）", resp.StatusCode)
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("AI 接口返回错误: %s", out.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI 接口返回 HTTP %d", resp.StatusCode)
	}
	if len(out.Choices) == 0 {
		return "", errors.New("AI 未返回内容")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// Ping validates connectivity with a tiny prompt.
func (c *Client) Ping(ctx context.Context) error {
	if !c.Enabled() {
		return errors.New("AI 未启用")
	}
	_, err := c.Chat(ctx, "You are a health check. Reply with the single word: ok", "ping")
	return err
}
