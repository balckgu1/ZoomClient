// ui/tui_events.go
//
// 定义 agentLoop 与 bubbletea TUI 之间的事件类型和数据体。
package ui

// UIEventType 枚举 agentLoop 中所有用户可见事件。
type UIEventType int

const (
	EventSessionStart UIEventType = iota
	EventAssistant
	EventReasoning
	EventToolCall
	EventToolResult
	EventSubAgent
	EventHookBlocked
	EventTodoPanel
	EventCompact
	EventError
	EventInfo
	EventTurnSeparator
	EventSessionEnd
	EventAgentDone // agentLoop 本轮结束，UI 恢复输入焦点
)

// UIEvent 是 agentLoop 推送给 bubbletea 的统一事件体。
type UIEvent struct {
	Type UIEventType
	Data any // string 或具体 struct
}

// ToolCallData 携带工具调用的名称和参数预览。
type ToolCallData struct {
	Name string
	Args string
}

// ToolResultData 携带工具执行结果。
type ToolResultData struct {
	Name    string
	Content string
	IsError bool
}
