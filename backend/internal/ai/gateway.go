// Package ai 是大模型对接层：OpenAI Chat Completions 兼容协议，
// 云端（DeepSeek/通义/Kimi/OpenAI）与本地 Ollama 统一接入，
// 通过 Function Calling 暴露只读查询工具。
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Config 由设置页写入。
type Config struct {
	BaseURL string // 例如 https://api.deepseek.com/v1 或 http://127.0.0.1:11434/v1
	APIKey  string
	Model   string // 例如 deepseek-chat / qwen2.5:3b
}

// Gateway 持有一个 OpenAI 兼容客户端。
type Gateway struct {
	cfg Config
	hc  *http.Client
}

func New(cfg Config) *Gateway {
	return &Gateway{cfg: cfg, hc: &http.Client{Timeout: 120 * time.Second}}
}

// ToolHandler 执行模型发起的工具调用，返回 JSON 字符串结果。
type ToolHandler func(name string, args json.RawMessage) (string, error)

// ---- OpenAI 兼容协议结构 ----

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Tools    []any     `json:"tools,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
}

// tools 暴露给模型的 Function Calling 工具集（MVP 只读）。
var tools = []any{
	map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "query_ip",
			"description": "查询某个 IP 的状态、标注与最近在线情况",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"ip": map[string]string{"type": "string"}},
				"required":   []string{"ip"},
			},
		},
	},
	map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "get_subnet_usage",
			"description": "获取子网使用率与空闲地址统计",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"cidr": map[string]string{"type": "string"}},
				"required":   []string{"cidr"},
			},
		},
	},
	map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "list_conflicts",
			"description": "列出当前的 IP 冲突与未授权设备告警",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	},
}

const systemPrompt = `你是 IPAMBox 的网络运维助手。规则：
1. 优先调用工具获取真实数据，绝不编造 IP/MAC/告警信息；
2. 用简洁的中文回答，IP 和 MAC 用行内代码格式；
3. 发现异常（冲突、未授权设备）时主动给出处理建议。`

// Chat 执行完整对话：模型可多次发起工具调用，直至给出最终文本。
func (g *Gateway) Chat(ctx context.Context, userMsg string, handle ToolHandler) (string, error) {
	messages := []message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMsg},
	}

	for round := 0; round < 6; round++ { // 工具调用轮次上限
		resp, err := g.round(ctx, messages)
		if err != nil {
			return "", err
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("llm gateway: empty choices")
		}
		msg := resp.Choices[0].Message
		messages = append(messages, msg)

		if len(msg.ToolCalls) == 0 { // 无工具调用 → 最终答案
			return msg.Content, nil
		}
		for _, tc := range msg.ToolCalls {
			result, err := handle(tc.Function.Name, json.RawMessage(tc.Function.Arguments))
			if err != nil {
				result = fmt.Sprintf(`{"error":%q}`, err.Error())
			}
			messages = append(messages, message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
			})
		}
	}
	return "", fmt.Errorf("llm gateway: too many tool rounds")
}

func (g *Gateway) round(ctx context.Context, messages []message) (*chatResponse, error) {
	body, _ := json.Marshal(chatRequest{Model: g.cfg.Model, Messages: messages, Tools: tools})
	req, err := http.NewRequestWithContext(ctx, "POST", g.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if g.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)
	}
	resp, err := g.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("llm gateway: status %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	var out chatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
