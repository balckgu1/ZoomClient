package main

// 本文件实现 REPL 斜杠命令（/clear、/compact、/setmode、/selectmode、
// /models、/workspace、/session 等）的解析与处理逻辑。

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"zoomClient/model"
)

// handleSessionCommand handles commands shared across Web REPL modes.
// Returns true if the command signals session exit.
func handleSessionCommand(action string, s *AgentSession) bool {
	switch action {
	case "clear":
		if len(s.State.Messages) > 0 && s.State.Messages[0].Role == "system" {
			s.State.Messages = s.State.Messages[:1]
		} else {
			s.State.Messages = s.State.Messages[:0]
		}
		s.State.TurnCount = 0
		s.Em.EmitInfo("history cleared")
		// 历史已清空，推送最新占用快照刷新前端指示器
		emitContextUsageWeb(s, s.Pipeline.BuildSystemPrompt(), s.Registry.GetAll())
	case "compact":
		if len(s.State.Messages) <= 1 {
			s.Em.EmitInfo("no history to compact")
			return false
		}
		before := s.CompactManager.EstimateSize(s.State.Messages)
		newMsgs, cerr := s.CompactManager.CompactHistory(s.State.Messages)
		if cerr != nil {
			s.Em.EmitError("compact", cerr.Error())
			return false
		}
		s.State.Messages = newMsgs
		after := s.CompactManager.EstimateSize(newMsgs)
		s.Em.EmitCompact(before, after)
		// 压缩完成后推送最新占用快照刷新前端指示器
		emitContextUsageWeb(s, s.Pipeline.BuildSystemPrompt(), s.Registry.GetAll())
	case "exit":
		return true
	}
	return false
}

// handleSlashCommand handles REPL slash commands. Returns true to exit the main loop.
// Shared commands (clear, compact, exit) are delegated to handleSessionCommand.
func handleSlashCommand(input string, s *AgentSession) bool {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false
	}
	cmd := strings.ToLower(parts[0])
	// Delegate shared commands to handleSessionCommand (strip leading "/")
	switch cmd {
	case "/clear", "/compact", "/exit", "/quit":
		action := strings.TrimPrefix(cmd, "/")
		if action == "quit" {
			action = "exit"
		}
		return handleSessionCommand(action, s)
	}
	// Slash-only commands
	switch cmd {
	case "/setmode":
		handleSetMode(parts[1:], s)
	case "/selectmode":
		handleSelectMode(parts[1:], s)
	case "/models":
		handleListModels(s)
	case "/workspace":
		handleWorkspace(parts[1:], s)
	case "/session":
		handleSessionCmd(parts[1:], s)
	case "/help":
		s.Em.EmitInfo("/exit       - quit")
		s.Em.EmitInfo("/clear      - clear conversation history (system prompt kept)")
		s.Em.EmitInfo("/compact    - manually compact conversation history")
		s.Em.EmitInfo("/setmode    - add/update a model preset (-m name -t type -u url -k key [--model modelname])")
		s.Em.EmitInfo("/selectmode - switch to a configured model (-m name)")
		s.Em.EmitInfo("/models     - list all configured model presets")
		s.Em.EmitInfo("/workspace  - show or switch working directory (/workspace [path])")
		s.Em.EmitInfo("/session    - manage sessions: list | load <id> | new | delete <id> | rename <id> <title>")
		s.Em.EmitInfo("/help       - show this message")
	default:
		s.Em.EmitInfo("unknown command: " + input + "  (try /help)")
	}
	return false
}

// handleSetMode 处理 /setmode 命令：解析参数并注册模型预设。
func handleSetMode(args []string, s *AgentSession) {
	fs := flag.NewFlagSet("setmode", flag.ContinueOnError)
	var name, typ, baseURL, apiKey, modelName string
	fs.StringVar(&name, "m", "", "preset name")
	fs.StringVar(&typ, "t", "openai", "backend type: openai | ollama | anthropic | gemini")
	fs.StringVar(&baseURL, "u", "", "API base URL")
	fs.StringVar(&apiKey, "k", "", "API key")
	fs.StringVar(&modelName, "model", "", "actual model name (defaults to preset name)")
	if err := fs.Parse(args); err != nil {
		s.Em.EmitInfo("usage: /setmode -m <name> -t <type> -u <baseurl> -k <apikey> [--model <modelname>]")
		return
	}
	if name == "" {
		s.Em.EmitInfo("error: -m <name> is required")
		return
	}
	if modelName == "" {
		modelName = name
	}
	preset := &model.Preset{
		Name:      name,
		Type:      typ,
		BaseURL:   baseURL,
		APIKey:    apiKey,
		ModelName: modelName,
	}
	s.ModelRegistry.Add(preset)
	s.Em.EmitInfo(fmt.Sprintf("Model preset %q saved (type: %s, model: %s)", name, typ, modelName))
}

// handleSelectMode 处理 /selectmode 命令：切换到指定模型
func handleSelectMode(args []string, s *AgentSession) {
	fs := flag.NewFlagSet("selectmode", flag.ContinueOnError)
	var name string
	fs.StringVar(&name, "m", "", "preset name to switch to")
	if err := fs.Parse(args); err != nil {
		s.Em.EmitInfo("usage: /selectmode -m <name>")
		return
	}
	if name == "" {
		s.Em.EmitInfo("error: -m <name> is required")
		return
	}
	if err := s.SwitchModel(name); err != nil {
		s.Em.EmitInfo(fmt.Sprintf("switch failed: %s", err.Error()))
		return
	}
	s.Em.EmitInfo(fmt.Sprintf("Switched to model %q (%s)", name, s.ModelName))
}

// handleListModels 处理 /models 命令：列出所有已配置的模型预设。
func handleListModels(s *AgentSession) {
	presets := s.ModelRegistry.List()
	active := s.ModelRegistry.Active()
	if len(presets) == 0 {
		s.Em.EmitInfo("No model presets configured. Use /setmode to add one.")
		return
	}
	for _, p := range presets {
		marker := "  "
		if p.Name == active {
			marker = "* "
		}
		activeTag := ""
		if p.Name == active {
			activeTag = " [active]"
		}
		s.Em.EmitInfo(fmt.Sprintf("%s%s (%s, %s)%s", marker, p.Name, p.Type, p.ModelName, activeTag))
	}
}

// handleWorkspace 处理 /workspace 命令：切换工作目录
func handleWorkspace(args []string, s *AgentSession) {
	if len(args) == 0 {
		s.Em.EmitInfo(fmt.Sprintf("Current workspace: %s", s.ToolCtx.WorkPath))
		s.Em.EmitInfo("Usage: /workspace <path>  (e.g. /workspace /path/to/project)")
		return
	}

	targetPath := args[0]
	// 解析为绝对路径
	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		s.Em.EmitError("workspace", fmt.Sprintf("Failed to resolve path: %v", err))
		return
	}

	// 校验路径存在且为目录
	info, err := os.Stat(absPath)
	if err != nil {
		s.Em.EmitError("workspace", fmt.Sprintf("Path does not exist: %s", absPath))
		return
	}
	if !info.IsDir() {
		s.Em.EmitError("workspace", fmt.Sprintf("Path is not a directory: %s", absPath))
		return
	}

	// 更新 ToolContext 和 SystemPrompt
	oldPath := s.ToolCtx.WorkPath
	s.ToolCtx.WorkPath = absPath
	s.Pipeline.UpdateWorkDir(absPath)

	s.Em.EmitInfo(fmt.Sprintf("Workspace switched: %s → %s", oldPath, absPath))
}

// handleSessionCmd 处理 /session 子命令：list | load <id> | new | delete <id> | rename <id> <title>
func handleSessionCmd(args []string, s *AgentSession) {
	if s.SessionMgr == nil {
		s.Em.EmitInfo("Session manager not available")
		return
	}
	if len(args) == 0 {
		s.Em.EmitInfo("/session list              - list all sessions")
		s.Em.EmitInfo("/session load <id>         - load a session by ID")
		s.Em.EmitInfo("/session new               - create a new empty session")
		s.Em.EmitInfo("/session delete <id>       - delete a session")
		s.Em.EmitInfo("/session rename <id> <title> - rename a session")
		s.Em.EmitInfo("/session current           - show current session info")
		return
	}

	subcmd := strings.ToLower(args[0])
	switch subcmd {
	case "list":
		metas, err := s.SessionMgr.List()
		if err != nil {
			s.Em.EmitError("session", err.Error())
			return
		}
		if len(metas) == 0 {
			s.Em.EmitInfo("No saved sessions.")
			return
		}
		for _, meta := range metas {
			marker := "  "
			if meta.ID == s.SessionRecordID {
				marker = "* "
			}
			s.Em.EmitInfo(fmt.Sprintf("%s[%s] %s (%d turns, %s)",
				marker, meta.ID[:8]+"...", meta.Title,
				meta.TurnCount, meta.UpdatedAt.Format("2006-01-02 15:04")))
		}

	case "load":
		if len(args) < 2 {
			s.Em.EmitInfo("Usage: /session load <id>")
			return
		}
		id := args[1]
		record, err := s.SessionMgr.Load(id)
		if err != nil {
			s.Em.EmitError("session", fmt.Sprintf("load failed: %s", err.Error()))
			return
		}
		s.State.Messages = record.Messages
		s.State.TurnCount = record.TurnCount
		s.SessionRecordID = record.ID
		s.IsNewSession = false
		s.cachedTitle = record.Title
		s.Em.EmitInfo(fmt.Sprintf("Loaded session: %s (%d messages, %d turns)",
			record.Title, len(record.Messages), record.TurnCount))

	case "new":
		record := s.SessionMgr.CreateSession()
		s.State.Messages = nil
		s.State.TurnCount = 0
		s.SessionRecordID = record.ID
		s.IsNewSession = true
		s.cachedTitle = ""
		s.Em.EmitInfo(fmt.Sprintf("New session created: %s", record.ID[:8]+"..."))

	case "delete":
		if len(args) < 2 {
			s.Em.EmitInfo("Usage: /session delete <id>")
			return
		}
		id := args[1]
		if err := s.SessionMgr.Delete(id); err != nil {
			s.Em.EmitError("session", fmt.Sprintf("delete failed: %s", err.Error()))
			return
		}
		s.SessionRecordID = s.SessionMgr.Current()
		s.Em.EmitInfo(fmt.Sprintf("Session deleted: %s", id[:8]+"..."))

	case "rename":
		if len(args) < 3 {
			s.Em.EmitInfo("Usage: /session rename <id> <new title>")
			return
		}
		id := args[1]
		newTitle := strings.Join(args[2:], " ")
		if err := s.SessionMgr.Rename(id, newTitle); err != nil {
			s.Em.EmitError("session", fmt.Sprintf("rename failed: %s", err.Error()))
			return
		}
		if id == s.SessionRecordID {
			s.cachedTitle = newTitle
		}
		s.Em.EmitInfo(fmt.Sprintf("Session renamed to: %s", newTitle))

	case "current":
		s.Em.EmitInfo(fmt.Sprintf("Current session: %s (new: %v, turns: %d)",
			s.SessionRecordID[:8]+"...", s.IsNewSession, s.State.TurnCount))

	default:
		s.Em.EmitInfo("Unknown session command: " + subcmd)
		s.Em.EmitInfo("Try: /session list | load | new | delete | rename | current")
	}
}
