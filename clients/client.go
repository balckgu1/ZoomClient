package clients

import (
	"zoomClient/tools"
)

// ChatResponse 模型响应
type ChatResponse struct {
	ID           string     `json:"id"`
	Model        string     `json:"model"`
	CreatedAt    int64      `json:"created_at"`
	Message      Message    `json:"message"`
	FinishReason string     `json:"finish_reason"`
	Done         bool       `json:"done"`
	Usage        TokenUsage `json:"usage"` // token 用量
	// Ollama 原生用量字段：done 行携带 prompt_eval_count / eval_count，
	// 仅用于协议解析，对外统一通过 Usage 访问
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}

// Message 表示单条消息
type Message struct {
	Role             string           `json:"role"`
	Content          interface{}      `json:"content"`                     // 字符串或工具结果数组
	ToolCalls        []tools.ToolCall `json:"tool_calls,omitempty"`        // 模型返回的工具调用
	ToolCallID       string           `json:"tool_call_id,omitempty"`      // tool 角色消息所关联的工具调用 ID
	ReasoningContent string           `json:"reasoning_content,omitempty"` // thinking 模式下返回内容
}

// TokenUsage 表示一次 LLM 调用的 token 用量，由各后端 API 返回的用量字段归一化而来。
type TokenUsage struct {
	PromptTokens          int `json:"prompt_tokens"`     // 输入侧 token 数
	CompletionTokens      int `json:"completion_tokens"` // 输出侧 token 数
	TotalTokens           int `json:"total_tokens"`      // 总 token 数
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
}

// ChatClient 抽象不同 LLM 服务的聊天能力
//
// 具体实现负责：
//   - 将通用的 []tools.Tool 转换为对应 LLM所需的 tool schema；
//   - 将通用的 []Message 转换为对应 LLM 的 message 协议；
//   - 调用 LLM API 并将响应转换为 *ChatResponse；
//   - 将工具调用参数统一解析为 map
type ChatClient interface {
	Chat(model string, messages []Message, toolList []tools.Tool, options map[string]interface{}) (*ChatResponse, error)
}
