package main

// 本文件测试上下文占用快照的组装（contextUsageSnapshot）与 web 模式推送缓存链路
// （emitContextUsageWeb → SseEmitter → Session 缓存）。

import (
	"testing"

	"zoomClient/clients"
	"zoomClient/compact"
	"zoomClient/fsm"
	"zoomClient/tools"
	"zoomClient/web"
)

// stubUsageTool 最小 tools.Tool 实现，仅用于工具 schema 尺寸与注册表组装测试。
type stubUsageTool struct{ name string }

func (s stubUsageTool) Name() string        { return s.name }
func (s stubUsageTool) Description() string { return "desc-" + s.name }
func (s stubUsageTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object"}
}
func (s stubUsageTool) Call(args map[string]interface{}, ctx *tools.ToolContext) tools.ToolResult {
	return tools.ToolResult{Ok: true}
}

// newUsageTestSession 组装一个仅含占用统计所需依赖的 AgentSession（无 skills、单工具）。
func newUsageTestSession(t *testing.T) *AgentSession {
	t.Helper()
	registry := tools.NewToolRegister()
	registry.Register(stubUsageTool{name: "read_file"})
	cfg := &compact.CompactConfig{ContextLimit: 1000, PersistDir: t.TempDir()}
	return &AgentSession{
		State:          &fsm.State{Messages: []fsm.Message{{Role: "user", Content: "hi"}}},
		Pipeline:       newTestPipeline(),
		Registry:       registry,
		CompactManager: compact.NewCompactManager(cfg, nil, "test-model"),
	}
}

// TestContextUsageSnapshot_NoSkills 验证无 skills 时快照拆分与汇总：
// skills 段为 0、system prompt 全部计入主体、total 为四部分之和、limit 透传。
func TestContextUsageSnapshot_NoSkills(t *testing.T) {
	s := newUsageTestSession(t)
	systemPrompt := s.Pipeline.BuildSystemPrompt()

	snap := contextUsageSnapshot(s, systemPrompt, s.Registry.GetAll())

	if snap.SkillsBytes != 0 || snap.SkillsCount != 0 {
		t.Errorf("无 skills 时 skills 段与数量应为 0，实际 %d/%d", snap.SkillsBytes, snap.SkillsCount)
	}
	if snap.SystemPromptBytes != len(systemPrompt) {
		t.Errorf("SystemPromptBytes 应等于完整 system prompt 长度 %d，实际 %d", len(systemPrompt), snap.SystemPromptBytes)
	}
	wantTools := clients.ToolsSchemaBytes(s.Registry.GetAll())
	if snap.ToolsBytes != wantTools {
		t.Errorf("ToolsBytes 期望 %d，实际 %d", wantTools, snap.ToolsBytes)
	}
	wantMessages := s.CompactManager.EstimateSize(s.State.Messages)
	if snap.MessagesBytes != wantMessages {
		t.Errorf("MessagesBytes 期望 %d，实际 %d", wantMessages, snap.MessagesBytes)
	}
	wantTotal := len(systemPrompt) + wantTools + wantMessages
	if snap.TotalBytes != wantTotal {
		t.Errorf("TotalBytes 期望 %d，实际 %d", wantTotal, snap.TotalBytes)
	}
	if snap.LimitBytes != 1000 {
		t.Errorf("LimitBytes 应透传配置 1000，实际 %d", snap.LimitBytes)
	}
}

// TestEmitContextUsageWeb_NonWebEmitterNoop 非 web 模式（emitter 非 SseEmitter）推送应为 no-op 且不 panic。
func TestEmitContextUsageWeb_NonWebEmitterNoop(t *testing.T) {
	s := newUsageTestSession(t)
	s.Em = nil // CLI/api 模式断言失败路径
	emitContextUsageWeb(s, s.Pipeline.BuildSystemPrompt(), s.Registry.GetAll())
}

// TestEmitContextUsageWeb_CachesSnapshotInWebSession 验证 web 模式推送时同步写入 Session 缓存，
// 且事件经 system 通道发出（GET /api/context-usage 依赖该缓存）。
func TestEmitContextUsageWeb_CachesSnapshotInWebSession(t *testing.T) {
	s := newUsageTestSession(t)
	webSess := web.NewSession("sess-1", "test-model")
	s.Em = web.NewSseEmitter(webSess)

	systemPrompt := s.Pipeline.BuildSystemPrompt()
	want := contextUsageSnapshot(s, systemPrompt, s.Registry.GetAll())
	emitContextUsageWeb(s, systemPrompt, s.Registry.GetAll())

	cached := webSess.ContextUsage()
	if cached != want {
		t.Errorf("Session 缓存应与快照一致，期望 %+v，实际 %+v", want, cached)
	}

	select {
	case evt := <-webSess.EventCh:
		if evt.CH != "system" {
			t.Errorf("事件通道期望 system，实际 %s", evt.CH)
		}
		data, ok := evt.Data.(map[string]any)
		if !ok || data["event"] != "context_usage" {
			t.Errorf("事件应为 system/context_usage，实际 %+v", evt.Data)
		}
	default:
		t.Fatal("EventCh 中应有一条 context_usage 事件")
	}
}
