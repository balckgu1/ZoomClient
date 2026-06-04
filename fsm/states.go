package fsm

import "zoomClient/tools"

// State 表示Agent的状态
type State struct {
	Messages         []Message `json:"messages"`
	TurnCount        int       `json:"turn_count"`
	TransitionReason *string   `json:"transition_reason,omitempty"`
}

// Message 表示单条消息
type Message struct {
	Role             string           `json:"role"`
	Content          interface{}      `json:"content"`                     // 字符串或工具结果数组
	ToolCalls        []tools.ToolCall `json:"tool_calls,omitempty"`        // 模型返回的工具调用
	ToolCallID       string           `json:"tool_call_id,omitempty"`      // tool 角色消息所关联的工具调用 ID
	ReasoningContent string           `json:"reasoning_content,omitempty"` // thinking 模式下返回内容
}
