// Package metrics 基于 hook 事件采集 Agent 运行指标（LLM 延迟、token 用量、
// 工具调用轨迹、错误与重试、会话生命周期），以 JSONL 逐行追加落盘。
// 采集结果是评测 harness 与 LangSmith 上报的数据源。
//
// 设计约束：
//   - 纯观察者：所有 handler 恒返回 ExitContinue，绝不影响 agent 主流程；
//   - 写入失败仅告警不中断：埋点属于旁路能力，优先保证业务可用性。
package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"zoomClient/clients"
	"zoomClient/hook"
	"zoomClient/logger"

	"go.uber.org/zap"
)

// Event 单条采集事件，对应 JSONL 中的一行
type Event struct {
	TS     time.Time     `json:"ts"`               // 事件时间戳
	Event  string        `json:"event"`            // 事件类型：session_start / llm_call / llm_error / tool_call / session_end
	Fields map[string]any `json:"fields,omitempty"` // 事件负载（模型、延迟、用量、工具入出参等）
}

// Collector 通过 hook 事件采集运行指标并写入 JSONL。
//
// 单次 LLM 延迟通过 PreChat 时间戳与 PostChat/LLMError 配对计算：
// agentLoop 内 chatWithHooks 的每次尝试均按 PreChat → (PostChat | LLMError) 顺序
// 同步触发，因此"最近一次 PreChat"即为本轮调用的起点；重试场景下每次
// PreChat 覆盖前值，配对依然成立。
type Collector struct {
	mu sync.Mutex
	w  io.Writer

	lastChatStart time.Time // 最近一次 PreChat 时间戳，用于配对计算单次调用延迟
	lastEstTokens int       // PreChat 估算的输入 token 数，透传到 llm_call 事件
}

// NewCollector 创建指标采集器，事件以 JSONL 追加写入 w。
// w 通常为 *os.File（生产）或 *bytes.Buffer（测试）。
func NewCollector(w io.Writer) *Collector {
	return &Collector{w: w}
}

// Register 将采集 handler 注册到 hook Runner，覆盖会话、LLM 调用与工具事件。
//
// 注册顺序约束：必须先于业务 handler 注册。Runner 在首个非 Continue 返回时短路，
// 观察者若注册在后，PostChatValidate 的 Retry、PreToolBlockDangerous 的 Block
// 等场景会跳过采集造成漏记。
func (c *Collector) Register(r *hook.Runner) {
	r.HookRegister(hook.EventSessionStart, c.onSessionStart)
	r.HookRegister(hook.EventPreChat, c.onPreChat)
	r.HookRegister(hook.EventPostChat, c.onPostChat)
	r.HookRegister(hook.EventLLMError, c.onLLMError)
	r.HookRegister(hook.EventPostToolUse, c.onPostToolUse)
	r.HookRegister(hook.EventSessionEnd, c.onSessionEnd)
}

// ─── 内部：事件写入 ───

// emit 将一条事件序列化为 JSONL 写出；写失败仅告警，不影响主流程
func (c *Collector) emit(event string, fields map[string]any) {
	b, err := json.Marshal(Event{TS: time.Now(), Event: event, Fields: fields})
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := fmt.Fprintf(c.w, "%s\n", b); err != nil && logger.Log != nil {
		logger.Log.Warn("metrics event write failed", zap.String("event", event), zap.Error(err))
	}
}

// sinceLastChat 计算距最近一次 PreChat 的耗时（须在持锁状态下调用）。
// 未观察到 PreChat 时 ok=false，此时不输出延迟字段，避免产生无意义的巨大数值。
func (c *Collector) sinceLastChat() (time.Duration, bool) {
	if c.lastChatStart.IsZero() {
		return 0, false
	}
	return time.Since(c.lastChatStart), true
}

// ─── 内部：各事件 handler ───

// onSessionStart 记录会话启动（模型名）
func (c *Collector) onSessionStart(p map[string]any) hook.HookResult {
	fields := map[string]any{"model": p["model"]}
	if sid, ok := p["session_id"]; ok {
		fields["session_id"] = sid
	}
	c.emit("session_start", fields)
	return hook.HookResult{ExitCode: hook.ExitContinue}
}

// onPreChat 记录调用起点时间戳与输入侧估算 token，供 PostChat/LLMError 配对使用
func (c *Collector) onPreChat(p map[string]any) hook.HookResult {
	c.mu.Lock()
	c.lastChatStart = time.Now()
	if est, ok := p["est_tokens"].(int); ok {
		c.lastEstTokens = est
	}
	c.mu.Unlock()
	return hook.HookResult{ExitCode: hook.ExitContinue}
}

// onPostChat 记录一次成功的 LLM 调用：延迟、真实 token 用量、回复内容与工具调用数。
// 注意 PostChat 对每个 attempt 都触发（包括被 PostChatValidate 拒绝重试的），
// 被拒调用同样计入，用于评测浪费调用成本。
func (c *Collector) onPostChat(p map[string]any) hook.HookResult {
	fields := map[string]any{
		"model":            p["model"],
		"content":          p["content"],
		"tool_calls_count": p["tool_calls_count"],
		"retry_count":      p["retry_count"],
	}
	c.mu.Lock()
	if d, ok := c.sinceLastChat(); ok {
		fields["latency_ms"] = d.Milliseconds()
	}
	if c.lastEstTokens > 0 {
		fields["est_tokens"] = c.lastEstTokens
	}
	c.mu.Unlock()
	// 后端未上报用量（零值）时不输出 usage 字段
	if usage, ok := p["usage"].(clients.TokenUsage); ok && usage.TotalTokens > 0 {
		fields["usage"] = usage
	}
	c.emit("llm_call", fields)
	return hook.HookResult{ExitCode: hook.ExitContinue}
}

// onLLMError 记录一次失败的 LLM 调用：错误信息、重试位置与本次尝试的延迟
func (c *Collector) onLLMError(p map[string]any) hook.HookResult {
	fields := map[string]any{
		"model":       p["model"],
		"error":       p["error"],
		"retry_count": p["retry_count"],
	}
	c.mu.Lock()
	if d, ok := c.sinceLastChat(); ok {
		fields["latency_ms"] = d.Milliseconds()
	}
	c.mu.Unlock()
	c.emit("llm_error", fields)
	return hook.HookResult{ExitCode: hook.ExitContinue}
}

// onPostToolUse 记录一次工具调用：完整入参、结果与错误标记。
// hook 拦截（Block）的调用经 mergeToolResults 以 is_error=true 回填，
// 因此被拦截的调用同样会出现在轨迹中，可用于安全拦截率统计。
func (c *Collector) onPostToolUse(p map[string]any) hook.HookResult {
	fields := map[string]any{
		"tool":   p["tool_name"],
		"input":  p["input"],
		"output": p["output"],
	}
	if v, ok := p["is_error"]; ok {
		fields["is_error"] = v
	}
	c.emit("tool_call", fields)
	return hook.HookResult{ExitCode: hook.ExitContinue}
}

// onSessionEnd 记录会话结束（总轮数）
func (c *Collector) onSessionEnd(p map[string]any) hook.HookResult {
	c.emit("session_end", map[string]any{"total_turns": p["total_turns"]})
	return hook.HookResult{ExitCode: hook.ExitContinue}
}
