package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"zoomClient/fsm"
	"zoomClient/tools"
)

// ===================== 客户端定义 =====================

// DeepSeekClient DeepSeek 聊天客户端，使用 OpenAI 兼容协议。
type DeepSeekClient struct {
	BaseURL string       // DeepSeek API base url
	APIKey  string       // 用于鉴权的 API Key
	Client  *http.Client // 底层 HTTP 客户端
}

// NewDeepSeekClient 创建新的 DeepSeek 客户端。
func NewDeepSeekClient(baseURL, apiKey string) *DeepSeekClient {
	return &DeepSeekClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Client:  &http.Client{},
	}
}

// ===================== 请求 / 响应数据模型（OpenAI 兼容） =====================

// DeepSeekTool 表示发送给 DeepSeek 的 tool 定义。
type DeepSeekTool struct {
	Type     string           `json:"type"`
	Function DeepSeekFunction `json:"function"`
}

// DeepSeekFunction 工具的函数描述。
type DeepSeekFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// DeepSeekMessage DeepSeek 协议中的消息结构（OpenAI 兼容）。
type DeepSeekMessage struct {
	Role             string             `json:"role"`
	Content          interface{}        `json:"content,omitempty"`
	ToolCalls        []DeepSeekToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string             `json:"tool_call_id,omitempty"`      // tool 角色消息所关联的工具调用 ID
	ReasoningContent string             `json:"reasoning_content,omitempty"` // thinking 模式下模型产生的推理内容，多轮对话必须原样回传
}

// DeepSeekToolCall OpenAI 兼容的工具调用结构。
// 注意：arguments 字段在 OpenAI 协议中是 JSON 字符串而非对象。
type DeepSeekToolCall struct {
	ID       string                   `json:"id"`
	Type     string                   `json:"type"`
	Function DeepSeekToolCallFunction `json:"function"`
}

// DeepSeekToolCallFunction 工具调用对应的函数信息。
type DeepSeekToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // 注意：JSON 字符串，需自行反序列化为对象
}

// DeepSeekChatRequest DeepSeek 聊天补全请求体。
type DeepSeekChatRequest struct {
	Model       string            `json:"model"`
	Messages    []DeepSeekMessage `json:"messages"`
	Tools       []DeepSeekTool    `json:"tools,omitempty"`
	Stream      bool              `json:"stream"`
	Temperature float64           `json:"temperature,omitempty"`
}

// DeepSeekChatResponse DeepSeek 聊天补全响应体。
type DeepSeekChatResponse struct {
	ID      string           `json:"id"`
	Model   string           `json:"model"`
	Choices []DeepSeekChoice `json:"choices"`
}

// DeepSeekChoice 单个候选结果。
type DeepSeekChoice struct {
	Index        int             `json:"index"`
	Message      DeepSeekMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

// ===================== 转换辅助函数 =====================

// BuildDeepSeekTools 将通用工具接口列表转换为 DeepSeek 所需的 tool schema。
func BuildDeepSeekTools(toolList []tools.Tool) []DeepSeekTool {
	result := make([]DeepSeekTool, 0, len(toolList))
	for _, t := range toolList {
		result = append(result, DeepSeekTool{
			Type: "function",
			Function: DeepSeekFunction{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return result
}

// convertToDeepSeekMessages 将内部 fsm.Message 列表转换为 DeepSeek 协议消息列表。
//
// 重点处理：
//  1. tool 角色消息需携带 tool_call_id 字段；
//  2. assistant 工具调用中的 arguments 需序列化为 JSON 字符串。
func convertToDeepSeekMessages(messages []fsm.Message) []DeepSeekMessage {
	result := make([]DeepSeekMessage, 0, len(messages))
	for _, msg := range messages {
		dsMsg := DeepSeekMessage{
			Role:             msg.Role,
			Content:          msg.Content,
			ToolCallID:       msg.ToolCallID,
			ReasoningContent: msg.ReasoningContent,
		}

		// 转换 assistant 消息中的工具调用：将参数对象序列化为 JSON 字符串
		if len(msg.ToolCalls) > 0 {
			dsToolCalls := make([]DeepSeekToolCall, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				argsBytes, err := json.Marshal(tc.Function.Arguments)
				if err != nil {
					// 序列化失败时退化为空对象，避免阻断整个请求
					argsBytes = []byte("{}")
				}
				dsToolCalls = append(dsToolCalls, DeepSeekToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: DeepSeekToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: string(argsBytes),
					},
				})
			}
			dsMsg.ToolCalls = dsToolCalls
		}

		result = append(result, dsMsg)
	}
	return result
}

// convertFromDeepSeekToolCalls 将 DeepSeek 返回的工具调用转换为内部 ToolCall 格式。
// 主要工作是将 arguments 的 JSON 字符串解析为 map。
func convertFromDeepSeekToolCalls(deepseekToolCalls []DeepSeekToolCall) []tools.ToolCall {
	if len(deepseekToolCalls) == 0 {
		return nil
	}
	result := make([]tools.ToolCall, 0, len(deepseekToolCalls))
	for _, tc := range deepseekToolCalls {
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

// ===================== Chat 方法实现 =====================

// Chat 实现 ChatClient 接口，向 DeepSeek 发起一次对话补全请求。
//
// 实现要点：
//  1. 工具列表转换为 OpenAI 兼容格式；
//  2. 消息列表中的工具调用 arguments 序列化为字符串；
//  3. 响应中的 arguments 字符串反序列化为 map，统一对外抽象。
func (c *DeepSeekClient) Chat(model string, messages []fsm.Message, toolList []tools.Tool, options map[string]interface{}) (*ChatResponse, error) {
	// 1. 协议转换
	deepseekTools := BuildDeepSeekTools(toolList)
	deepseekMessages := convertToDeepSeekMessages(messages)

	// 2. 构造请求体
	reqData := DeepSeekChatRequest{
		Model:    model,
		Messages: deepseekMessages,
		Tools:    deepseekTools,
		Stream:   false,
	}
	if temp, ok := options["temperature"].(float64); ok {
		reqData.Temperature = temp
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("DeepSeek 请求序列化失败: %w", err)
	}

	// 3. 发送 HTTP 请求
	url := fmt.Sprintf("%s/chat/completions", c.BaseURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("DeepSeek 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DeepSeek 请求发送失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("DeepSeek 响应读取失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DeepSeek API 返回状态码 %d: %s", resp.StatusCode, string(body))
	}

	// 4. 解析响应
	var dsResp DeepSeekChatResponse
	if err := json.Unmarshal(body, &dsResp); err != nil {
		return nil, fmt.Errorf("DeepSeek 响应解析失败: %w", err)
	}

	if len(dsResp.Choices) == 0 {
		return nil, fmt.Errorf("DeepSeek 响应中无有效 choices")
	}

	// 5. 归一化为内部 ChatResponse 结构
	choice := dsResp.Choices[0]
	chatResp := &ChatResponse{
		Model: dsResp.Model,
		Done:  true,
		Message: fsm.Message{
			Role:             choice.Message.Role,
			Content:          choice.Message.Content,
			ToolCalls:        convertFromDeepSeekToolCalls(choice.Message.ToolCalls),
			ReasoningContent: choice.Message.ReasoningContent,
		},
	}
	// content 字段为 null 时 Go 会得到 nil，统一回填为空字符串避免 main 中类型断言失败
	if chatResp.Message.Content == nil {
		chatResp.Message.Content = ""
	}

	return chatResp, nil
}
