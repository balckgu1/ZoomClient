package main

// 本文件实现上下文窗口占用快照的组装与推送：
// contextUsageSnapshot 汇总 system prompt / skills / 工具 schema / 消息历史四部分字节占用，
// emitContextUsageWeb 仅在 web 模式下经 SseEmitter 推送给前端右下角指示器。

import (
	"zoomClient/clients"
	"zoomClient/compact"
	"zoomClient/tools"
	"zoomClient/web"
)

// contextUsageSnapshot 汇总当前会话的上下文窗口占用快照。
// systemPrompt 为已组装的完整 system prompt（含 skills 目录段），
// toolList 为当前注册的工具列表；skills 段字节数从 system prompt 中拆分单列。
func contextUsageSnapshot(s *AgentSession, systemPrompt string, toolList []tools.Tool) compact.UsageSnapshot {
	skillsSection := s.Pipeline.SkillsSection()
	skillsBytes := len(skillsSection)
	return s.CompactManager.ComputeUsage(compact.UsageParts{
		SystemPromptBytes: len(systemPrompt) - skillsBytes,
		SkillsBytes:       skillsBytes,
		SkillsCount:       s.Pipeline.SkillCount(),
		ToolsBytes:        clients.ToolsSchemaBytes(toolList),
	}, s.State.Messages)
}

// emitContextUsageWeb 组装快照并推送给 web 前端；非 web 模式（emitter 非 SseEmitter）时为 no-op。
func emitContextUsageWeb(s *AgentSession, systemPrompt string, toolList []tools.Tool) {
	if sseEm, ok := s.Em.(*web.SseEmitter); ok {
		sseEm.EmitContextUsage(contextUsageSnapshot(s, systemPrompt, toolList))
	}
}
