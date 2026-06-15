// web/sse_emitter.go
//
// SseEmitter 实现 emitter.Emitter 接口，将所有事件推送到 Session 的 EventCh，
// 由 SSE HTTP handler 消费并以 text/event-stream 格式推送给浏览器。
package web

// SseEmitter 将 agent 事件转为 SSE 事件流。
type SseEmitter struct {
	session *Session
}

// NewSseEmitter 创建一个绑定到指定 Session 的 SseEmitter
func NewSseEmitter(s *Session) *SseEmitter {
	return &SseEmitter{session: s}
}

// emit 将事件推入 Session.EventCh
func (e *SseEmitter) emit(ch string, data any) {
	e.session.EventCh <- Event{CH: ch, Data: data}
}

// ─── 会话生命周期 ───

func (e *SseEmitter) EmitSessionStart(model, logPath string) {
	e.emit("system", map[string]string{
		"event":    "ready",
		"model":    model,
		"log_path": logPath,
	})
}

func (e *SseEmitter) EmitSessionEnd(totalTurns int) {
	e.emit("system", map[string]any{
		"event":       "session_end",
		"total_turns": totalTurns,
	})
}

func (e *SseEmitter) EmitTurnSeparator() {
	// Web 模式不需要视觉分隔符，no-op
}

// ─── Agent 输出 ───

func (e *SseEmitter) EmitAssistant(text string) {
	e.emit("agent", map[string]string{
		"type":    "assistant",
		"content": text,
	})
}

func (e *SseEmitter) EmitReasoning(text string) {
	e.emit("agent", map[string]string{
		"type":    "reasoning",
		"content": text,
	})
}

func (e *SseEmitter) EmitDone() {
	e.emit("agent", map[string]string{
		"type": "done",
	})
}

// ─── 工具事件 ───

func (e *SseEmitter) EmitToolCall(name, argsPreview string) {
	e.emit("agent", map[string]string{
		"type": "tool_call",
		"name": name,
		"args": argsPreview,
	})
}

func (e *SseEmitter) EmitToolResult(name, content string, isError bool) {
	e.emit("agent", map[string]any{
		"type":     "tool_result",
		"name":     name,
		"content":  content,
		"is_error": isError,
	})
}

func (e *SseEmitter) EmitSubAgent(promptPreview string) {
	e.emit("agent", map[string]string{
		"type":   "sub_agent",
		"prompt": promptPreview,
	})
}

func (e *SseEmitter) EmitHookBlocked(toolName, reason string) {
	e.emit("agent", map[string]string{
		"type":   "hook_blocked",
		"tool":   toolName,
		"reason": reason,
	})
}

// ─── 计划 / 压缩 ───

func (e *SseEmitter) EmitTodoPanel(rendered string) {
	e.emit("agent", map[string]string{
		"type":    "todo_panel",
		"content": rendered,
	})
}

func (e *SseEmitter) EmitCompact(beforeBytes, afterBytes int) {
	e.emit("system", map[string]any{
		"event":        "compact",
		"before_bytes": beforeBytes,
		"after_bytes":  afterBytes,
	})
}

func (e *SseEmitter) EmitError(scope, msg string) {
	e.emit("system", map[string]string{
		"event":   "error",
		"scope":   scope,
		"message": msg,
	})
}

func (e *SseEmitter) EmitInfo(msg string) {
	e.emit("system", map[string]string{
		"event":   "info",
		"message": msg,
	})
}

func (e *SseEmitter) EmitEmotion(state string, meta map[string]string) {
	data := map[string]any{
		"state": state,
	}
	for k, v := range meta {
		data[k] = v
	}
	e.emit("emotion", data)
}

func (e *SseEmitter) EmitSystem(event string, data map[string]string) {
	payload := map[string]any{
		"event": event,
	}
	for k, v := range data {
		payload[k] = v
	}
	e.emit("system", payload)
}
