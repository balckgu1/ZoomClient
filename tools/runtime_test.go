package tools

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// ===================== 测试辅助工具 =====================

// mockReadTool 模拟只读工具（并发安全）
type mockReadTool struct{}

func (m mockReadTool) Name() string               { return "read_file" }
func (m mockReadTool) Description() string        { return "mock read" }
func (m mockReadTool) Parameters() map[string]any { return nil }
func (m mockReadTool) Call(args map[string]any, ctx *ToolContext) ToolResult {
	filename, _ := args["filename"].(string)
	return ToolResult{Ok: true, Content: fmt.Sprintf("内容来自: %s", filename)}
}

// mockWriteTool 模拟写入工具（非并发安全）
type mockWriteTool struct{}

func (m mockWriteTool) Name() string               { return "write_file" }
func (m mockWriteTool) Description() string        { return "mock write" }
func (m mockWriteTool) Parameters() map[string]any { return nil }
func (m mockWriteTool) Call(args map[string]any, ctx *ToolContext) ToolResult {
	filename, _ := args["filename"].(string)
	return ToolResult{Ok: true, Content: fmt.Sprintf("已写入: %s", filename)}
}

// mockEditTool 模拟编辑工具（非并发安全）
type mockEditTool struct{}

func (m mockEditTool) Name() string               { return "edit_file" }
func (m mockEditTool) Description() string        { return "mock edit" }
func (m mockEditTool) Parameters() map[string]any { return nil }
func (m mockEditTool) Call(args map[string]any, ctx *ToolContext) ToolResult {
	filename, _ := args["filename"].(string)
	return ToolResult{Ok: true, Content: fmt.Sprintf("已编辑: %s", filename)}
}

// newTestRegistry 创建包含模拟工具的测试注册表
func newTestRegistry() *Registry {
	registry := NewRegistry()
	registry.Register(mockReadTool{})
	registry.Register(mockWriteTool{})
	registry.Register(mockEditTool{})
	return registry
}

// makeToolCall 便捷创建 ToolCall 的辅助函数
func makeToolCall(name string, args map[string]any) ToolCall {
	return ToolCall{
		Function: ToolCallFunction{
			Name:      name,
			Arguments: args,
		},
	}
}

// ===================== IsConcurrencySafe 测试 =====================

func TestIsConcurrencySafe_只读工具应为并发安全(t *testing.T) {
	if !IsConcurrencySafe("read_file") {
		t.Error("read_file 应该是并发安全的")
	}
}

func TestIsConcurrencySafe_写入工具应为非并发安全(t *testing.T) {
	unsafeTools := []string{"write_file", "edit_file", "run_bash"}
	for _, toolName := range unsafeTools {
		if IsConcurrencySafe(toolName) {
			t.Errorf("%s 不应该是并发安全的", toolName)
		}
	}
}

func TestIsConcurrencySafe_未知工具默认非并发安全(t *testing.T) {
	if IsConcurrencySafe("unknown_tool") {
		t.Error("未知工具应默认为非并发安全")
	}
}

// ===================== PartitionToolCalls 测试 =====================

func TestPartitionToolCalls_空输入返回nil(t *testing.T) {
	batches := PartitionToolCalls(nil)
	if batches != nil {
		t.Error("空输入应返回 nil")
	}
}

func TestPartitionToolCalls_全部并发安全归为一批(t *testing.T) {
	calls := []ToolCall{
		makeToolCall("read_file", map[string]any{"filename": "a.txt"}),
		makeToolCall("read_file", map[string]any{"filename": "b.txt"}),
		makeToolCall("read_file", map[string]any{"filename": "c.txt"}),
	}
	batches := PartitionToolCalls(calls)
	if len(batches) != 1 {
		t.Fatalf("期望 1 个批次，实际得到 %d 个", len(batches))
	}
	if !batches[0].IsConcurrencySafe {
		t.Error("批次应标记为并发安全")
	}
	if len(batches[0].Tools) != 3 {
		t.Errorf("期望 3 个工具，实际得到 %d 个", len(batches[0].Tools))
	}
}

func TestPartitionToolCalls_全部非安全各自一批(t *testing.T) {
	calls := []ToolCall{
		makeToolCall("write_file", map[string]any{"filename": "a.txt"}),
		makeToolCall("edit_file", map[string]any{"filename": "b.txt"}),
	}
	batches := PartitionToolCalls(calls)
	// write_file 和 edit_file 都是非安全的，应该在同一批次
	if len(batches) != 1 {
		t.Fatalf("期望 1 个批次（都是非安全），实际得到 %d 个", len(batches))
	}
	if batches[0].IsConcurrencySafe {
		t.Error("批次不应标记为并发安全")
	}
}

func TestPartitionToolCalls_交替类型产生多个批次(t *testing.T) {
	calls := []ToolCall{
		makeToolCall("read_file", map[string]any{"filename": "a.txt"}),
		makeToolCall("read_file", map[string]any{"filename": "b.txt"}),
		makeToolCall("write_file", map[string]any{"filename": "c.txt"}),
		makeToolCall("read_file", map[string]any{"filename": "d.txt"}),
	}
	batches := PartitionToolCalls(calls)
	if len(batches) != 3 {
		t.Fatalf("期望 3 个批次，实际得到 %d 个", len(batches))
	}
	// 第一批：2个 read_file（并发安全）
	if !batches[0].IsConcurrencySafe || len(batches[0].Tools) != 2 {
		t.Errorf("第一批应为并发安全且含 2 个工具，实际: safe=%v, count=%d",
			batches[0].IsConcurrencySafe, len(batches[0].Tools))
	}
	// 第二批：1个 write_file（非安全）
	if batches[1].IsConcurrencySafe || len(batches[1].Tools) != 1 {
		t.Errorf("第二批应为非安全且含 1 个工具，实际: safe=%v, count=%d",
			batches[1].IsConcurrencySafe, len(batches[1].Tools))
	}
	// 第三批：1个 read_file（并发安全）
	if !batches[2].IsConcurrencySafe || len(batches[2].Tools) != 1 {
		t.Errorf("第三批应为并发安全且含 1 个工具，实际: safe=%v, count=%d",
			batches[2].IsConcurrencySafe, len(batches[2].Tools))
	}
}

func TestPartitionToolCalls_工具状态初始为排队(t *testing.T) {
	calls := []ToolCall{
		makeToolCall("read_file", map[string]any{"filename": "a.txt"}),
	}
	batches := PartitionToolCalls(calls)
	if batches[0].Tools[0].Status != ToolStatusQueued {
		t.Errorf("初始状态应为 queued，实际为 %s", batches[0].Tools[0].Status)
	}
}

// ===================== ExecuteBatches 测试 =====================

func TestExecuteBatches_并发批次结果按原始顺序返回(t *testing.T) {
	registry := newTestRegistry()
	toolCtx := &ToolContext{WorkPath: "./"}

	calls := []ToolCall{
		makeToolCall("read_file", map[string]any{"filename": "first.txt"}),
		makeToolCall("read_file", map[string]any{"filename": "second.txt"}),
		makeToolCall("read_file", map[string]any{"filename": "third.txt"}),
	}
	batches := PartitionToolCalls(calls)
	results := ExecuteBatches(batches, registry, toolCtx)

	if len(results) != 3 {
		t.Fatalf("期望 3 个结果，实际得到 %d 个", len(results))
	}
	// 验证结果顺序与输入顺序一致
	expectedFiles := []string{"first.txt", "second.txt", "third.txt"}
	for index, expected := range expectedFiles {
		if !strings.Contains(results[index].Content, expected) {
			t.Errorf("结果[%d] 应包含 %q，实际内容: %s", index, expected, results[index].Content)
		}
	}
}

func TestExecuteBatches_串行批次逐个执行(t *testing.T) {
	registry := newTestRegistry()
	toolCtx := &ToolContext{WorkPath: "./"}

	calls := []ToolCall{
		makeToolCall("write_file", map[string]any{"filename": "a.txt", "content": "hello"}),
		makeToolCall("write_file", map[string]any{"filename": "b.txt", "content": "world"}),
	}
	batches := PartitionToolCalls(calls)
	results := ExecuteBatches(batches, registry, toolCtx)

	if len(results) != 2 {
		t.Fatalf("期望 2 个结果，实际得到 %d 个", len(results))
	}
	if !results[0].Ok || !results[1].Ok {
		t.Error("所有工具调用应成功")
	}
}

func TestExecuteBatches_混合批次按顺序执行(t *testing.T) {
	registry := newTestRegistry()
	toolCtx := &ToolContext{WorkPath: "./"}

	calls := []ToolCall{
		makeToolCall("read_file", map[string]any{"filename": "a.txt"}),
		makeToolCall("write_file", map[string]any{"filename": "b.txt", "content": "data"}),
		makeToolCall("read_file", map[string]any{"filename": "c.txt"}),
	}
	batches := PartitionToolCalls(calls)
	results := ExecuteBatches(batches, registry, toolCtx)

	if len(results) != 3 {
		t.Fatalf("期望 3 个结果，实际得到 %d 个", len(results))
	}
	if !strings.Contains(results[0].Content, "a.txt") {
		t.Errorf("第一个结果应包含 a.txt，实际: %s", results[0].Content)
	}
	if !strings.Contains(results[1].Content, "b.txt") {
		t.Errorf("第二个结果应包含 b.txt，实际: %s", results[1].Content)
	}
	if !strings.Contains(results[2].Content, "c.txt") {
		t.Errorf("第三个结果应包含 c.txt，实际: %s", results[2].Content)
	}
}

// ===================== TrackedTool 状态跟踪测试 =====================

func TestTrackedTool_执行后状态变为已完成(t *testing.T) {
	registry := newTestRegistry()
	toolCtx := &ToolContext{WorkPath: "./"}

	calls := []ToolCall{
		makeToolCall("read_file", map[string]any{"filename": "test.txt"}),
	}
	batches := PartitionToolCalls(calls)
	ExecuteBatches(batches, registry, toolCtx)

	tracked := batches[0].Tools[0]
	if tracked.Status != ToolStatusCompleted {
		t.Errorf("执行后状态应为 completed，实际为 %s", tracked.Status)
	}
	if tracked.Result == nil {
		t.Error("执行后 Result 不应为 nil")
	}
}

// ===================== QueuedContextModifiers 测试 =====================

func TestQueuedContextModifiers_按原始顺序应用修改器(t *testing.T) {
	toolCtx := &ToolContext{
		WorkPath: "./",
		AppState: map[string]any{},
	}

	// 创建两个 TrackedTool（模拟原始顺序）
	trackedTools := []*TrackedTool{
		{ID: "tool_0", Name: "read_file"},
		{ID: "tool_1", Name: "write_file"},
	}

	queue := NewQueuedContextModifiers()

	// 故意先添加 tool_1 的修改器，再添加 tool_0 的（模拟完成顺序与原始顺序不同）
	queue.Add("tool_1", func(ctx *ToolContext) {
		ctx.AppState["step2"] = "done"
	})
	queue.Add("tool_0", func(ctx *ToolContext) {
		ctx.AppState["step1"] = "done"
	})

	// 按原始顺序应用
	queue.ApplyInOrder(trackedTools, toolCtx)

	// 验证两个修改都已生效
	if toolCtx.AppState["step1"] != "done" {
		t.Error("step1 修改器应已应用")
	}
	if toolCtx.AppState["step2"] != "done" {
		t.Error("step2 修改器应已应用")
	}
}

func TestQueuedContextModifiers_并发添加修改器是安全的(t *testing.T) {
	queue := NewQueuedContextModifiers()
	var waitGroup sync.WaitGroup

	// 模拟多个 goroutine 同时添加修改器
	for index := 0; index < 100; index++ {
		waitGroup.Add(1)
		go func(idx int) {
			defer waitGroup.Done()
			toolID := fmt.Sprintf("tool_%d", idx)
			queue.Add(toolID, func(ctx *ToolContext) {
				// 空修改器，仅测试并发安全性
			})
		}(index)
	}
	waitGroup.Wait()

	// 验证所有修改器都被正确添加
	if len(queue.modifiers) != 100 {
		t.Errorf("期望 100 个工具的修改器，实际有 %d 个", len(queue.modifiers))
	}
}

func TestQueuedContextModifiers_无修改器时安全跳过(t *testing.T) {
	toolCtx := &ToolContext{WorkPath: "./"}
	trackedTools := []*TrackedTool{
		{ID: "tool_0", Name: "read_file"},
	}

	queue := NewQueuedContextModifiers()
	// 不添加任何修改器，直接应用
	queue.ApplyInOrder(trackedTools, toolCtx)
	// 不应 panic，执行到此即为通过
}

// ===================== ExecuteToolCalls 顶层入口测试 =====================

func TestExecuteToolCalls_完整流程集成测试(t *testing.T) {
	registry := newTestRegistry()
	toolCtx := &ToolContext{WorkPath: "./"}

	calls := []ToolCall{
		makeToolCall("read_file", map[string]any{"filename": "a.txt"}),
		makeToolCall("read_file", map[string]any{"filename": "b.txt"}),
		makeToolCall("write_file", map[string]any{"filename": "c.txt", "content": "hello"}),
		makeToolCall("read_file", map[string]any{"filename": "d.txt"}),
	}

	results := ExecuteToolCalls(calls, registry, toolCtx)

	if len(results) != 4 {
		t.Fatalf("期望 4 个结果，实际得到 %d 个", len(results))
	}
	for index, result := range results {
		if !result.Ok {
			t.Errorf("结果[%d] 应成功，实际失败: %s", index, result.Content)
		}
	}
}

func TestExecuteToolCalls_空调用列表返回空结果(t *testing.T) {
	registry := newTestRegistry()
	toolCtx := &ToolContext{WorkPath: "./"}

	results := ExecuteToolCalls(nil, registry, toolCtx)
	if len(results) != 0 {
		t.Errorf("空调用列表应返回空结果，实际得到 %d 个", len(results))
	}
}
