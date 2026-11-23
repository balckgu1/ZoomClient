package main

// 本文件定义 AgentSession：聚合一次 Agent 运行所需的全部会话级依赖。

import (
	"zoomClient/clients"
	"zoomClient/compact"
	"zoomClient/emitter"
	"zoomClient/fsm"
	"zoomClient/hook"
	"zoomClient/memory"
	"zoomClient/model"
	"zoomClient/permission"
	"zoomClient/prompt"
	"zoomClient/session"
	"zoomClient/skills"
	"zoomClient/tools"
	"zoomClient/utils"
)

// AgentSession 聚合一次 Agent 运行所需的全部会话级依赖。
type AgentSession struct {
	State           *fsm.State
	Cfg             *utils.Config
	Client          clients.ChatClient
	ModelName       string
	ModelRegistry   *model.Registry
	Pipeline        *prompt.MessagePipeline
	SkillRegistry   *skills.SkillRegistry // skill 注册表（Web 模式对外提供技能目录）
	Registry        *tools.ToolRegister
	ToolCtx         *tools.ToolContext
	TodoManager     *tools.TodoManager
	CompactManager  *compact.CompactManager
	HookRunner      *hook.Runner
	Temperature     float64
	ReasonLevel     string
	Em              emitter.Emitter
	PermissionMgr   *permission.Manager // 权限管理器
	SessionMgr      *session.Manager    // 会话持久化管理器
	SessionRecordID string              // 当前会话记录 ID
	IsNewSession    bool                // 是否为新建会话（用于触发自动命名）
	cachedTitle     string              // 缓存的会话标题，避免每帧读磁盘
	MemoryExtractor *memory.Extractor   // per-turn 同步记忆抽取器（nil 表示功能关闭）
}

// SwitchModel 热切换到指定模型预设，保留对话历史。
func (s *AgentSession) SwitchModel(name string) error {
	preset, err := s.ModelRegistry.Select(name)
	if err != nil {
		return err
	}
	client, modelName := model.BuildClient(preset)
	s.Client = client
	s.ModelName = modelName
	s.CompactManager.UpdateModel(client, modelName)
	s.Pipeline.UpdateModelName(modelName)
	return nil
}

// runMemoryExtraction 在回合结束时同步执行增量记忆抽取（阻塞直到完成或超时）。
// announce=true 时经 Extractor 推送“记忆更新中…”相位事件供前端展示；
// 斜杠命令路径传 false，仅保留 info 文本提示，避免相位状态无人清理而卡住。
func (s *AgentSession) runMemoryExtraction(announce bool) {
	if s.MemoryExtractor == nil {
		return
	}

	// 轻量 ToolContext 快照：只保留抽取工具所需的只读字段，避免子代理污染主上下文
	toolCtxSnap := tools.ToolContext{
		WorkPath:           s.ToolCtx.WorkPath,
		Ctx:                s.ToolCtx.Ctx,
		DefaultBashTimeout: s.ToolCtx.DefaultBashTimeout,
		Logger:             s.ToolCtx.Logger,
		SessionID:          s.ToolCtx.SessionID,
		AppState:           map[string]any{},
	}

	s.MemoryExtractor.Run(memory.ExtractionRequest{
		Messages: s.State.Messages,
		Client:   s.Client,
		Model:    s.ModelName,
		ToolCtx:  toolCtxSnap,
	}, announce)
}
