// metrics/metrics_test.go
//
// Collector 单元测试：验证 JSONL 事件采集、延迟配对、用量透传、
// is_error 标记，以及关键的注册顺序不变量（观察者先于短路 handler 注册）。
package metrics

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"zoomClient/clients"
	"zoomClient/hook"
	"zoomClient/logger"
)

// TestMain 在所有测试运行前初始化全局日志：短路场景会触发 Runner 内的
// logger.Log.Debug 调用，不初始化会引发空指针 panic（与 hook 包测试一致）。
func TestMain(m *testing.M) {
	logger.Init()
	defer logger.Sync()
	m.Run()
}

// parseEvents 将 JSONL 输出解析为事件列表
func parseEvents(t *testing.T, buf *bytes.Buffer) []Event {
	t.Helper()
	var events []Event
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("invalid JSONL line: %q, err: %v", line, err)
		}
		events = append(events, ev)
	}
	return events
}

// findEvent 返回首个匹配类型的事件
func findEvent(events []Event, name string) (Event, bool) {
	for _, ev := range events {
		if ev.Event == name {
			return ev, true
		}
	}
	return Event{}, false
}

// TestCollector_FullLifecycle 按真实事件顺序驱动一轮完整采集，校验事件类型与关键字段
func TestCollector_FullLifecycle(t *testing.T) {
	var buf bytes.Buffer
	c := NewCollector(&buf)
	runner := hook.NewRunner()
	c.Register(runner)

	runner.HookRun(hook.EventSessionStart, map[string]any{"model": "test-model", "session_id": "sess-1"})
	runner.HookRun(hook.EventPreChat, map[string]any{"model": "test-model", "est_tokens": 42})
	runner.HookRun(hook.EventPostChat, map[string]any{
		"model":            "test-model",
		"content":          "hello",
		"tool_calls_count": 1,
		"usage":            clients.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		"retry_count":      0,
	})
	runner.HookRun(hook.EventPostToolUse, map[string]any{
		"tool_name": "read_file",
		"input":     map[string]any{"path": "/tmp/a.txt"},
		"output":    "file content",
		"is_error":  false,
	})
	runner.HookRun(hook.EventSessionEnd, map[string]any{"total_turns": 1})

	events := parseEvents(t, &buf)
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d: %s", len(events), buf.String())
	}

	// 事件顺序与类型（PreChat 只更新内部时间戳，不产生事件）
	wantOrder := []string{"session_start", "llm_call", "tool_call", "session_end"}
	for i, want := range wantOrder {
		if events[i].Event != want {
			t.Errorf("event[%d] = %q, want %q", i, events[i].Event, want)
		}
	}

	// session_start 携带 session_id
	start := events[0]
	if start.Fields["session_id"] != "sess-1" {
		t.Errorf("session_start.session_id = %v, want sess-1", start.Fields["session_id"])
	}

	// llm_call 携带延迟、估算 token 与真实用量
	llm := events[1]
	if _, ok := llm.Fields["latency_ms"]; !ok {
		t.Error("llm_call should carry latency_ms when paired with PreChat")
	}
	if llm.Fields["est_tokens"] != float64(42) {
		t.Errorf("llm_call.est_tokens = %v, want 42", llm.Fields["est_tokens"])
	}
	usage, ok := llm.Fields["usage"].(map[string]any)
	if !ok {
		t.Fatalf("llm_call.usage missing or wrong type: %v", llm.Fields["usage"])
	}
	if usage["total_tokens"] != float64(15) {
		t.Errorf("llm_call.usage.total_tokens = %v, want 15", usage["total_tokens"])
	}

	// tool_call 携带完整入出参与 is_error
	tool := events[2]
	if tool.Fields["tool"] != "read_file" {
		t.Errorf("tool_call.tool = %v, want read_file", tool.Fields["tool"])
	}
	if tool.Fields["is_error"] != false {
		t.Errorf("tool_call.is_error = %v, want false", tool.Fields["is_error"])
	}

	// session_end 携带总轮数
	end := events[3]
	if end.Fields["total_turns"] != float64(1) {
		t.Errorf("session_end.total_turns = %v, want 1", end.Fields["total_turns"])
	}
}

// TestCollector_ZeroUsageOmitted 后端未上报用量（零值）时不应输出 usage 字段
func TestCollector_ZeroUsageOmitted(t *testing.T) {
	var buf bytes.Buffer
	c := NewCollector(&buf)
	runner := hook.NewRunner()
	c.Register(runner)

	runner.HookRun(hook.EventPostChat, map[string]any{
		"model":            "m",
		"content":          "x",
		"tool_calls_count": 0,
		"usage":            clients.TokenUsage{},
	})

	events := parseEvents(t, &buf)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if _, exists := events[0].Fields["usage"]; exists {
		t.Error("zero usage should be omitted from llm_call event")
	}
}

// TestCollector_LatencyWithoutPreChat 未观察到 PreChat 时不应输出无意义的延迟字段
func TestCollector_LatencyWithoutPreChat(t *testing.T) {
	var buf bytes.Buffer
	c := NewCollector(&buf)
	runner := hook.NewRunner()
	c.Register(runner)

	runner.HookRun(hook.EventPostChat, map[string]any{
		"model": "m", "content": "x", "tool_calls_count": 0,
	})

	events := parseEvents(t, &buf)
	if _, exists := events[0].Fields["latency_ms"]; exists {
		t.Error("latency_ms should be omitted when no PreChat was observed")
	}
}

// TestCollector_RetryLatencyRepaired 重试场景：新 PreChat 覆盖旧时间戳，延迟配对仍成立
func TestCollector_RetryLatencyRepaired(t *testing.T) {
	var buf bytes.Buffer
	c := NewCollector(&buf)
	runner := hook.NewRunner()
	c.Register(runner)

	// 第一次尝试：PreChat → LLMError（失败）
	runner.HookRun(hook.EventPreChat, map[string]any{"model": "m", "est_tokens": 1})
	runner.HookRun(hook.EventLLMError, map[string]any{"model": "m", "error": "timeout", "retry_count": 0})
	// 第二次尝试：PreChat → PostChat（成功）
	runner.HookRun(hook.EventPreChat, map[string]any{"model": "m", "est_tokens": 2})
	runner.HookRun(hook.EventPostChat, map[string]any{
		"model": "m", "content": "ok", "tool_calls_count": 0, "retry_count": 1,
	})

	events := parseEvents(t, &buf)
	errEvent, ok := findEvent(events, "llm_error")
	if !ok {
		t.Fatal("llm_error event missing")
	}
	if _, exists := errEvent.Fields["latency_ms"]; !exists {
		t.Error("llm_error should carry latency_ms for the failed attempt")
	}
	llmEvent, ok := findEvent(events, "llm_call")
	if !ok {
		t.Fatal("llm_call event missing")
	}
	// 重试成功的调用延迟应基于第二次 PreChat，量级为毫秒级而非天文数字
	if latency, exists := llmEvent.Fields["latency_ms"]; exists {
		if v, ok := latency.(float64); !ok || v > 10_000 {
			t.Errorf("llm_call.latency_ms = %v, expected small value paired with second PreChat", latency)
		}
	} else {
		t.Error("llm_call should carry latency_ms")
	}
}

// TestCollector_HookBlockedToolCall hook 拦截的调用以 is_error=true 记入轨迹（安全拦截率统计依据）
func TestCollector_HookBlockedToolCall(t *testing.T) {
	var buf bytes.Buffer
	c := NewCollector(&buf)
	runner := hook.NewRunner()
	c.Register(runner)

	runner.HookRun(hook.EventPostToolUse, map[string]any{
		"tool_name": "run_bash",
		"input":     map[string]any{"command": "rm -rf /"},
		"output":    "<hook blocked> dangerous command blocked by hook: rm -rf /",
		"is_error":  true,
	})

	events := parseEvents(t, &buf)
	if len(events) != 1 || events[0].Event != "tool_call" {
		t.Fatalf("expected single tool_call event, got %v", events)
	}
	if events[0].Fields["is_error"] != true {
		t.Errorf("blocked tool call should record is_error=true, got %v", events[0].Fields["is_error"])
	}
}

// TestCollector_RegisteredBeforeShortCircuitHandler 关键不变量：
// 采集器先于返回非 Continue 的业务 handler 注册时，短路场景不丢事件。
// 若未来改动注册顺序导致本测试失败，说明埋点会在 Block/Retry 时漏采。
func TestCollector_RegisteredBeforeShortCircuitHandler(t *testing.T) {
	var buf bytes.Buffer
	c := NewCollector(&buf)
	runner := hook.NewRunner()

	// 观察者先注册
	c.Register(runner)
	// 业务 handler 后注册，且直接短路（模拟 PostChatValidate Retry / PreToolUse Block）
	runner.HookRegister(hook.EventPostChat, func(p map[string]any) hook.HookResult {
		return hook.HookResult{ExitCode: hook.ExitRetry}
	})
	runner.HookRegister(hook.EventPostToolUse, func(p map[string]any) hook.HookResult {
		return hook.HookResult{ExitCode: hook.ExitBlock}
	})

	runner.HookRun(hook.EventPreChat, map[string]any{"model": "m"})
	runner.HookRun(hook.EventPostChat, map[string]any{
		"model": "m", "content": "x", "tool_calls_count": 0,
	})
	runner.HookRun(hook.EventPostToolUse, map[string]any{
		"tool_name": "run_bash", "input": nil, "output": "blocked",
	})

	events := parseEvents(t, &buf)
	if _, ok := findEvent(events, "llm_call"); !ok {
		t.Error("llm_call event lost when a later handler short-circuits with Retry")
	}
	if _, ok := findEvent(events, "tool_call"); !ok {
		t.Error("tool_call event lost when a later handler short-circuits with Block")
	}
}

// TestCollector_AllHandlersAlwaysContinue 采集 handler 恒返回 Continue，不影响主流程决策
func TestCollector_AllHandlersAlwaysContinue(t *testing.T) {
	var buf bytes.Buffer
	c := NewCollector(&buf)
	payloads := []map[string]any{
		{"model": "m"},
		{"model": "m", "est_tokens": 1},
		{"model": "m", "content": "x", "tool_calls_count": 0},
		{"model": "m", "error": "e", "retry_count": 0},
		{"tool_name": "t", "input": nil, "output": "o"},
		{"total_turns": 0},
	}
	results := []hook.HookResult{
		c.onSessionStart(payloads[0]),
		c.onPreChat(payloads[1]),
		c.onPostChat(payloads[2]),
		c.onLLMError(payloads[3]),
		c.onPostToolUse(payloads[4]),
		c.onSessionEnd(payloads[5]),
	}
	for i, r := range results {
		if r.ExitCode != hook.ExitContinue {
			t.Errorf("handler[%d] exit code = %d, want ExitContinue(0)", i, r.ExitCode)
		}
	}
}
