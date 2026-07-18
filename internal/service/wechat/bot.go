package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Pusher 微信推送接口
type Pusher interface {
	Push(ctx context.Context, title, content string) error
}

// weComPusher 企业微信群机器人 Webhook 推送
type weComPusher struct {
	webhookURL string
	httpClient *http.Client
}

// NewPusher 创建推送器
// webhookURL 为空时返回 NoopPusher（不推送）
func NewPusher(webhookURL string) Pusher {
	if webhookURL == "" {
		return &noopPusher{}
	}
	return &weComPusher{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Push 推送消息到企业微信群
func (p *weComPusher) Push(ctx context.Context, title, content string) error {
	body, _ := json.Marshal(map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": fmt.Sprintf("【%s】%s", title, content),
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("WeCom webhook 推送失败: %w", err)
	}
	defer resp.Body.Close()
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil // 响应体解析失败但请求已发送，视为成功
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("WeCom 返回错误: %d %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

// noopPusher 空推送器（webhook 未配置时用）
type noopPusher struct{}

func (p *noopPusher) Push(ctx context.Context, title, content string) error {
	return nil
}
