package main

// 本文件定义 AgentSession：聚合一次 Agent 运行所需的全部会话级依赖，
// 并实现 ui.AgentSessionBridge 接口，供 TUI 前端调用。

import (
	"fmt"
	"time"

	"zoomClient/clients"
	"zoomClient/compact"
	"zoomClient/emitter"
	"zoomClient/fsm"
	"zoomClient/hook"
	"zoomClient/logger"
	"zoomClient/model"
	"zoomClient/permission"
	"zoomClient/prompt"
	"zoomClient/session"
	"zoomClient/tools"
	"zoomClient/ui"
	"zoomClient/utils"

	"go.uber.org/zap"
)

// AgentSession aggregates all session-level dependencies
type AgentSession struct {
	State           *fsm.State
	Cfg             *utils.Config
	Client          clients.ChatClient
	ModelName       string
	ModelRegistry   *model.Registry
	Pipeline        *prompt.MessagePipeline
	Registry        *tools.ToolRegister
	ToolCtx         *tools.ToolContext
	TodoManager     *tools.TodoManager
	CompactManager  *compact.CompactManager
	HookRunner      *hook.Runner
	Em              emitter.Emitter
	PermissionMgr   *permission.Manager // 权限管理器（TUI 模式会替换其 Asker）
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

// GetPermissionManager 返回当前权限管理器（实现 AgentSessionBridge 接口）。
func (s *AgentSession) GetPermissionManager() *permission.Manager {
	return s.PermissionMgr
}

// RunAgentLoop runs agentLoop in a goroutine and sends EventAgentDone when finished.
func (s *AgentSession) RunAgentLoop(eventCh chan ui.UIEvent, userMessage string) {
	// Append user message to state
	s.State.Messages = append(s.State.Messages, fsm.Message{Role: "user", Content: userMessage})
	agentLoop(s, nil)
	eventCh <- ui.UIEvent{Type: ui.EventAgentDone}

	// Save session after each turn
	if s.SessionMgr != nil {
		record := &session.SessionRecord{
			ID:        s.SessionRecordID,
			Title:     "NewSession",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Model:     s.ModelName,
			TurnCount: s.State.TurnCount,
			Messages:  s.State.Messages,
		}
		if err := s.SessionMgr.Save(record); err != nil {
			logger.Log.Warn("save session failed", zap.Error(err))
		}
		// Generate title for new sessions (async)
		if s.IsNewSession {
			s.IsNewSession = false
			go func() {
				generatedTitle, err := s.SessionMgr.GenerateTitle(record)
				if err != nil {
					logger.Log.Warn("generate title failed", zap.Error(err))
					return
				}
				if generatedTitle != "" {
					record.Title = generatedTitle
					s.cachedTitle = generatedTitle
					if serr := s.SessionMgr.Save(record); serr != nil {
						logger.Log.Warn("save title failed", zap.Error(serr))
					}
					eventCh <- ui.UIEvent{Type: ui.EventInfo, Data: fmt.Sprintf("Session title: %s", generatedTitle)}
				}
			}()
		}
	}

	eventCh <- ui.UIEvent{Type: ui.EventTurnSeparator}
}

// SlashCommand handles a slash command input and returns "exit" if the program should quit.
func (s *AgentSession) SlashCommand(input string) string {
	if handleSlashCommand(input, s) {
		return "exit"
	}
	return ""
}

// GetModelName returns the current model name.
func (s *AgentSession) GetModelName() string { return s.ModelName }

// WorkDir returns the current working directory.
func (s *AgentSession) WorkDir() string { return s.ToolCtx.WorkPath }

// LogPath returns the log file path.
func (s *AgentSession) LogPath() string { return logger.LogFilePath }

// TurnCount returns the current turn count.
func (s *AgentSession) TurnCount() int { return s.State.TurnCount }

// SessionTitle returns the current session title (cached).
func (s *AgentSession) SessionTitle() string {
	if s.cachedTitle != "" {
		return s.cachedTitle
	}
	return "-"
}

// TokenEstimate returns a rough token estimate for the current conversation.
func (s *AgentSession) TokenEstimate() int {
	totalChars := 0
	for _, msg := range s.State.Messages {
		totalChars += len(messageContentToString(msg.Content))
		totalChars += len(msg.ReasoningContent)
		for _, tc := range msg.ToolCalls {
			totalChars += len(tc.Name)
			for _, v := range tc.Arguments {
				totalChars += len(fmt.Sprintf("%v", v))
			}
		}
	}
	// 粗略估算：4 字符 ≈ 1 token
	return totalChars / 4
}
