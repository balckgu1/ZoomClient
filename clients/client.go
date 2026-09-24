package clients

import (
	"zoomClient/fsm"
	"zoomClient/tools"
)

// ChatClient 抽象不同 LLM 服务的聊天能力
//
// 具体实现负责：
//   - 将通用的 []tools.Tool 转换为对应 LLM所需的 tool schema；
//   - 将通用的 []fsm.Message 转换为对应 LLM 的 message 协议；
//   - 调用 LLM API 并将响应转换为 *ChatResponse；
//   - 将工具调用参数统一解析为 map
type ChatClient interface {
	Chat(model string, messages []fsm.Message, toolList []tools.Tool, options map[string]interface{}) (*ChatResponse, error)
}

// TokenUsage 表示一次 LLM 调用的 token 用量，由各后端 API 返回的用量字段归一化而来。
// 后端未上报用量时保持零值，调用方以此判断用量数据是否可用。
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`     // 输入侧 token 数
	CompletionTokens int `json:"completion_tokens"` // 输出侧 token 数
	TotalTokens      int `json:"total_tokens"`      // 总 token 数（后端缺失时以分项之和兜底）
}
