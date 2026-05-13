package compact

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zoomClient/clients"
	"zoomClient/fsm"
	"zoomClient/tools"
)

// ===================== 测试辅助 =====================

// stubChatClient 用于在不连真实模型的前提下测试第 3 层（整体摘要）。
// 可以控制返回的摘要文本或返回错误，同时记录最近一次收到的 messages 便于断言。
type stubChatClient struct {
	summaryContent string        // 模型要返回的摘要文本；为空则走 returnErr
	returnErr      error         // 模拟模型调用失败
	lastMessages   []fsm.Message // 记录最近一次收到的消息列表
	callCount      int           // 被调用的次数
}

// Chat 实现 clients.ChatClient 接口。
func (s *stubChatClient) Chat(model string, messages []fsm.Message, toolList []tools.Tool, options map[string]interface{}) (*clients.ChatResponse, error) {
	s.callCount++
	// 复制一份避免外部修改
	s.lastMessages = append([]fsm.Message{}, messages...)
	if s.returnErr != nil {
		return nil, s.returnErr
	}
	return &clients.ChatResponse{
		Model: model,
		Done:  true,
		Message: fsm.Message{
			Role:    "assistant",
			Content: s.summaryContent,
		},
	}, nil
}

// newTestManager 构造一个可自定义阈值 + 临时落盘目录的 CompactManager。
func newTestManager(t *testing.T, cfg *CompactConfig, client clients.ChatClient) *CompactManager {
	t.Helper()
	if cfg.PersistDir == "" {
		cfg.PersistDir = t.TempDir()
	}
	return NewCompactManager(cfg, client, "stub-model")
}

// defaultTestConfig 教学测试用的一组小阈值，便于在短输入下也能触发压缩。
func defaultTestConfig() *CompactConfig {
	return &CompactConfig{
		PersistThreshold:      100, // 100 字节就算"大输出"
		PreviewBytes:          20,
		KeepRecentToolResults: 3,
		ContextLimit:          500,
	}
}

// ===================== 第 1 层：PersistLargeOutput =====================

// TestPersistLargeOutput_SmallOutput_ReturnAsIs 小输出应原样返回，不落盘。
func TestPersistLargeOutput_SmallOutput_ReturnAsIs(t *testing.T) {
	cfg := defaultTestConfig()
	m := newTestManager(t, cfg, &stubChatClient{})

	small := "hello world"
	got := m.PersistLargeOutput("call_1", small)

	if got != small {
		t.Errorf("小输出应原样返回，期望 %q，实际 %q", small, got)
	}

	// 确认 PersistDir 没有落盘文件
	entries, _ := os.ReadDir(cfg.PersistDir)
	if len(entries) != 0 {
		t.Errorf("小输出不应落盘，但 PersistDir 下有 %d 个文件", len(entries))
	}
}

// TestPersistLargeOutput_LargeOutput_PersistAndPreview 大输出应写磁盘并返回预览占位。
func TestPersistLargeOutput_LargeOutput_PersistAndPreview(t *testing.T) {
	cfg := defaultTestConfig()
	m := newTestManager(t, cfg, &stubChatClient{})

	// 构造一个远超阈值的输出
	large := strings.Repeat("A", 500)
	got := m.PersistLargeOutput("call_big", large)

	// 返回的占位文本应按 s06 文档格式包裹
	if !strings.HasPrefix(got, "<persisted-output>\n") || !strings.HasSuffix(got, "</persisted-output>") {
		t.Errorf("返回文本应被 <persisted-output> 包裹，实际：%s", got)
	}
	if !strings.Contains(got, "Full output saved to:") {
		t.Errorf("占位文本应包含落盘路径提示，实际：%s", got)
	}

	// 预览部分应精确地是前 PreviewBytes 个字节
	expectedPreview := large[:cfg.PreviewBytes]
	if !strings.Contains(got, expectedPreview) {
		t.Errorf("占位文本应包含前 %d 字节的预览，实际：%s", cfg.PreviewBytes, got)
	}

	// 磁盘上应恰好有一个文件，内容 = 原始全文
	entries, err := os.ReadDir(cfg.PersistDir)
	if err != nil {
		t.Fatalf("读取 PersistDir 失败：%v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("应只落盘 1 个文件，实际 %d 个", len(entries))
	}
	// 文件名应以 toolUseID 开头
	if !strings.HasPrefix(entries[0].Name(), "call_big-") {
		t.Errorf("落盘文件名应以 toolUseID 为前缀，实际：%s", entries[0].Name())
	}

	diskContent, err := os.ReadFile(filepath.Join(cfg.PersistDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("读取落盘文件失败：%v", err)
	}
	if string(diskContent) != large {
		t.Errorf("落盘内容应等于原始全文，长度 expected=%d actual=%d", len(large), len(diskContent))
	}
}

// TestPersistLargeOutput_EmptyToolID_FallbackName 工具 ID 为空时应使用兜底命名。
func TestPersistLargeOutput_EmptyToolID_FallbackName(t *testing.T) {
	cfg := defaultTestConfig()
	m := newTestManager(t, cfg, &stubChatClient{})

	large := strings.Repeat("B", 300)
	_ = m.PersistLargeOutput("", large)

	entries, _ := os.ReadDir(cfg.PersistDir)
	if len(entries) != 1 {
		t.Fatalf("应落盘 1 个文件，实际 %d 个", len(entries))
	}
	// 兜底前缀是 "tool-"
	if !strings.HasPrefix(entries[0].Name(), "tool-") {
		t.Errorf("ID 为空时应使用 tool 兜底前缀，实际：%s", entries[0].Name())
	}
}

// ===================== 第 2 层：MicroCompact =====================

// TestMicroCompact_FewerThanKeep_NoChange 工具结果少于保留阈值时不应修改。
func TestMicroCompact_FewerThanKeep_NoChange(t *testing.T) {
	cfg := defaultTestConfig() // KeepRecentToolResults = 3
	m := newTestManager(t, cfg, &stubChatClient{})

	messages := []fsm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "q"},
		{Role: "assistant", Content: "a"},
		{Role: "tool", Content: "result-1", ToolCallID: "c1"},
		{Role: "tool", Content: "result-2", ToolCallID: "c2"},
	}

	got := m.MicroCompact(messages)

	// 只有 2 条 tool 消息，全部应保留原文
	if s, _ := got[3].Content.(string); s != "result-1" {
		t.Errorf("第一条 tool 结果不应被改写，实际：%v", got[3].Content)
	}
	if s, _ := got[4].Content.(string); s != "result-2" {
		t.Errorf("第二条 tool 结果不应被改写，实际：%v", got[4].Content)
	}
}

// TestMicroCompact_MoreThanKeep_ReplaceEarlier 工具结果超出保留阈值时仅保留最近 N 条。
func TestMicroCompact_MoreThanKeep_ReplaceEarlier(t *testing.T) {
	cfg := defaultTestConfig() // KeepRecentToolResults = 3
	m := newTestManager(t, cfg, &stubChatClient{})

	// 5 条 tool 消息：前 2 条应被占位替换，后 3 条保留
	messages := []fsm.Message{
		{Role: "system", Content: "sys"},
		{Role: "tool", Content: "old-1", ToolCallID: "c1"},
		{Role: "user", Content: "中间穿插一条 user"},
		{Role: "tool", Content: "old-2", ToolCallID: "c2"},
		{Role: "tool", Content: "keep-1", ToolCallID: "c3"},
		{Role: "assistant", Content: "assist"},
		{Role: "tool", Content: "keep-2", ToolCallID: "c4"},
		{Role: "tool", Content: "keep-3", ToolCallID: "c5"},
	}

	got := m.MicroCompact(messages)

	// 前 2 条被替换为占位
	if s, _ := got[1].Content.(string); s != microCompactPlaceholder {
		t.Errorf("old-1 应被替换为占位，实际：%v", got[1].Content)
	}
	if s, _ := got[3].Content.(string); s != microCompactPlaceholder {
		t.Errorf("old-2 应被替换为占位，实际：%v", got[3].Content)
	}

	// 最近 3 条保留原文
	if s, _ := got[4].Content.(string); s != "keep-1" {
		t.Errorf("keep-1 应保留原文，实际：%v", got[4].Content)
	}
	if s, _ := got[6].Content.(string); s != "keep-2" {
		t.Errorf("keep-2 应保留原文，实际：%v", got[6].Content)
	}
	if s, _ := got[7].Content.(string); s != "keep-3" {
		t.Errorf("keep-3 应保留原文，实际：%v", got[7].Content)
	}

	// 非 tool 角色消息不应被改动
	if got[0].Role != "system" || got[0].Content != "sys" {
		t.Errorf("system 消息不应被改动")
	}
	if got[5].Role != "assistant" || got[5].Content != "assist" {
		t.Errorf("assistant 消息不应被改动")
	}

	// ToolCallID 配对关系必须完整保留，否则后续 OpenAI 请求会被拒
	wantIDs := []string{"c1", "c2", "c3", "c4", "c5"}
	gotIDs := []string{got[1].ToolCallID, got[3].ToolCallID, got[4].ToolCallID, got[6].ToolCallID, got[7].ToolCallID}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Errorf("ToolCallID 在第 %d 条应为 %s，实际：%s", i, wantIDs[i], gotIDs[i])
		}
	}
}

// TestMicroCompact_Idempotent 已经是占位的 tool 消息再次调用 MicroCompact 应保持不变。
func TestMicroCompact_Idempotent(t *testing.T) {
	cfg := defaultTestConfig()
	m := newTestManager(t, cfg, &stubChatClient{})

	messages := []fsm.Message{
		{Role: "tool", Content: microCompactPlaceholder, ToolCallID: "c1"},
		{Role: "tool", Content: "old", ToolCallID: "c2"},
		{Role: "tool", Content: "keep-1", ToolCallID: "c3"},
		{Role: "tool", Content: "keep-2", ToolCallID: "c4"},
		{Role: "tool", Content: "keep-3", ToolCallID: "c5"},
	}

	got := m.MicroCompact(messages)
	got = m.MicroCompact(got) // 再跑一次应该幂等

	if s, _ := got[0].Content.(string); s != microCompactPlaceholder {
		t.Errorf("原占位应继续是占位")
	}
	if s, _ := got[1].Content.(string); s != microCompactPlaceholder {
		t.Errorf("old 应被替换为占位")
	}
	if s, _ := got[2].Content.(string); s != "keep-1" {
		t.Errorf("keep-1 应保留原文")
	}
}

// ===================== 大小估算与触发判断 =====================

// TestEstimateSize_IncludesAllFields 验证估算涵盖 role/content/reasoning/toolcalls。
func TestEstimateSize_IncludesAllFields(t *testing.T) {
	m := newTestManager(t, defaultTestConfig(), &stubChatClient{})

	messages := []fsm.Message{
		{Role: "user", Content: "hi"},
		{
			Role:             "assistant",
			Content:          "resp",
			ReasoningContent: "think",
			ToolCalls: []tools.ToolCall{
				{
					ID: "c1",
					Function: tools.ToolCallFunction{
						Name:      "read_file",
						Arguments: map[string]interface{}{"filename": "a.txt"},
					},
				},
			},
		},
	}

	size := m.EstimateSize(messages)
	if size <= 0 {
		t.Fatalf("估算大小应为正数，实际：%d", size)
	}
	// 至少应该比单纯 content 大（因为还算了 role/reasoning/toolcalls）
	if size < len("hi")+len("resp")+len("think") {
		t.Errorf("估算大小显著偏小：%d", size)
	}
}

// TestShouldAutoCompact_UnderLimit_False 上下文未超限且无手动请求应返回 false。
func TestShouldAutoCompact_UnderLimit_False(t *testing.T) {
	cfg := defaultTestConfig() // ContextLimit = 500
	m := newTestManager(t, cfg, &stubChatClient{})

	messages := []fsm.Message{{Role: "user", Content: "short"}}
	if m.ShouldAutoCompact(messages) {
		t.Error("上下文很小且无手动请求，不应触发压缩")
	}
}

// TestShouldAutoCompact_OverLimit_True 上下文超限应自动触发。
func TestShouldAutoCompact_OverLimit_True(t *testing.T) {
	cfg := defaultTestConfig() // ContextLimit = 500
	m := newTestManager(t, cfg, &stubChatClient{})

	messages := []fsm.Message{{Role: "user", Content: strings.Repeat("x", 600)}}
	if !m.ShouldAutoCompact(messages) {
		t.Error("上下文超过 ContextLimit 应触发压缩")
	}
}

// TestShouldAutoCompact_ManualRequest_True 手动标记应直接触发，不看大小。
func TestShouldAutoCompact_ManualRequest_True(t *testing.T) {
	m := newTestManager(t, defaultTestConfig(), &stubChatClient{})
	m.RequestManualCompact()

	messages := []fsm.Message{{Role: "user", Content: "short"}}
	if !m.ShouldAutoCompact(messages) {
		t.Error("手动请求后应触发压缩，无视上下文大小")
	}
}

// ===================== 第 3 层：CompactHistory =====================

// TestCompactHistory_Success_ReplaceWithSummary 成功压缩后应得到 system + 摘要消息。
func TestCompactHistory_Success_ReplaceWithSummary(t *testing.T) {
	stub := &stubChatClient{summaryContent: "SUMMARY: goal=X; files=a.go; next=run tests"}
	m := newTestManager(t, defaultTestConfig(), stub)

	messages := []fsm.Message{
		{Role: "system", Content: "you are helpful"},
		{Role: "user", Content: "帮我修 bug"},
		{Role: "assistant", Content: "好的"},
		{Role: "tool", Content: "read result", ToolCallID: "c1"},
	}

	got, err := m.CompactHistory(messages)
	if err != nil {
		t.Fatalf("压缩应成功，实际报错：%v", err)
	}
	if len(got) != 2 {
		t.Fatalf("压缩后应为 system + summary 共 2 条，实际 %d 条", len(got))
	}
	if got[0].Role != "system" || got[0].Content != "you are helpful" {
		t.Errorf("第一条应保留原 system 消息，实际：%+v", got[0])
	}
	if got[1].Role != "user" {
		t.Errorf("第二条应为 user 角色的摘要消息，实际：%s", got[1].Role)
	}
	if s, _ := got[1].Content.(string); !strings.Contains(s, "SUMMARY: goal=X") {
		t.Errorf("摘要消息应包含模型返回的摘要正文，实际：%v", got[1].Content)
	}
	if !strings.Contains(got[1].Content.(string), "compacted for continuity") {
		t.Errorf("摘要消息应包含连续性提示语，实际：%v", got[1].Content)
	}

	// State 应被更新
	if !m.State.HasCompacted {
		t.Error("压缩后 State.HasCompacted 应为 true")
	}
	if m.State.LastSummary == "" {
		t.Error("压缩后 State.LastSummary 不应为空")
	}

	// stub 应被调用一次，并且 summary 请求里的 user content 包含原对话的关键文本
	if stub.callCount != 1 {
		t.Errorf("模型应被调用 1 次，实际 %d", stub.callCount)
	}
	if len(stub.lastMessages) != 2 {
		t.Fatalf("摘要请求应为 [prompt-system, history-user] 2 条，实际 %d", len(stub.lastMessages))
	}
	if userHist, _ := stub.lastMessages[1].Content.(string); !strings.Contains(userHist, "帮我修 bug") {
		t.Errorf("摘要请求的历史内容应包含原 user 消息，实际：%v", stub.lastMessages[1].Content)
	}
}

// TestCompactHistory_ClientError_FallbackToOriginal 模型失败时应返回原消息与 error。
func TestCompactHistory_ClientError_FallbackToOriginal(t *testing.T) {
	stub := &stubChatClient{returnErr: fmt.Errorf("network down")}
	m := newTestManager(t, defaultTestConfig(), stub)

	original := []fsm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
	}

	got, err := m.CompactHistory(original)
	if err == nil {
		t.Fatal("模型失败时应返回 error")
	}
	if len(got) != len(original) {
		t.Errorf("失败时应原样返回 messages，长度 expected=%d actual=%d", len(original), len(got))
	}
	if m.State.HasCompacted {
		t.Error("失败时不应把 State.HasCompacted 置为 true")
	}
}

// TestCompactHistory_ConsumesManualFlag 执行一次压缩后，手动标记应被消费。
func TestCompactHistory_ConsumesManualFlag(t *testing.T) {
	stub := &stubChatClient{summaryContent: "ok"}
	m := newTestManager(t, defaultTestConfig(), stub)

	m.RequestManualCompact()
	if !m.pendingManualCompact {
		t.Fatal("RequestManualCompact 后标记应为 true")
	}

	_, err := m.CompactHistory([]fsm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("压缩应成功：%v", err)
	}
	if m.pendingManualCompact {
		t.Error("CompactHistory 执行后手动标记应被消费为 false")
	}
	// 再判断 ShouldAutoCompact，不应再因手动标记返回 true
	if m.ShouldAutoCompact([]fsm.Message{{Role: "user", Content: "short"}}) {
		t.Error("手动标记消费后不应再触发")
	}
}

// ===================== CompactTool =====================

// TestCompactTool_Call_MarksManualRequest 调用 compact 工具应标记手动压缩请求。
func TestCompactTool_Call_MarksManualRequest(t *testing.T) {
	m := newTestManager(t, defaultTestConfig(), &stubChatClient{})
	tool := NewCompactTool(m)

	if tool.Name() != "compact" {
		t.Errorf("工具名应为 compact，实际：%s", tool.Name())
	}

	result := tool.Call(map[string]interface{}{}, &tools.ToolContext{})
	if !result.Ok {
		t.Errorf("Call 应成功，实际：%+v", result)
	}
	if result.IsError {
		t.Error("Call 结果不应是错误")
	}
	if !m.pendingManualCompact {
		t.Error("Call 后 Manager 应标记 pendingManualCompact=true")
	}
}

// TestCompactTool_Call_TriggersCompactionViaShouldAutoCompact 验证工具调用可驱动后续自动压缩判断。
// 这条用例把"工具 → 标记 → 主循环判断"的接入链路走通，保证教学文档中"手动/自动复用同一条机制"成立。
func TestCompactTool_Call_TriggersCompactionViaShouldAutoCompact(t *testing.T) {
	m := newTestManager(t, defaultTestConfig(), &stubChatClient{summaryContent: "ok"})
	tool := NewCompactTool(m)

	shortMsgs := []fsm.Message{{Role: "user", Content: "短消息"}}
	if m.ShouldAutoCompact(shortMsgs) {
		t.Fatal("前置条件错误：短消息本不应触发压缩")
	}

	_ = tool.Call(map[string]interface{}{}, &tools.ToolContext{})

	if !m.ShouldAutoCompact(shortMsgs) {
		t.Error("工具调用后 ShouldAutoCompact 应返回 true")
	}
}
