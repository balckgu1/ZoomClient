// emitter/api_emitter.go
//
// ApiEmitter 实现 Emitter 接口，将所有事件以 NDJSON 格式写入 io.Writer（通常是 os.Stdout）。
// 每行一条 JSON，通过 ch 字段区分通道（agent / emotion / system）。
package emitter

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// ApiEmitter 将 agent 事件转为 NDJSON 输出，供 Tauri Sidecar 前端消费
type ApiEmitter struct {
	w  io.Writer
	mu sync.Mutex // 保护并发写入
}

// NewApiEmitter 创建一个 ApiEmitter，输出到指定 Writer
func NewApiEmitter(w io.Writer) *ApiEmitter {
	return &ApiEmitter{w: w}
}

// ─── 内部辅助 ───

// emit 写入一行 NDJSON
func (e *ApiEmitter) emit(ch string, data any) {
	msg := map[string]any{
		"ch":   ch,
		"data": data,
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	b, _ := json.Marshal(msg)
	fmt.Fprintf(e.w, "%s\n", b)
}

// ─── 会话生命周期 ───

func (e *ApiEmitter) EmitSessionStart(model, logPath string) {
	e.emit("system", map[string]string{
		"event":    "ready",
		"model":    model,
		"log_path": logPath,
	})
}

func (e *ApiEmitter) EmitSessionEnd(totalTurns int) {
	e.emit("system", map[string]any{
		"event":       "session_end",
		"total_turns": totalTurns,
	})
}

func (e *ApiEmitter) EmitTurnSeparator() {
	// API 模式不需要视觉分隔符，no-op
}

// ─── Agent 输出 ───

func (e *ApiEmitter) EmitAssistant(text string) {
	e.emit("agent", map[string]string{
		"type":    "assistant",
		"content": text,
	})
}

func (e *ApiEmitter) EmitReasoning(text string) {
	e.emit("agent", map[string]string{
		"type":    "reasoning",
		"content": text,
	})
}

func (e *ApiEmitter) EmitDone() {
	e.emit("agent", map[string]string{
		"type": "done",
	})
}

// ─── 工具事件 ───

func (e *ApiEmitter) EmitToolCall(name, argsPreview string) {
	e.emit("agent", map[string]string{
		"type": "tool_call",
		"name": name,
		"args": argsPreview,
	})
}

func (e *ApiEmitter) EmitToolResult(name, content string, isError bool) {
	e.emit("agent", map[string]any{
		"type":     "tool_result",
		"name":     name,
		"content":  content,
		"is_error": isError,
	})
}

func (e *ApiEmitter) EmitSubAgent(promptPreview string) {
	e.emit("agent", map[string]string{
		"type":   "sub_agent",
		"prompt": promptPreview,
	})
}

func (e *ApiEmitter) EmitHookBlocked(toolName, reason string) {
	e.emit("agent", map[string]string{
		"type":   "hook_blocked",
		"tool":   toolName,
		"reason": reason,
	})
}

// ─── 计划 / 压缩 ───

func (e *ApiEmitter) EmitTodoPanel(rendered string) {
	e.emit("agent", map[string]string{
		"type":    "todo_panel",
		"content": rendered,
	})
}

func (e *ApiEmitter) EmitCompact(beforeBytes, afterBytes int) {
	e.emit("system", map[string]any{
		"event":        "compact",
		"before_bytes": beforeBytes,
		"after_bytes":  afterBytes,
	})
}

// ─── 系统 / 控制 ───

func (e *ApiEmitter) EmitError(scope, msg string) {
	e.emit("system", map[string]string{
		"event":   "error",
		"scope":   scope,
		"message": msg,
	})
}

func (e *ApiEmitter) EmitInfo(msg string) {
	e.emit("system", map[string]string{
		"event":   "info",
		"message": msg,
	})
}

func (e *ApiEmitter) EmitEmotion(state string, meta map[string]string) {
	data := map[string]any{
		"state": state,
	}
	for k, v := range meta {
		data[k] = v
	}
	e.emit("emotion", data)
}

func (e *ApiEmitter) EmitSystem(event string, data map[string]string) {
	payload := map[string]any{
		"event": event,
	}
	for k, v := range data {
		payload[k] = v
	}
	e.emit("system", payload)
}
