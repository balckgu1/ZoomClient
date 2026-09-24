package tools

// 本文件测试后台任务管理器的判定、启动、通知收集与混合执行逻辑。

import (
	"strings"
	"testing"
	"time"
)

// fakeBashTool 测试用：模拟 run_bash 工具，可配置执行耗时
type fakeBashTool struct {
	delay time.Duration // 每次调用的模拟耗时
}

func (f fakeBashTool) Name() string        { return "run_bash" }
func (f fakeBashTool) Description() string { return "fake bash for testing" }
func (f fakeBashTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object"}
}
func (f fakeBashTool) Call(args map[string]interface{}, ctx *ToolContext) ToolResult {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	return ToolResult{Ok: true, Content: "fake bash output"}
}

// fakeReadTool 测试用：模拟 read_file 快速同步工具
type fakeReadTool struct{}

func (fakeReadTool) Name() string        { return "read_file" }
func (fakeReadTool) Description() string { return "fake read for testing" }
func (fakeReadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object"}
}
func (fakeReadTool) Call(args map[string]interface{}, ctx *ToolContext) ToolResult {
	return ToolResult{Ok: true, Content: "file content"}
}

// newTestBashRegistry 构造含假 run_bash 的注册表
func newTestBashRegistry(delay time.Duration) *ToolRegister {
	registry := NewToolRegister()
	registry.Register(fakeBashTool{delay: delay})
	registry.Register(fakeReadTool{})
	return registry
}

// collectUntil 轮询收集后台通知，直到返回至少一条或超时
func collectUntil(t *testing.T, mgr *BackgroundTaskManager) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if notes := mgr.CollectCompleted(); len(notes) > 0 {
			return notes
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("后台任务未在期限内完成")
	return nil
}

// ===================== ShouldRunBackground 测试 =====================

// TestShouldRunBackground_显式参数优先：模型显式指定后台执行，即使命令不含慢关键词也应后台
func TestShouldRunBackground_显式参数优先(t *testing.T) {
	call := ToolCall{
		ID:        "call_1",
		Name:      "run_bash",
		Arguments: map[string]interface{}{"command": "echo hi", "run_in_background": true},
	}
	if !ShouldRunBackground(call) {
		t.Error("显式 run_in_background=true 时应返回 true")
	}
}

// TestShouldRunBackground_启发式关键词命中：命令含慢关键词时兜底判定为后台
func TestShouldRunBackground_启发式关键词命中(t *testing.T) {
	calls := []ToolCall{
		{ID: "c1", Name: "run_bash", Arguments: map[string]interface{}{"command": "npm install"}},
		{ID: "c2", Name: "run_bash", Arguments: map[string]interface{}{"command": "go build ./..."}},
		{ID: "c3", Name: "run_bash", Arguments: map[string]interface{}{"command": "pip install torch"}},
	}
	for _, call := range calls {
		if !ShouldRunBackground(call) {
			t.Errorf("命令 %q 命中慢关键词，应判定为后台", call.Arguments["command"])
		}
	}
}

// TestShouldRunBackground_普通命令不后台：快命令保持同步执行
func TestShouldRunBackground_普通命令不后台(t *testing.T) {
	call := ToolCall{
		ID:        "call_1",
		Name:      "run_bash",
		Arguments: map[string]interface{}{"command": "git status"},
	}
	if ShouldRunBackground(call) {
		t.Error("git status 是快命令，不应判定为后台")
	}
}

// TestShouldRunBackground_非Bash工具不后台：仅 run_bash 允许后台化
func TestShouldRunBackground_非Bash工具不后台(t *testing.T) {
	call := ToolCall{
		ID:        "call_1",
		Name:      "read_file",
		Arguments: map[string]interface{}{"filename": "a.txt", "run_in_background": true},
	}
	if ShouldRunBackground(call) {
		t.Error("read_file 不应支持后台执行")
	}
}

// TestShouldRunBackground_显式关闭优先级：run_in_background=false 且命令含慢关键词时，
// 模型显式意图为准，不进入后台
func TestShouldRunBackground_显式关闭优先级(t *testing.T) {
	call := ToolCall{
		ID:        "call_1",
		Name:      "run_bash",
		Arguments: map[string]interface{}{"command": "npm install", "run_in_background": false},
	}
	if ShouldRunBackground(call) {
		t.Error("显式 run_in_background=false 应覆盖启发式判定")
	}
}

// ===================== BackgroundTaskManager 测试 =====================

// TestBackgroundTaskManager_Start返回占位并异步完成：占位结果立即可得，任务最终完成并产出通知
func TestBackgroundTaskManager_Start返回占位并异步完成(t *testing.T) {
	registry := newTestBashRegistry(50 * time.Millisecond)
	mgr := NewBackgroundTaskManager(registry, &ToolContext{WorkPath: t.TempDir()})

	bgID, placeholder := mgr.Start(ToolCall{
		ID:        "toolu_1",
		Name:      "run_bash",
		Arguments: map[string]interface{}{"command": "npm install"},
	})
	if bgID == "" {
		t.Fatal("bgID 不应为空")
	}
	if !strings.Contains(placeholder.Content, bgID) {
		t.Errorf("占位结果应包含任务 ID %q，实际：%q", bgID, placeholder.Content)
	}
	if !placeholder.Ok {
		t.Error("占位结果应标记为成功")
	}

	notes := collectUntil(t, mgr)
	if len(notes) != 1 {
		t.Fatalf("应收集到 1 条通知，实际 %d 条", len(notes))
	}
	note := notes[0]
	for _, want := range []string{"<task_notification>", bgID, "completed", "npm install", "fake bash output"} {
		if !strings.Contains(note, want) {
			t.Errorf("通知应包含 %q，实际：%s", want, note)
		}
	}
}

// TestBackgroundTaskManager_CollectCompleted移除任务：通知只产出一次，重复收集为空
func TestBackgroundTaskManager_CollectCompleted移除任务(t *testing.T) {
	registry := newTestBashRegistry(10 * time.Millisecond)
	mgr := NewBackgroundTaskManager(registry, &ToolContext{WorkPath: t.TempDir()})

	mgr.Start(ToolCall{ID: "t1", Name: "run_bash", Arguments: map[string]interface{}{"command": "go build"}})
	collectUntil(t, mgr)

	// 任务已被移除，重复收集应返回空
	if notes := mgr.CollectCompleted(); len(notes) != 0 {
		t.Errorf("通知应只注入一次，重复收集得到 %d 条", len(notes))
	}
	if mgr.PendingCount() != 0 {
		t.Errorf("任务收集后 PendingCount 应为 0，实际 %d", mgr.PendingCount())
	}
}

// TestBackgroundTaskManager_ID递增：连续启动的任务 ID 序号递增
func TestBackgroundTaskManager_ID递增(t *testing.T) {
	registry := newTestBashRegistry(time.Hour) // 长延迟，保证任务始终未完成
	mgr := NewBackgroundTaskManager(registry, &ToolContext{WorkPath: t.TempDir()})

	id1, _ := mgr.Start(ToolCall{ID: "a", Name: "run_bash", Arguments: map[string]interface{}{"command": "npm install"}})
	id2, _ := mgr.Start(ToolCall{ID: "b", Name: "run_bash", Arguments: map[string]interface{}{"command": "npm install"}})
	if id1 == id2 {
		t.Errorf("两次启动的任务 ID 不应相同：%q", id1)
	}
	if id1 != "bg_0001" || id2 != "bg_0002" {
		t.Errorf("ID 应为 bg_0001 / bg_0002，实际 %q / %q", id1, id2)
	}
}

// TestBackgroundTaskManager_通知摘要截断：超长输出只保留前 200 字符
func TestBackgroundTaskManager_通知摘要截断(t *testing.T) {
	longContent := strings.Repeat("x", 500)
	registry := NewToolRegister()
	// 自定义假工具：直接返回超长输出
	registry.Register(longOutputTool{content: longContent})
	mgr := NewBackgroundTaskManager(registry, &ToolContext{WorkPath: t.TempDir()})

	_, placeholder := mgr.Start(ToolCall{ID: "t1", Name: "run_bash", Arguments: map[string]interface{}{"command": "npm install"}})
	if !placeholder.Ok {
		t.Fatal("占位结果应成功")
	}

	notes := collectUntil(t, mgr)
	summary := extractSummary(notes[0])
	if len(summary) > notificationSummaryLimit+3 {
		t.Errorf("摘要应被截断至 %d 字符左右，实际 %d", notificationSummaryLimit, len(summary))
	}
	if !strings.Contains(summary, "...") {
		t.Error("截断摘要应以 ... 结尾")
	}
}

// longOutputTool 测试用：模拟返回超长输出的 run_bash
type longOutputTool struct {
	content string
}

func (l longOutputTool) Name() string        { return "run_bash" }
func (l longOutputTool) Description() string { return "long output fake bash" }
func (l longOutputTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object"}
}
func (l longOutputTool) Call(args map[string]interface{}, ctx *ToolContext) ToolResult {
	return ToolResult{Ok: true, Content: l.content}
}

// extractSummary 从通知 XML 文本中提取 summary 标签内容
func extractSummary(notification string) string {
	start := strings.Index(notification, "<summary>")
	end := strings.Index(notification, "</summary>")
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	return notification[start+len("<summary>") : end]
}

// ===================== ExecuteWithBackground 测试 =====================

// TestExecuteWithBackground_混合前后台保持顺序：后台调用回填占位结果，同步调用正常执行，顺序稳定
func TestExecuteWithBackground_混合前后台保持顺序(t *testing.T) {
	registry := newTestBashRegistry(30 * time.Millisecond)
	mgr := NewBackgroundTaskManager(registry, &ToolContext{WorkPath: t.TempDir()})

	calls := []ToolCall{
		{ID: "c1", Name: "read_file", Arguments: map[string]interface{}{"filename": "a.txt"}},
		{ID: "c2", Name: "run_bash", Arguments: map[string]interface{}{"command": "npm install"}},
		{ID: "c3", Name: "read_file", Arguments: map[string]interface{}{"filename": "b.txt"}},
	}
	results := ExecuteWithBackground(calls, mgr, registry, &ToolContext{WorkPath: t.TempDir()})

	if len(results) != 3 {
		t.Fatalf("应返回 3 个结果，实际 %d", len(results))
	}
	// 顺序 0：同步 read_file 正常执行
	if results[0].Content != "file content" {
		t.Errorf("同步调用结果错误：%q", results[0].Content)
	}
	// 顺序 1：后台 run_bash 返回占位结果
	if !strings.Contains(results[1].Content, "Background task") {
		t.Errorf("后台调用应返回占位结果：%q", results[1].Content)
	}
	// 顺序 2：同步 read_file 正常执行
	if results[2].Content != "file content" {
		t.Errorf("同步调用结果错误：%q", results[2].Content)
	}

	// 后台任务最终完成并产出通知
	notes := collectUntil(t, mgr)
	if len(notes) != 1 {
		t.Fatalf("应收集到 1 条通知，实际 %d", len(notes))
	}
}

// TestExecuteWithBackground_管理器为nil时全同步：bgMgr 为空时退化为原有同步行为
func TestExecuteWithBackground_管理器为nil时全同步(t *testing.T) {
	registry := newTestBashRegistry(0)
	calls := []ToolCall{
		{ID: "c1", Name: "run_bash", Arguments: map[string]interface{}{"command": "npm install"}},
	}
	results := ExecuteWithBackground(calls, nil, registry, &ToolContext{WorkPath: t.TempDir()})

	if len(results) != 1 {
		t.Fatalf("应返回 1 个结果，实际 %d", len(results))
	}
	if results[0].Content != "fake bash output" {
		t.Errorf("bgMgr 为 nil 时应同步执行返回真实结果，实际 %q", results[0].Content)
	}
}
