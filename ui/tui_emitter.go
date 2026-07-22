// ui/tui_emitter.go
//
// TuiEmitter 实现 emitter.Emitter 接口，将 agentLoop 事件转发到 bubbletea 的 UIEvent channel。
package ui

// TuiEmitter 将 agentLoop 的所有 Emit* 调用转为 UIEvent 发送到 channel。
type TuiEmitter struct {
	ch chan UIEvent
}

// NewTuiEmitter 创建一个带缓冲的 TuiEmitter。
func NewTuiEmitter(ch chan UIEvent) *TuiEmitter {
	return &TuiEmitter{ch: ch}
}

// ---------- emitter.Emitter 接口实现 ----------

func (e *TuiEmitter) EmitSessionStart(model, logPath string) {
	e.ch <- UIEvent{Type: EventSessionStart, Data: [2]string{model, logPath}}
}

func (e *TuiEmitter) EmitSessionEnd(totalTurns int) {
	e.ch <- UIEvent{Type: EventSessionEnd, Data: totalTurns}
}

func (e *TuiEmitter) EmitTurnSeparator() {
	e.ch <- UIEvent{Type: EventTurnSeparator}
}

func (e *TuiEmitter) EmitAssistant(text string) {
	e.ch <- UIEvent{Type: EventAssistant, Data: text}
}

func (e *TuiEmitter) EmitReasoning(text string) {
	e.ch <- UIEvent{Type: EventReasoning, Data: text}
}

func (e *TuiEmitter) EmitDone() {
	// CLI TUI 模式暂不需要显式 done 信号
}

func (e *TuiEmitter) EmitToolCall(name, argsPreview string) {
	e.ch <- UIEvent{Type: EventToolCall, Data: ToolCallData{Name: name, Args: argsPreview}}
}

func (e *TuiEmitter) EmitToolResult(name, content string, isError bool) {
	e.ch <- UIEvent{Type: EventToolResult, Data: ToolResultData{
		Name: name, Content: content, IsError: isError,
	}}
}

func (e *TuiEmitter) EmitSubAgent(promptPreview string) {
	e.ch <- UIEvent{Type: EventSubAgent, Data: promptPreview}
}

func (e *TuiEmitter) EmitHookBlocked(toolName, reason string) {
	e.ch <- UIEvent{Type: EventHookBlocked, Data: [2]string{toolName, reason}}
}

func (e *TuiEmitter) EmitTodoPanel(rendered string) {
	e.ch <- UIEvent{Type: EventTodoPanel, Data: rendered}
}

func (e *TuiEmitter) EmitCompact(beforeBytes, afterBytes int) {
	e.ch <- UIEvent{Type: EventCompact, Data: [2]int{beforeBytes, afterBytes}}
}

func (e *TuiEmitter) EmitError(scope, msg string) {
	e.ch <- UIEvent{Type: EventError, Data: [2]string{scope, msg}}
}

func (e *TuiEmitter) EmitInfo(msg string) {
	e.ch <- UIEvent{Type: EventInfo, Data: msg}
}

func (e *TuiEmitter) EmitEmotion(state string, meta map[string]string) {
	// CLI TUI 模式无宠物情绪
}

func (e *TuiEmitter) EmitSystem(event string, data map[string]string) {
	// CLI TUI 模式无系统事件流
}
