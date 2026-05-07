package subagent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"zoomClient/clients"
	"zoomClient/fsm"
	"zoomClient/tools"
)

// =============================================================================
// 测试辅助：Mock ChatClient
// =============================================================================

// fakeChatClient 按预设序列依次返回响应或错误，用于在不依赖真实 LLM 的情况下驱动子智能体循环
type fakeChatClient struct {
	responses         []*clients.ChatResponse // 预设响应序列
	errors            []error                 // 预设错误序列（与 responses 同索引）
	calls             int                     // 已调用次数
	capturedMessages  [][]fsm.Message         // 每次调用时收到的 messages 快照（深拷贝）
	capturedToolLists [][]tools.Tool          // 每次调用时收到的 toolList 快照
}

// Chat 实现 clients.ChatClient 接口
// 对入参 messages 做深拷贝，避免子智能体后续 append 污染断言结果
func (f *fakeChatClient) Chat(model string, messages []fsm.Message, toolList []tools.Tool, options map[string]interface{}) (*clients.ChatResponse, error) {
	snapshot := make([]fsm.Message, len(messages))
	copy(snapshot, messages)
	f.capturedMessages = append(f.capturedMessages, snapshot)
	f.capturedToolLists = append(f.capturedToolLists, toolList)

	idx := f.calls
	f.calls++
	if idx < len(f.errors) && f.errors[idx] != nil {
		return nil, f.errors[idx]
	}
	if idx >= len(f.responses) {
		return nil, errors.New("fakeChatClient: 超出预设响应序列")
	}
	return f.responses[idx], nil
}

// finalResponse 构造"不含工具调用"的最终回复
func finalResponse(text string) *clients.ChatResponse {
	return &clients.ChatResponse{
		Message: fsm.Message{Role: "assistant", Content: text},
	}
}

// toolCallResponse 构造"含单个工具调用"的中间回复
func toolCallResponse(toolName string, args map[string]interface{}, id string) *clients.ChatResponse {
	return &clients.ChatResponse{
		Message: fsm.Message{
			Role:    "assistant",
			Content: "",
			ToolCalls: []tools.ToolCall{
				{
					ID: id,
					Function: tools.ToolCallFunction{
						Name:      toolName,
						Arguments: args,
					},
				},
			},
		},
	}
}

// newForkSubAgent 构造一个用于 fork 测试的子智能体
// 使用显式的 ForkSubtaskPromptPrefix 便于断言前缀是否生效
func newForkSubAgent(client clients.ChatClient, workPath string, maxTurns int) *SubAgent {
	return &SubAgent{
		Client:                  client,
		Model:                   "fake-model",
		SystemPrompt:            "test-system-prompt",
		ForkSubtaskPromptPrefix: "[FORK] ",
		Registry:                BuildSubAgentRegistry(),
		ToolCtx:                 &tools.ToolContext{WorkPath: workPath},
		MaxTurns:                maxTurns,
	}
}

// =============================================================================
// A. parseBoolArg 单元测试
// =============================================================================

// TestParseBoolArg_AllVariants 覆盖 bool / 字符串 / 缺失 / 非法类型 四类场景
func TestParseBoolArg_AllVariants(t *testing.T) {
	testCases := []struct {
		name     string
		args     map[string]any
		key      string
		expected bool
	}{
		{"原生布尔 true", map[string]any{"fork": true}, "fork", true},
		{"原生布尔 false", map[string]any{"fork": false}, "fork", false},
		{"字符串 true", map[string]any{"fork": "true"}, "fork", true},
		{"字符串 True（首字母大写）", map[string]any{"fork": "True"}, "fork", true},
		{"字符串 TRUE（全大写）", map[string]any{"fork": "TRUE"}, "fork", true},
		{"字符串 false", map[string]any{"fork": "false"}, "fork", false},
		{"字符串 yes（非法）", map[string]any{"fork": "yes"}, "fork", false},
		{"键缺失", map[string]any{}, "fork", false},
		{"非法类型：数字", map[string]any{"fork": 1}, "fork", false},
		{"非法类型：切片", map[string]any{"fork": []string{"true"}}, "fork", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := parseBoolArg(testCase.args, testCase.key)
			if got != testCase.expected {
				t.Errorf("parseBoolArg(%v, %q) = %v，期望 %v", testCase.args, testCase.key, got, testCase.expected)
			}
		})
	}
}

// =============================================================================
// B. SubAgent.RunWithFork 行为测试
// =============================================================================

// TestRunWithFork_InheritsParentAndAppendsForkPrompt
// 验证 fork 正常路径：父消息被完整继承（假设末尾不是 assistant），并在末尾追加带 prefix 的 user 消息
func TestRunWithFork_InheritsParentAndAppendsForkPrompt(t *testing.T) {
	fake := &fakeChatClient{
		responses: []*clients.ChatResponse{finalResponse("已完成子任务")},
	}
	sub := newForkSubAgent(fake, t.TempDir(), 3)

	parentMessages := []fsm.Message{
		{Role: "system", Content: "parent-system"},
		{Role: "user", Content: "parent-user-1"},
		{Role: "assistant", Content: "parent-assistant-1"},
		{Role: "user", Content: "parent-user-2"}, // 末尾是 user，不应被裁剪
	}

	summary, err := sub.RunWithFork("写测试", parentMessages)
	if err != nil {
		t.Fatalf("RunWithFork 不应报错：%v", err)
	}
	if summary != "已完成子任务" {
		t.Errorf("摘要应等于模型最终回复，实际：%s", summary)
	}

	// 断言第一次 Chat 收到的 messages：父 4 条全保留 + 1 条新 user
	if len(fake.capturedMessages) == 0 {
		t.Fatal("Chat 应被调用至少一次")
	}
	firstCall := fake.capturedMessages[0]
	if len(firstCall) != len(parentMessages)+1 {
		t.Fatalf("首轮消息数应为 %d，实际 %d", len(parentMessages)+1, len(firstCall))
	}

	// 前 4 条应原样继承
	for i, expected := range parentMessages {
		if firstCall[i].Role != expected.Role || firstCall[i].Content != expected.Content {
			t.Errorf("第 %d 条继承消息不一致：got=%+v, want=%+v", i, firstCall[i], expected)
		}
	}

	// 最后一条：role=user，内容带 prefix
	last := firstCall[len(firstCall)-1]
	if last.Role != "user" {
		t.Errorf("末尾消息 Role 应为 user，实际：%s", last.Role)
	}
	lastContent, _ := last.Content.(string)
	if !strings.HasPrefix(lastContent, "[FORK] ") || !strings.Contains(lastContent, "写测试") {
		t.Errorf("末尾消息应以 [FORK] 前缀开头并包含原 prompt，实际：%s", lastContent)
	}
}

// TestRunWithFork_TrimsTrailingAssistant
// 验证：父消息末尾是 assistant 时，会被裁剪掉（避免子智能体看见自己正在被调用的那一轮）
func TestRunWithFork_TrimsTrailingAssistant(t *testing.T) {
	fake := &fakeChatClient{
		responses: []*clients.ChatResponse{finalResponse("ok")},
	}
	sub := newForkSubAgent(fake, t.TempDir(), 3)

	parentMessages := []fsm.Message{
		{Role: "system", Content: "parent-system"},
		{Role: "user", Content: "parent-user"},
		{Role: "assistant", Content: "parent-assistant（触发 sub_task 的那一轮）"},
	}

	_, err := sub.RunWithFork("继续", parentMessages)
	if err != nil {
		t.Fatalf("RunWithFork 不应报错：%v", err)
	}

	// 期望：裁掉末尾 assistant 之后还剩 2 条 + 新 user = 3 条
	firstCall := fake.capturedMessages[0]
	if len(firstCall) != 3 {
		t.Fatalf("裁剪后应剩 3 条消息，实际 %d: %+v", len(firstCall), firstCall)
	}
	// 末尾 assistant 必须不存在
	for _, msg := range firstCall {
		if content, _ := msg.Content.(string); content == "parent-assistant（触发 sub_task 的那一轮）" {
			t.Errorf("末尾 assistant 应被裁剪，但仍存在：%+v", msg)
		}
	}
	// 新末尾应该是 fork 注入的 user
	last := firstCall[len(firstCall)-1]
	if last.Role != "user" {
		t.Errorf("新末尾 Role 应为 user，实际：%s", last.Role)
	}
}

// TestRunWithFork_KeepsTrailingTool
// 验证：父消息末尾是 tool（工具结果）时不被裁剪
func TestRunWithFork_KeepsTrailingTool(t *testing.T) {
	fake := &fakeChatClient{
		responses: []*clients.ChatResponse{finalResponse("ok")},
	}
	sub := newForkSubAgent(fake, t.TempDir(), 3)

	parentMessages := []fsm.Message{
		{Role: "user", Content: "u1"},
		{Role: "tool", Content: "tool-result", ToolCallID: "call_1"},
	}

	_, err := sub.RunWithFork("下一步", parentMessages)
	if err != nil {
		t.Fatalf("RunWithFork 不应报错：%v", err)
	}

	firstCall := fake.capturedMessages[0]
	// 期望：父 2 条全保留 + 新 user = 3 条
	if len(firstCall) != 3 {
		t.Fatalf("末尾是 tool 时不应裁剪，应剩 3 条，实际 %d", len(firstCall))
	}
	if firstCall[1].Role != "tool" {
		t.Errorf("tool 消息应被保留，实际第 2 条 Role=%s", firstCall[1].Role)
	}
}

// TestRunWithFork_DoesNotMutateParentMessages
// 验证隔离性：RunWithFork 不修改调用方传入的 parentMessages 切片
func TestRunWithFork_DoesNotMutateParentMessages(t *testing.T) {
	fake := &fakeChatClient{
		responses: []*clients.ChatResponse{finalResponse("ok")},
	}
	sub := newForkSubAgent(fake, t.TempDir(), 3)

	parentMessages := []fsm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"}, // 会被 fork 裁剪
	}
	originalLen := len(parentMessages)
	originalLastContent, _ := parentMessages[2].Content.(string)

	_, err := sub.RunWithFork("子任务", parentMessages)
	if err != nil {
		t.Fatalf("RunWithFork 不应报错：%v", err)
	}

	// 原切片长度与内容均不应改变
	if len(parentMessages) != originalLen {
		t.Errorf("父消息切片长度被修改：before=%d, after=%d", originalLen, len(parentMessages))
	}
	afterLastContent, _ := parentMessages[2].Content.(string)
	if afterLastContent != originalLastContent {
		t.Errorf("父消息末尾内容被修改：before=%q, after=%q", originalLastContent, afterLastContent)
	}
}

// TestRunWithFork_ReturnsErrorOnMissingDependencies
// 依赖校验：Client 或 Registry 为 nil 时应报错
func TestRunWithFork_ReturnsErrorOnMissingDependencies(t *testing.T) {
	parentMessages := []fsm.Message{{Role: "user", Content: "x"}}

	// 缺 Client
	subNoClient := &SubAgent{Registry: BuildSubAgentRegistry()}
	if _, err := subNoClient.RunWithFork("p", parentMessages); err == nil {
		t.Error("缺少 Client 应报错")
	}

	// 缺 Registry
	subNoRegistry := &SubAgent{Client: &fakeChatClient{}}
	if _, err := subNoRegistry.RunWithFork("p", parentMessages); err == nil {
		t.Error("缺少 Registry 应报错")
	}
}

// TestRunWithFork_ExecutesToolCallsInForkedContext
// 验证 fork 场景下工具调用链可以正常跑通
func TestRunWithFork_ExecutesToolCallsInForkedContext(t *testing.T) {
	// 准备临时文件供 read_file 读取
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "note.txt")
	if err := os.WriteFile(targetFile, []byte("fork-context-payload"), 0644); err != nil {
		t.Fatalf("写入临时文件失败：%v", err)
	}

	fake := &fakeChatClient{
		responses: []*clients.ChatResponse{
			toolCallResponse("read_file", map[string]interface{}{"filename": "note.txt"}, "c1"),
			finalResponse("文件内容：fork-context-payload"),
		},
	}
	sub := newForkSubAgent(fake, tmpDir, 5)

	parentMessages := []fsm.Message{
		{Role: "user", Content: "parent-user"},
	}

	summary, err := sub.RunWithFork("读 note.txt", parentMessages)
	if err != nil {
		t.Fatalf("RunWithFork 不应报错：%v", err)
	}
	if !strings.Contains(summary, "fork-context-payload") {
		t.Errorf("摘要应包含文件内容，实际：%s", summary)
	}
	if fake.calls != 2 {
		t.Errorf("应调用两次 LLM（工具 + 最终答复），实际：%d", fake.calls)
	}
}

// TestRunWithFork_HitsMaxTurns
// 验证 fork 场景下达到 MaxTurns 也能优雅截断
func TestRunWithFork_HitsMaxTurns(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatalf("写入临时文件失败：%v", err)
	}

	// 每轮都要求继续调工具，逼 subagent 达到 MaxTurns
	responses := make([]*clients.ChatResponse, 0, 5)
	for i := 0; i < 5; i++ {
		responses = append(responses, toolCallResponse(
			"read_file",
			map[string]interface{}{"filename": "a.txt"},
			fmt.Sprintf("c_%d", i),
		))
	}
	fake := &fakeChatClient{responses: responses}

	sub := newForkSubAgent(fake, tmpDir, 2)
	parentMessages := []fsm.Message{{Role: "user", Content: "p"}}

	summary, err := sub.RunWithFork("死循环读文件", parentMessages)
	if err != nil {
		t.Fatalf("达到 MaxTurns 应优雅退出而非报错：%v", err)
	}
	if !strings.Contains(summary, "truncated") {
		t.Errorf("摘要应含 truncated 提示，实际：%s", summary)
	}
	if fake.calls != 2 {
		t.Errorf("LLM 调用次数应等于 MaxTurns=2，实际：%d", fake.calls)
	}
}

// =============================================================================
// C. TaskTool.Call 的 fork 分支测试
// =============================================================================

// TestTaskTool_Call_ForkTrue_PassesParentMessagesToRunner
// 正常路径：fork=true 时 TaskTool 应从 provider 取父消息并传给 runner
func TestTaskTool_Call_ForkTrue_PassesParentMessagesToRunner(t *testing.T) {
	providerMessages := []fsm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u1"},
	}

	var capturedPrompt string
	var capturedParent []fsm.Message
	runner := func(prompt string, parentMessages []fsm.Message) (string, error) {
		capturedPrompt = prompt
		capturedParent = parentMessages
		return "ok-summary", nil
	}
	provider := func() []fsm.Message { return providerMessages }

	tool := NewTaskTool(runner, provider)
	result := tool.Call(map[string]any{"prompt": "子任务", "fork": true}, nil)

	if !result.Ok || result.IsError {
		t.Fatalf("fork=true 正常路径应返回成功，实际：%+v", result)
	}
	if capturedPrompt != "子任务" {
		t.Errorf("runner 收到的 prompt 应为 '子任务'，实际：%s", capturedPrompt)
	}
	if len(capturedParent) != len(providerMessages) {
		t.Errorf("runner 收到的 parentMessages 长度应为 %d，实际 %d", len(providerMessages), len(capturedParent))
	}
	if result.Content != "ok-summary" {
		t.Errorf("Content 应为 runner 返回值，实际：%s", result.Content)
	}
}

// TestTaskTool_Call_ForkFalseOrMissing_PassesNilParentMessages
// 默认路径：fork=false 或缺省时 runner 收到 nil parentMessages
func TestTaskTool_Call_ForkFalseOrMissing_PassesNilParentMessages(t *testing.T) {
	testCases := []struct {
		name string
		args map[string]any
	}{
		{"fork=false 显式", map[string]any{"prompt": "p", "fork": false}},
		{"fork 缺省", map[string]any{"prompt": "p"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var capturedParent []fsm.Message
			runner := func(prompt string, parentMessages []fsm.Message) (string, error) {
				capturedParent = parentMessages
				return "done", nil
			}
			// provider 给非空，但 fork=false 时不应被使用
			provider := func() []fsm.Message {
				return []fsm.Message{{Role: "user", Content: "should-not-be-used"}}
			}

			tool := NewTaskTool(runner, provider)
			result := tool.Call(testCase.args, nil)

			if !result.Ok {
				t.Fatalf("应返回成功，实际：%+v", result)
			}
			if capturedParent != nil {
				t.Errorf("runner 收到的 parentMessages 应为 nil，实际：%+v", capturedParent)
			}
		})
	}
}

// TestTaskTool_Call_ForkTrue_WithoutProvider_ReturnsError
// 异常：fork=true 但未配置 provider
func TestTaskTool_Call_ForkTrue_WithoutProvider_ReturnsError(t *testing.T) {
	runner := func(string, []fsm.Message) (string, error) { return "x", nil }

	tool := NewTaskTool(runner, nil) // provider 为 nil
	result := tool.Call(map[string]any{"prompt": "p", "fork": true}, nil)

	if result.Ok || !result.IsError {
		t.Fatalf("fork=true + provider=nil 应报错，实际：%+v", result)
	}
	if !strings.Contains(result.Content, "no parent messages provider") {
		t.Errorf("错误信息应提示 provider 缺失，实际：%s", result.Content)
	}
}

// TestTaskTool_Call_ForkTrue_EmptyParent_ReturnsError
// 异常：fork=true 但 provider 返回空切片
func TestTaskTool_Call_ForkTrue_EmptyParent_ReturnsError(t *testing.T) {
	runner := func(string, []fsm.Message) (string, error) { return "x", nil }
	provider := func() []fsm.Message { return []fsm.Message{} }

	tool := NewTaskTool(runner, provider)
	result := tool.Call(map[string]any{"prompt": "p", "fork": true}, nil)

	if result.Ok || !result.IsError {
		t.Fatalf("fork=true + 空父消息应报错，实际：%+v", result)
	}
	if !strings.Contains(result.Content, "parent messages are empty") {
		t.Errorf("错误信息应提示父消息为空，实际：%s", result.Content)
	}
}

// TestTaskTool_Call_ForkStringTrue_TreatedAsBool
// 兼容性：fork="true"（字符串）应被识别为 true
func TestTaskTool_Call_ForkStringTrue_TreatedAsBool(t *testing.T) {
	called := false
	runner := func(prompt string, parentMessages []fsm.Message) (string, error) {
		called = true
		if parentMessages == nil {
			t.Error("fork 字符串 'true' 应触发 fork 逻辑，runner 应收到非 nil parentMessages")
		}
		return "ok", nil
	}
	provider := func() []fsm.Message { return []fsm.Message{{Role: "user", Content: "x"}} }

	tool := NewTaskTool(runner, provider)
	result := tool.Call(map[string]any{"prompt": "p", "fork": "true"}, nil)

	if !result.Ok {
		t.Fatalf("应返回成功，实际：%+v", result)
	}
	if !called {
		t.Error("runner 应被调用")
	}
}

// TestTaskTool_Call_ForkInvalidType_TreatedAsFalse
// 兼容性：fork 为非法类型（数字）时退化为 false，runner 收到 nil parentMessages
func TestTaskTool_Call_ForkInvalidType_TreatedAsFalse(t *testing.T) {
	var capturedParent []fsm.Message
	runner := func(prompt string, parentMessages []fsm.Message) (string, error) {
		capturedParent = parentMessages
		return "ok", nil
	}
	provider := func() []fsm.Message { return []fsm.Message{{Role: "user", Content: "x"}} }

	tool := NewTaskTool(runner, provider)
	result := tool.Call(map[string]any{"prompt": "p", "fork": 123}, nil)

	if !result.Ok {
		t.Fatalf("应返回成功，实际：%+v", result)
	}
	if capturedParent != nil {
		t.Errorf("非法 fork 类型应退化为 false，runner 应收到 nil parentMessages，实际：%+v", capturedParent)
	}
}

// =============================================================================
// D. ParentMessagesProvider 闭包语义验证
// =============================================================================

// TestParentMessagesProvider_ClosureCapturesByReference
// 验证闭包对外层变量是"引用捕获"而非"值捕获"：
// provider 注册后，外层 state 变化，下一次调用 provider() 能拿到最新值
func TestParentMessagesProvider_ClosureCapturesByReference(t *testing.T) {
	// 模拟 main.go 中通过闭包暴露 state.Messages 的做法
	state := &fsm.State{
		Messages: []fsm.Message{{Role: "user", Content: "snapshot-v1"}},
	}
	provider := func() []fsm.Message { return state.Messages }

	// 首次调用：应拿到 v1
	first := provider()
	if len(first) != 1 || first[0].Content != "snapshot-v1" {
		t.Fatalf("首次快照不符合预期：%+v", first)
	}

	// 模拟 agentLoop 运行过程中往 state.Messages 追加新消息
	state.Messages = append(state.Messages, fsm.Message{Role: "assistant", Content: "snapshot-v2"})

	// 再次调用：应拿到 v2（证明 provider 是闭包，读的是引用）
	second := provider()
	if len(second) != 2 {
		t.Fatalf("第二次快照长度应为 2，实际 %d", len(second))
	}
	if second[1].Content != "snapshot-v2" {
		t.Errorf("闭包未读取到最新 state.Messages：%+v", second)
	}
}

// TestTaskTool_WithLiveProvider_ReadsLatestState
// 集成：TaskTool 多次触发 fork 时，每次都拿到最新的 state 快照
func TestTaskTool_WithLiveProvider_ReadsLatestState(t *testing.T) {
	state := &fsm.State{
		Messages: []fsm.Message{{Role: "user", Content: "round-1"}},
	}
	provider := func() []fsm.Message { return state.Messages }

	var observedLengths []int
	runner := func(prompt string, parentMessages []fsm.Message) (string, error) {
		observedLengths = append(observedLengths, len(parentMessages))
		return "ok", nil
	}

	tool := NewTaskTool(runner, provider)

	// 第一次调用：state 有 1 条
	_ = tool.Call(map[string]any{"prompt": "p1", "fork": true}, nil)
	// 模拟主循环追加消息
	state.Messages = append(state.Messages, fsm.Message{Role: "user", Content: "round-2"})
	// 第二次调用：state 有 2 条
	_ = tool.Call(map[string]any{"prompt": "p2", "fork": true}, nil)

	if len(observedLengths) != 2 {
		t.Fatalf("runner 应被调用两次，实际 %d", len(observedLengths))
	}
	if observedLengths[0] != 1 || observedLengths[1] != 2 {
		t.Errorf("两次 fork 观察到的父消息长度应为 [1,2]，实际 %v", observedLengths)
	}
}
