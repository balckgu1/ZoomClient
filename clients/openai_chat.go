package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"zoomClient/fsm"
	"zoomClient/tools"
)

// OpenAIClient is an OpenAI Client
type OpenAIClient struct {
	BaseURL string       // API base url
	APIKey  string       // API Key
	Client  *http.Client // HTTP Client
}

// NewOpenAIClient 初始化 OpenAI Client
func NewOpenAIClient(baseURL, apiKey string) *OpenAIClient {
	return &OpenAIClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// OpenAITool 表示发送给 OpenAI 的 tool 定义
type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

// OpenAIFunction 工具的函数描述
type OpenAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// OpenAIMessage OpenAI 兼容协议中的消息结构
type OpenAIMessage struct {
	Role             string           `json:"role"`
	Content          interface{}      `json:"content,omitempty"`
	ToolCalls        []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`      // tool role 消息所关联的工具调用 ID
	ReasoningContent string           `json:"reasoning_content,omitempty"` // thinking 模式下模型产生的推理内容，多轮对话必须原样回传
}

// OpenAIToolCall OpenAI 兼容的工具调用结构
// arguments 在 OpenAI 协议中是 JSON 字符串
type OpenAIToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function OpenAIToolCallFunction `json:"function"`
}

// OpenAIToolCallFunction 工具调用对应的函数信息
type OpenAIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
}

// OpenAIChatRequest OpenAI Chat 请求
type OpenAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	Stream      bool            `json:"stream"`
	Temperature float64         `json:"temperature,omitempty"`
}

// OpenAIChatResponse OpenAI Chat 响应
type OpenAIChatResponse struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
}

// OpenAIChoice 单个候选结果
type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// BuildOpenAITools 将 tools 切片转换为 OpenAI 兼容的 tool schema
func BuildOpenAITools(toolList []tools.Tool) []OpenAITool {
	result := make([]OpenAITool, 0, len(toolList))
	for _, t := range toolList {
		result = append(result, OpenAITool{
			Type: "function",
			Function: OpenAIFunction{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return result
}

// convertToOpenAIMessages 将内部 fsm.Message 列表转换为 OpenAI 兼容协议消息列表
//   - tool role 消息需携带 tool_call_id
//   - assistant 工具调用中的 arguments 需序列化为 JSON 字符串
func convertToOpenAIMessages(messages []fsm.Message) []OpenAIMessage {
	result := make([]OpenAIMessage, 0, len(messages))
	for _, msg := range messages {
		oiMsg := OpenAIMessage{
			Role:             msg.Role,
			Content:          msg.Content,
			ToolCallID:       msg.ToolCallID,
			ReasoningContent: msg.ReasoningContent,
		}

		// 转换 assistant 消息中的工具调用：将参数对象序列化为 JSON 字符串
		if len(msg.ToolCalls) > 0 {
			oiToolCalls := make([]OpenAIToolCall, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				argsBytes, err := json.Marshal(tc.Function.Arguments)
				if err != nil {
					// 序列化失败时退化为空对象，避免阻断整个请求
					argsBytes = []byte("{}")
				}
				oiToolCalls = append(oiToolCalls, OpenAIToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: OpenAIToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: string(argsBytes),
					},
				})
			}
			oiMsg.ToolCalls = oiToolCalls
		}

		result = append(result, oiMsg)
	}
	return result
}

// convertFromOpenAIToolCalls 将 OpenAI APU 返回工具调用转换为 ToolCall 格式
// 将 arguments 的 JSON 字符串解析为 map
func convertFromOpenAIToolCalls(openaiToolCalls []OpenAIToolCall) []tools.ToolCall {
	if len(openaiToolCalls) == 0 {
		return nil
	}
	result := make([]tools.ToolCall, 0, len(openaiToolCalls))
	for _, tc := range openaiToolCalls {
		args := map[string]interface{}{}
		if tc.Function.Arguments != "" {
			// 解析失败时保留为空 map，让上层工具自行处理参数缺失
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		}
		result = append(result, tools.ToolCall{
			ID: tc.ID,
			Function: tools.ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: args,
			},
		})
	}
	return result
}

// Chat 实现 ChatClient 接口，向 OpenAI 后端发起一次对话
//
//   - 工具列表转换为 OpenAI 兼容格式
//   - 消息列表中的工具调用 arguments 序列化为字符串
//   - 响应中的 arguments 字符串反序列化为 map
func (c *OpenAIClient) Chat(model string, messages []fsm.Message, toolList []tools.Tool, options map[string]interface{}) (*ChatResponse, error) {
	// 协议转换
	openaiTools := BuildOpenAITools(toolList)
	openaiMessages := convertToOpenAIMessages(messages)

	// 构造请求体
	reqData := OpenAIChatRequest{
		Model:    model,
		Messages: openaiMessages,
		Tools:    openaiTools,
		Stream:   false,
	}

	// 解析额外参数
	if temp, ok := options["temperature"].(float64); ok {
		reqData.Temperature = temp
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("OpenAI request serialization failed: %w", err)
	}

	// 构造 HTTP 请求
	url := fmt.Sprintf("%s/chat/completions", c.BaseURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("OpenAI request creation failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	// 发送 HTTP 请求
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI request send failed: %w", err)
	}
	defer resp.Body.Close()

	// 读取 HTTP 响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("OpenAI request body read failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API returns status code %d: %s", resp.StatusCode, string(body))
	}

	// 解析 HTTP 响应
	var openaiResp OpenAIChatResponse
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		return nil, fmt.Errorf("OpenAI response parsing failed: %w", err)
	}

	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI response has invalid choices")
	}

	// 将 OpenAI 响应归一化为内部 ChatResponse 结构
	choice := openaiResp.Choices[0]
	chatResp := &ChatResponse{
		Model: openaiResp.Model,
		Done:  true,
		Message: fsm.Message{
			Role:             choice.Message.Role,
			Content:          choice.Message.Content,
			ToolCalls:        convertFromOpenAIToolCalls(choice.Message.ToolCalls),
			ReasoningContent: choice.Message.ReasoningContent,
		},
	}
	// content 为空时，设置为空字符串，避免类型断言失败
	if chatResp.Message.Content == nil {
		chatResp.Message.Content = ""
	}

	return chatResp, nil
}
