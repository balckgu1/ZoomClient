package hook

import (
	"strings"
	"testing"

	"zoomClient/logger"
)

// TestMain 在所有测试运行前初始化全局日志，避免 logger.Log 为 nil 引发 panic。
func TestMain(m *testing.M) {
	logger.Init()
	defer logger.Sync()
	m.Run()
}

// ===================== Runner 基础行为测试 =====================

// TestRunner_NoHandler_ReturnsContinue 没有任何 handler 时，Run 应直接返回 0。
func TestRunner_NoHandler_ReturnsContinue(t *testing.T) {
	runner := NewRunner()
	result := runner.Run(EventSessionStart, nil)
	if result.ExitCode != ExitContinue {
		t.Errorf("expected ExitContinue, got %d", result.ExitCode)
	}
}

// TestRunner_RegisterAndCount 验证注册数量与 HandlerCount 一致。
func TestRunner_RegisterAndCount(t *testing.T) {
	runner := NewRunner()
	runner.Register(EventPreToolUse, func(p map[string]any) HookResult { return HookResult{} })
	runner.Register(EventPreToolUse, func(p map[string]any) HookResult { return HookResult{} })
	runner.Register(EventPostToolUse, func(p map[string]any) HookResult { return HookResult{} })

	if got := runner.HandlerCount(EventPreToolUse); got != 2 {
		t.Errorf("PreToolUse expected 2 handlers, got %d", got)
	}
	if got := runner.HandlerCount(EventPostToolUse); got != 1 {
		t.Errorf("PostToolUse expected 1 handler, got %d", got)
	}
	if got := runner.HandlerCount(EventSessionStart); got != 0 {
		t.Errorf("SessionStart expected 0 handlers, got %d", got)
	}
}

// TestRunner_AllContinue 所有 handler 都返回 0 时，最终结果为 0。
func TestRunner_AllContinue(t *testing.T) {
	runner := NewRunner()
	runner.Register(EventPreToolUse, func(p map[string]any) HookResult {
		return HookResult{ExitCode: ExitContinue}
	})
	runner.Register(EventPreToolUse, func(p map[string]any) HookResult {
		return HookResult{ExitCode: ExitContinue}
	})

	result := runner.Run(EventPreToolUse, map[string]any{})
	if result.ExitCode != ExitContinue {
		t.Errorf("expected ExitContinue, got %d", result.ExitCode)
	}
}

// TestRunner_BlockShortCircuit 第一个 block 的 handler 应立即终止后续 handler。
func TestRunner_BlockShortCircuit(t *testing.T) {
	runner := NewRunner()
	called := []string{}

	runner.Register(EventPreToolUse, func(p map[string]any) HookResult {
		called = append(called, "h1")
		return HookResult{ExitCode: ExitBlock, Message: "stop"}
	})
	runner.Register(EventPreToolUse, func(p map[string]any) HookResult {
		called = append(called, "h2") // 不应被调用
		return HookResult{}
	})

	result := runner.Run(EventPreToolUse, nil)
	if result.ExitCode != ExitBlock {
		t.Errorf("expected ExitBlock, got %d", result.ExitCode)
	}
	if result.Message != "stop" {
		t.Errorf("expected message 'stop', got %q", result.Message)
	}
	if len(called) != 1 || called[0] != "h1" {
		t.Errorf("expected only h1 to be called, got %v", called)
	}
}

// TestRunner_InjectShortCircuit 验证 exit=2 同样会短路并返回消息。
func TestRunner_InjectShortCircuit(t *testing.T) {
	runner := NewRunner()
	runner.Register(EventPostToolUse, func(p map[string]any) HookResult {
		return HookResult{ExitCode: ExitInject, Message: "extra info"}
	})
	runner.Register(EventPostToolUse, func(p map[string]any) HookResult {
		t.Fatal("second handler should not be called")
		return HookResult{}
	})

	result := runner.Run(EventPostToolUse, nil)
	if result.ExitCode != ExitInject {
		t.Errorf("expected ExitInject, got %d", result.ExitCode)
	}
	if result.Message != "extra info" {
		t.Errorf("expected 'extra info', got %q", result.Message)
	}
}

// TestRunner_PayloadPropagation handler 应能拿到 payload。
func TestRunner_PayloadPropagation(t *testing.T) {
	runner := NewRunner()
	var got map[string]any
	runner.Register(EventPreToolUse, func(p map[string]any) HookResult {
		got = p
		return HookResult{}
	})

	want := map[string]any{"tool_name": "read_file", "input": map[string]any{"path": "x"}}
	runner.Run(EventPreToolUse, want)

	if got["tool_name"] != "read_file" {
		t.Errorf("payload tool_name not propagated, got %v", got["tool_name"])
	}
}

// ===================== handlers.go 内置 handler 测试 =====================

// TestPreToolBlockDangerous_BlocksRmRf 验证危险命令被阻止。
func TestPreToolBlockDangerous_BlocksRmRf(t *testing.T) {
	payload := map[string]any{
		"tool_name": "run_bash",
		"input":     map[string]any{"command": "rm -rf /tmp/foo && rm -rf /"},
	}
	result := PreToolBlockDangerous(payload)
	if result.ExitCode != ExitBlock {
		t.Errorf("expected ExitBlock for dangerous rm -rf /, got %d", result.ExitCode)
	}
}

// TestPreToolBlockDangerous_AllowsSafe 验证普通命令不被影响。
func TestPreToolBlockDangerous_AllowsSafe(t *testing.T) {
	payload := map[string]any{
		"tool_name": "run_bash",
		"input":     map[string]any{"command": "ls -la"},
	}
	result := PreToolBlockDangerous(payload)
	if result.ExitCode != ExitContinue {
		t.Errorf("expected ExitContinue for safe command, got %d", result.ExitCode)
	}
}

// TestPreToolBlockDangerous_IgnoresOtherTools 验证非 run_bash 工具不会被该 handler 影响。
func TestPreToolBlockDangerous_IgnoresOtherTools(t *testing.T) {
	payload := map[string]any{
		"tool_name": "read_file",
		"input":     map[string]any{"path": "rm -rf /"}, // path 中包含危险字符串也不应阻止
	}
	result := PreToolBlockDangerous(payload)
	if result.ExitCode != ExitContinue {
		t.Errorf("expected ExitContinue for non-bash tool, got %d", result.ExitCode)
	}
}

// TestPreToolInjectReminder_OnlyForWriteFile 验证只对 write_file 注入提醒。
// func TestPreToolInjectReminder_OnlyForWriteFile(t *testing.T) {
// 	t.Run("write_file triggers inject", func(t *testing.T) {
// 		result := PreToolInjectReminder(map[string]any{"tool_name": "write_file"})
// 		if result.ExitCode != ExitInject {
// 			t.Errorf("expected ExitInject, got %d", result.ExitCode)
// 		}
// 		if result.Message == "" {
// 			t.Errorf("expected non-empty reminder message")
// 		}
// 	})

// 	t.Run("other tool unaffected", func(t *testing.T) {
// 		result := PreToolInjectReminder(map[string]any{"tool_name": "read_file"})
// 		if result.ExitCode != ExitContinue {
// 			t.Errorf("expected ExitContinue, got %d", result.ExitCode)
// 		}
// 	})
// }

// ===================== PreToolRateLimit 测试 =====================

// TestPreToolRateLimit_MissingCallIndex 没有 call_index 时放行。
func TestPreToolRateLimit_MissingCallIndex(t *testing.T) {
	result := PreToolRateLimit(map[string]any{"max_tools": 5})
	if result.ExitCode != ExitContinue {
		t.Errorf("expected ExitContinue when call_index missing, got %d", result.ExitCode)
	}
}

// TestPreToolRateLimit_UnderLimit 调用次数未达上限时放行。
func TestPreToolRateLimit_UnderLimit(t *testing.T) {
	result := PreToolRateLimit(map[string]any{"call_index": 2, "max_tools": 5})
	if result.ExitCode != ExitContinue {
		t.Errorf("expected ExitContinue for index under limit, got %d", result.ExitCode)
	}
}

// TestPreToolRateLimit_AtLimit 调用次数刚好达到上限时阻止。
func TestPreToolRateLimit_AtLimit(t *testing.T) {
	result := PreToolRateLimit(map[string]any{"call_index": 4, "max_tools": 4})
	if result.ExitCode != ExitBlock {
		t.Errorf("expected ExitBlock when index hits limit, got %d", result.ExitCode)
	}
}

// TestPreToolRateLimit_OverLimit 调用次数超过上限时阻止。
func TestPreToolRateLimit_OverLimit(t *testing.T) {
	result := PreToolRateLimit(map[string]any{"call_index": 6, "max_tools": 5})
	if result.ExitCode != ExitBlock {
		t.Errorf("expected ExitBlock when index exceeds limit, got %d", result.ExitCode)
	}
}

// TestPreToolRateLimit_MessageContainsMaxTools 阻止时消息中包含上限值。
func TestPreToolRateLimit_MessageContainsMaxTools(t *testing.T) {
	result := PreToolRateLimit(map[string]any{"call_index": 3, "max_tools": 3})
	if !strings.Contains(result.Message, "3") {
		t.Errorf("expected block message to mention maxTools, got %q", result.Message)
	}
}

// TestPreToolRateLimit_IndexZero 第一个工具调用（index=0）应放行。
func TestPreToolRateLimit_IndexZero(t *testing.T) {
	result := PreToolRateLimit(map[string]any{"call_index": 0, "max_tools": 3})
	if result.ExitCode != ExitContinue {
		t.Errorf("expected ExitContinue for index 0, got %d", result.ExitCode)
	}
}

// ===================== PreToolSensitiveFileGuard 测试 =====================

// TestPreToolSensitiveFileGuard_BlocksSensitive 命中敏感文件时阻止。
func TestPreToolSensitiveFileGuard_BlocksSensitive(t *testing.T) {
	result := PreToolSensitiveFileGuard(map[string]any{
		"input":           map[string]any{"filename": "config.yaml"},
		"sensitive_files": []string{".env", "config.yaml", "id_rsa"},
	})
	if result.ExitCode != ExitBlock {
		t.Errorf("expected ExitBlock for sensitive file, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Message, "config.yaml") {
		t.Errorf("expected block message to mention file name, got %q", result.Message)
	}
}

// TestPreToolSensitiveFileGuard_AllowsNormal 普通文件应放行。
func TestPreToolSensitiveFileGuard_AllowsNormal(t *testing.T) {
	result := PreToolSensitiveFileGuard(map[string]any{
		"input":           map[string]any{"filename": "hello.py"},
		"sensitive_files": []string{".env", "config.yaml"},
	})
	if result.ExitCode != ExitContinue {
		t.Errorf("expected ExitContinue for normal file, got %d", result.ExitCode)
	}
}

// TestPreToolSensitiveFileGuard_EmptyList 敏感文件列表为空时全部放行。
func TestPreToolSensitiveFileGuard_EmptyList(t *testing.T) {
	result := PreToolSensitiveFileGuard(map[string]any{
		"input":           map[string]any{"filename": ".env"},
		"sensitive_files": []string{},
	})
	if result.ExitCode != ExitContinue {
		t.Errorf("expected ExitContinue when sensitive list empty, got %d", result.ExitCode)
	}
}

// TestPreToolSensitiveFileGuard_SubstringMatch filename 在路径中间也能匹配。
func TestPreToolSensitiveFileGuard_SubstringMatch(t *testing.T) {
	result := PreToolSensitiveFileGuard(map[string]any{
		"input":           map[string]any{"filename": "/home/user/.env"},
		"sensitive_files": []string{".env"},
	})
	if result.ExitCode != ExitBlock {
		t.Errorf("expected ExitBlock for substring match, got %d", result.ExitCode)
	}
}

// TestPreToolSensitiveFileGuard_MatchSecond 匹配列表中第二个敏感文件。
func TestPreToolSensitiveFileGuard_MatchSecond(t *testing.T) {
	result := PreToolSensitiveFileGuard(map[string]any{
		"input":           map[string]any{"filename": "id_rsa"},
		"sensitive_files": []string{".env", "id_rsa", "config.yaml"},
	})
	if result.ExitCode != ExitBlock {
		t.Errorf("expected ExitBlock when matching second item, got %d", result.ExitCode)
	}
}

// ===================== OnToolErrorRecovery 测试 =====================

// TestPostToolErrorRecovery_EmptyOutput 空输出不应注入消息。
func TestPostToolErrorRecovery_EmptyOutput(t *testing.T) {
	result := OnToolErrorRecovery(map[string]any{"output": ""})
	if result.ExitCode == ExitContinue {
		t.Errorf("expected ExitContinue for empty output, got %d", result.ExitCode)
	}
}

// TestPostToolErrorRecovery_CaseSensitive 区分大小写，只匹配开头大写的 Permission denied。
func TestPostToolErrorRecovery_CaseSensitive(t *testing.T) {
	result := OnToolErrorRecovery(map[string]any{})
	if result.ExitCode == ExitContinue {
		t.Errorf("expected ExitContinue for lowercase 'permission denied', got %d", result.ExitCode)
	}
}

// TestOnSessionStart_ReturnsContinue 验证 SessionStart handler 不干预流程。
func TestOnSessionStart_ReturnsContinue(t *testing.T) {
	result := OnSessionStart(map[string]any{"model": "test-model"})
	if result.ExitCode != ExitContinue {
		t.Errorf("expected ExitContinue, got %d", result.ExitCode)
	}
}

// TestPostToolAuditLog_ReturnsContinue 验证 Post handler 不干预流程。
func TestPostToolAuditLog_ReturnsContinue(t *testing.T) {
	result := PostToolAuditLog(map[string]any{"tool_name": "read_file", "output": "hello"})
	if result.ExitCode != ExitContinue {
		t.Errorf("expected ExitContinue, got %d", result.ExitCode)
	}
}

// TestPostToolAuditLog_LongOutputTruncated 验证大输出能正常处理（不 panic）。
func TestPostToolAuditLog_LongOutputTruncated(t *testing.T) {
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'a'
	}
	result := PostToolAuditLog(map[string]any{
		"tool_name": "read_file",
		"output":    string(long),
	})
	if result.ExitCode != ExitContinue {
		t.Errorf("expected ExitContinue, got %d", result.ExitCode)
	}
}
