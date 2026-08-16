// Package alert 负责告警的产生与推送（Webhook / 钉钉 / 企业微信）。
package alert

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// NotifyConfig 通知渠道配置（来自 settings 表）。
type NotifyConfig struct {
	Enabled  bool
	Channel  string // webhook / dingtalk / wecom
	Webhook  string
	Secret   string // 钉钉机器人加签密钥（可选）
	Events   map[string]bool // 关注的事件类型：conflict / offline
}

// Valid 配置是否足以发送通知。
func (c NotifyConfig) Valid() bool {
	return c.Enabled && c.Webhook != "" && (c.Channel == "webhook" || c.Channel == "dingtalk" || c.Channel == "wecom")
}

// Wants 是否关注某事件类型（空集合 = 全部）。
func (c NotifyConfig) Wants(eventType string) bool {
	if len(c.Events) == 0 {
		return true
	}
	return c.Events[eventType]
}

// Send 发送一条通知。同步调用，带 5 秒超时；返回渠道侧错误详情。
func Send(cfg NotifyConfig, title, text string) error {
	if !cfg.Valid() {
		return fmt.Errorf("通知未启用或配置不完整")
	}
	payload, err := buildPayload(cfg, title, text)
	if err != nil {
		return err
	}
	endpoint := cfg.Webhook
	if cfg.Channel == "dingtalk" && cfg.Secret != "" {
		endpoint = signDingTalk(cfg.Webhook, cfg.Secret)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	// 钉钉/企微返回 {"errcode":0,...}
	var ack struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if json.Unmarshal(body, &ack) == nil && ack.ErrCode != 0 {
		return fmt.Errorf("渠道拒绝: %s（errcode %d）", ack.ErrMsg, ack.ErrCode)
	}
	return nil
}

// buildPayload 按渠道构造消息体。
func buildPayload(cfg NotifyConfig, title, text string) ([]byte, error) {
	content := "【IPAMBox】" + title + "\n" + text
	switch cfg.Channel {
	case "dingtalk", "wecom":
		return json.Marshal(map[string]any{
			"msgtype": "text",
			"text":    map[string]string{"content": content},
		})
	default: // 通用 Webhook：结构化 JSON
		return json.Marshal(map[string]any{
			"title": title,
			"text":  text,
			"source": "ipambox",
			"time":  time.Now().Format(time.RFC3339),
		})
	}
}

// signDingTalk 钉钉机器人加签：timestamp + HMAC-SHA256(secret, timestamp+"\n"+secret)。
func signDingTalk(webhook, secret string) string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "\n" + secret))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	sep := "?"
	if strings.Contains(webhook, "?") {
		sep = "&"
	}
	return webhook + sep + "timestamp=" + ts + "&sign=" + url.QueryEscape(sign)
}
