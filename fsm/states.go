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
	Role      string           `json:"role"`
	Content   interface{}      `json:"content"`              // 可以是字符串或工具结果数组
	ToolCalls []tools.ToolCall `json:"tool_calls,omitempty"` // 模型返回的工具调用
}
