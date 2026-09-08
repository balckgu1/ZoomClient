package main

// 本文件定义 AgentSession：聚合一次 Agent 运行所需的全部会话级依赖。

import (
	"zoomClient/clients"
	"zoomClient/compact"
	"zoomClient/emitter"
	"zoomClient/fsm"
	"zoomClient/hook"
	"zoomClient/model"
	"zoomClient/permission"
	"zoomClient/prompt"
	"zoomClient/session"
	"zoomClient/skills"
	"zoomClient/tools"
	"zoomClient/utils"
)

// AgentSession aggregates all session-level dependencies
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
	Em              emitter.Emitter
	PermissionMgr   *permission.Manager // 权限管理器
	SessionMgr      *session.Manager    // 会话持久化管理器
	SessionRecordID string              // 当前会话记录 ID
	IsNewSession    bool                // 是否为新建会话（用于触发自动命名）
	cachedTitle     string              // 缓存的会话标题，避免每帧读磁盘
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
