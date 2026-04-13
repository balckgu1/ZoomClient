package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"zoomClient/fsm"
	"zoomClient/tools"
)

// ChatRequest 表示聊天请求的结构
type ChatRequest struct {
	Model    string                 `json:"model"`
	Messages []fsm.Message          `json:"messages"`
	Tools    []OllamaTool           `json:"tools,omitempty"`
	Stream   bool                   `json:"stream,omitempty"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

// ChatResponse 表示聊天响应的结构
type ChatResponse struct {
	Model     string      `json:"model"`
	CreatedAt string      `json:"created_at"`
	Message   fsm.Message `json:"message"`
	Done      bool        `json:"done"`
}

// Chat 发起聊天请求
func (c *OllamaClient) Chat(model string, messages []fsm.Message, ollamaTools []OllamaTool, options map[string]interface{}) (*ChatResponse, error) {
	reqData := ChatRequest{
		Model:    model,
		Messages: messages,
		Tools:    ollamaTools,
		Stream:   false,
		Options:  options,
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("error marshaling JSON: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", c.BaseURL)
	resp, err := c.Client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status code %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	// Ollama API 可能返回 NDJSON（多行JSON）格式，即使 stream:false
	// 每行包含一个 token 片段，需要拼接所有行的 content
	var chatResp ChatResponse
	var fullContent strings.Builder
	var allToolCalls []tools.ToolCall
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var partial ChatResponse
		if err := json.Unmarshal([]byte(line), &partial); err != nil {
			continue
		}
		// 拼接每行的 content 片段
		if partialContent, ok := partial.Message.Content.(string); ok {
			fullContent.WriteString(partialContent)
		}
		// 收集工具调用
		if len(partial.Message.ToolCalls) > 0 {
			allToolCalls = append(allToolCalls, partial.Message.ToolCalls...)
		}
		if partial.Done {
			chatResp = partial
			break
		}
	}
	// 将拼接后的完整内容和工具调用设置回响应
	chatResp.Message.Content = fullContent.String()
	chatResp.Message.ToolCalls = allToolCalls

	return &chatResp, nil
}
