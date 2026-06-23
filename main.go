package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"time"
	"zoomClient/clients"
	"zoomClient/compact"
	"zoomClient/emitter"
	"zoomClient/fsm"
	"zoomClient/hook"
	"zoomClient/logger"
	"zoomClient/memory"
	"zoomClient/model"
	"zoomClient/permission"
	"zoomClient/prompt"
	"zoomClient/session"
	"zoomClient/skills"
	"zoomClient/subagent"
	"zoomClient/tools"
	"zoomClient/ui"
	"zoomClient/utils"
	"zoomClient/web"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AgentSession aggregates all session-level dependencies
type AgentSession struct {
	State          *fsm.State
	Cfg            *utils.Config
	Client         clients.ChatClient
	ModelName      string
	ModelRegistry  *model.Registry
	Pipeline       *prompt.MessagePipeline
	Registry       *tools.Registry
	ToolCtx        *tools.ToolContext
	TodoManager    *tools.TodoManager
	CompactManager *compact.CompactManager
	HookRunner     *hook.Runner
	Em             emitter.Emitter
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

// agentLoop is the main agent reasoning loop.
// stopCh is optional (nil for CLI/API mode). When closed, the loop aborts at the next safe point.
func agentLoop(s *AgentSession, stopCh <-chan struct{}) {
	cfg, client, state, model := s.Cfg, s.Client, s.State, s.ModelName
	pipeline, registry, toolCtx := s.Pipeline, s.Registry, s.ToolCtx
	todoManager, compactManager := s.TodoManager, s.CompactManager
	hookRunner, em := s.HookRunner, s.Em
	log := logger.Log

	// Get tool list
	toolList := registry.GetAll()

	// Infinite loop until no tool call or max turn count reached
	for {
		// Check for stop signal (web mode)
		select {
		case <-stopCh:
			em.EmitInfo("Generation stopped by user")
			em.EmitDone()
			return
		default:
		}
		// Context compact: micro-compact, replace older tool results with placeholders
		state.Messages = compactManager.MicroCompact(state.Messages)

		// Pipeline assemble: system prompt + state.messages + reminders + attachments
		payload := pipeline.AssemblePayload(state.Messages)
		// Add system prompt at the beginning of state.messages
		fullMessages := append([]fsm.Message{{Role: "system", Content: payload.SystemPrompt}}, payload.Messages...)

		// LLM chat
		response, err := client.Chat(model, fullMessages, toolList, map[string]interface{}{"temperature": 0.7})
		if err != nil {
			log.Error("call llm failed", zap.Error(err))
			em.EmitError("LLM", err.Error())
			break
		}

		// Clear OneShot reminder
		pipeline.ClearOneShotReminders()

		// Add the assistant's response to state.messages
		state.Messages = append(state.Messages, fsm.Message{
			Role:             "assistant",
			Content:          response.Message.Content,
			ToolCalls:        response.Message.ToolCalls,
			ReasoningContent: response.Message.ReasoningContent,
		})

		// Check if there are any tool calls, if not, render the reasoning+assistant text to user
		if len(response.Message.ToolCalls) == 0 {
			// Render reasoning
			if response.Message.ReasoningContent != "" {
				em.EmitReasoning(response.Message.ReasoningContent)
			}
			// Render assistant
			em.EmitAssistant(messageContentToString(response.Message.Content))
			em.EmitDone()
			state.TransitionReason = nil
			break
		}

		log.Info("Model requested tool call", zap.Int("tool count", len(response.Message.ToolCalls)))

		// If reasoning is not empty, render to user
		if response.Message.ReasoningContent != "" {
			em.EmitReasoning(response.Message.ReasoningContent)
		}

		// Batch tools based on concurrency safety
		toolCalls := response.Message.ToolCalls
		batches := tools.PartitionToolCalls(toolCalls)
		log.Info("Batch calling of tools", zap.Int("Batch number", len(batches)))
		for batchIndex, batch := range batches {
			batchToolNames := make([]string, 0, len(batch.Tools))
			for _, tracked := range batch.Tools {
				batchToolNames = append(batchToolNames, tracked.Name)
			}
			log.Info("Batch details", zap.Int("Batch number", batchIndex),
				zap.Bool("Is concurrency safe", batch.IsConcurrencySafe),
				zap.Strings("tool list", batchToolNames),
			)
		}

		// Render all tool calls
		for _, tc := range toolCalls {
			if tc.Function.Name == "sub_task" {
				prompt, _ := tc.Function.Arguments["prompt"].(string)
				em.EmitSubAgent(prompt)
				continue
			}
			em.EmitToolCall(tc.Function.Name, formatArgsPreview(tc.Function.Arguments))
		}

		// Check if the Todo tool was called in this round
		usedTodo := false
		for _, tc := range toolCalls {
			if tc.Function.Name == "todo" {
				usedTodo = true
				break
			}
		}

		// Hook: Trigger EventPreToolUse for each tool before tool execution
		preDecisions := make([]hook.HookResult, len(toolCalls))
		for i, tc := range toolCalls {
			preDecisions[i] = hookRunner.Run(hook.EventPreToolUse, map[string]any{
				"tool_name":       tc.Function.Name,
				"input":           tc.Function.Arguments,
				"call_index":      i,
				"max_tools":       cfg.AgentLoop.MaxTools,
				"tool_ctx":        toolCtx,
				"sensitive_files": cfg.AgentLoop.SensitiveFiles,
			})
			if preDecisions[i].ExitCode == hook.ExitInject && preDecisions[i].Message != "" {
				pipeline.AddReminder(prompt.Reminder{
					Content: preDecisions[i].Message,
					Source:  "pre_hook",
					OneShot: true,
				})
			}
			if preDecisions[i].ExitCode == hook.ExitBlock {
				em.EmitHookBlocked(tc.Function.Name, preDecisions[i].Message)
			}
		}

		// Execute all batches
		allowedCalls, allowedIndex := filterAllowedCalls(toolCalls, preDecisions)
		allowedBatches := tools.PartitionToolCalls(allowedCalls)
		allowedResults := tools.ExecuteBatches(allowedBatches, registry, toolCtx)
		results := mergeToolResults(toolCalls, preDecisions, allowedIndex, allowedResults)

		// Write Tool Result back to state.messages in the order of call
		for resultIndex, result := range results {
			log.Info("tool call finished",
				zap.String("tool name", toolCalls[resultIndex].Function.Name),
				zap.String("args", formatArgsPreview(toolCalls[resultIndex].Function.Arguments)),
				zap.String("result", result.Content),
			)

			// Render tool result summary
			if preDecisions[resultIndex].ExitCode != hook.ExitBlock {
				em.EmitToolResult(toolCalls[resultIndex].Function.Name, result.Content, result.IsError)
			}

			// If todo tool was called and succeeded, render the latest plan panel to user
			if toolCalls[resultIndex].Function.Name == "todo" && result.Ok {
				em.EmitTodoPanel(todoManager.Render())
			}

			// Context compact: Layer 1 (large output persistence): when a single tool result is too large, write full content to disk and keep only a preview in the message
			persistedContent := compactManager.PersistLargeOutput(toolCalls[resultIndex].ID, result.Content)
			if persistedContent != result.Content {
				log.Info("tool result content too large, persisted to disk and replaced with preview",
					zap.String("tool name", toolCalls[resultIndex].Function.Name),
					zap.Int("origin bytes", len(result.Content)),
				)
			}

			// Add toolCallID to state.messages
			state.Messages = append(state.Messages, fsm.Message{
				Role:       "tool",
				Content:    persistedContent,
				ToolCallID: toolCalls[resultIndex].ID,
			})
		}

		// Hook: PostToolUse
		runPostToolUseHooks(hookRunner, toolCalls, results, pipeline)

		// If the Todo tool is not used in this round, increase the count and inject a reminder through the pipeline when the threshold is exceeded
		if !usedTodo {
			todoManager.IncrementRoundsSinceUpdate()
			if reminder := todoManager.Reminder(cfg.AgentLoop.TodoRoundsThreshold); reminder != "" {
				log.Info("Plan has not been updated for a long time, injecting reminders",
					zap.Int("Rounds since update", cfg.AgentLoop.TodoRoundsThreshold),
				)
				pipeline.AddReminder(prompt.Reminder{
					Content: reminder,
					Source:  "todo",
					OneShot: true,
				})
			}
		}

		state.TurnCount++
		reason := "tool_result"
		state.TransitionReason = &reason

		// Full Context compact
		// Determine whether to trigger full compression after all tool results have been appended to state.messages
		// After successful compression, state.Messages will be replaced with system + a continuity summary
		if compactManager.ShouldAutoCompact(state.Messages) {
			beforeSize := compactManager.EstimateSize(state.Messages)
			newMessages, cerr := compactManager.CompactHistory(state.Messages)
			if cerr != nil {
				log.Warn("Complete compression failed, keep the original message history to continue", zap.String("session", toolCtx.SessionID), zap.Error(cerr))
			} else {
				afterSize := compactManager.EstimateSize(newMessages)
				log.Info("Complete compression completed",
					zap.String("Session", toolCtx.SessionID),
					zap.Int("Bytes before compression", beforeSize),
					zap.Int("Bytes after compression", afterSize),
					zap.Int("Message count", len(newMessages)),
				)
				em.EmitCompact(beforeSize, afterSize)
				state.Messages = newMessages
			}
		}

		// Limit the maximum number of rounds to avoid infinite loops
		if state.TurnCount >= cfg.AgentLoop.MaxTurns {
			log.Warn("Reaching the maximum round, stop the loop", zap.Int("max_turns", cfg.AgentLoop.MaxTurns))
			em.EmitInfo(fmt.Sprintf("reached max turns (%d), stop", cfg.AgentLoop.MaxTurns))
			break
		}
	}
}

// cliFlags holds parsed command-line flags.
type cliFlags struct {
	ModelType  string
	OutputMode string
	ConfigDir  string
	WebPort    int
}

func parseFlags() cliFlags {
	var f cliFlags
	flag.StringVar(&f.ModelType, "m", "openai", "Model backend type: ollama | openai | anthropic | gemini, default: openai")
	flag.StringVar(&f.OutputMode, "mode", "cli", "Output mode: cli (terminal) | api (NDJSON for Tauri sidecar) | web (browser UI)")
	flag.StringVar(&f.ConfigDir, "config-dir", "", "Config directory path (default: ./config)")
	flag.IntVar(&f.WebPort, "port", 8080, "Web server port (web mode only)")
	flag.Parse()
	return f
}

// initEmitter creates the appropriate emitter and optional session based on output mode.
func initEmitter(outputMode string) (emitter.Emitter, *ui.Renderer, *web.Session) {
	log := logger.Log
	switch strings.ToLower(outputMode) {
	case "api":
		return emitter.NewApiEmitter(os.Stdout), nil, nil
	case "web":
		sess := web.NewSession(uuid.NewString(), "")
		return web.NewSseEmitter(sess), nil, sess
	case "cli", "":
		v := ui.New()
		return v, v, nil
	default:
		log.Fatal("Unsupported output mode", zap.String("--mode", outputMode))
		return nil, nil, nil
	}
}

// initClient creates the ChatClient and resolves model name based on backend type.
func initClient(modelType string, cfg *utils.Config, em emitter.Emitter) (clients.ChatClient, string) {
	log := logger.Log
	switch strings.ToLower(modelType) {
	case "openai":
		apiKey := cfg.OpenAI.ApiKey
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
		if apiKey == "" {
			em.EmitError("config", "Please set the API key")
			log.Fatal("No API key OPENAI_API_KEY")
		}
		baseURL := cfg.OpenAI.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
		modelname := cfg.OpenAI.ModelName
		if modelname == "" {
			modelname = "gpt-4o"
		}
		client := clients.NewOpenAIClient(baseURL, apiKey)
		log.Info("OpenAI backend has been selected", zap.String("model", modelname), zap.String("base_url", baseURL))
		return client, modelname
	case "ollama", "":
		if cfg.Ollama.BaseURL == "" {
			log.Fatal("BaseURL is empty", zap.String("base_url", cfg.Ollama.BaseURL))
		}
		client := clients.NewOllamaClient(cfg.Ollama.BaseURL)
		if cfg.Ollama.ModelName == "" {
			log.Fatal("modelName is empty", zap.String("model_name", cfg.Ollama.ModelName))
		}
		log.Info("Olama backend has been selected", zap.String("model", cfg.Ollama.ModelName))
		return client, cfg.Ollama.ModelName
	case "anthropic":
		apiKey := cfg.Anthropic.ApiKey
		if apiKey == "" {
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
		if apiKey == "" {
			em.EmitError("config", "Please set the API key ANTHROPIC_API_KEY")
			log.Fatal("No API key ANTHROPIC_API_KEY")
		}
		client := clients.NewAnthropicClient(apiKey)
		modelname := cfg.Anthropic.ModelName
		if modelname == "" {
			modelname = "claude-opus-4-8"
		}
		log.Info("Anthropic backend has been selected", zap.String("model", modelname))
		return client, modelname
	case "gemini":
		apiKey := cfg.Gemini.ApiKey
		if apiKey == "" {
			apiKey = os.Getenv("GEMINI_API_KEY")
		}
		if apiKey == "" {
			em.EmitError("config", "Please set the API key GEMINI_API_KEY")
			log.Fatal("No API key GEMINI_API_KEY")
		}
		client := clients.NewGeminiClient(apiKey)
		modelname := cfg.Gemini.ModelName
		if modelname == "" {
			modelname = "gemini-3.5-flash"
		}
		log.Info("Gemini backend has been selected", zap.String("model", modelname))
		return client, modelname
	default:
		em.EmitError("config", "Unsupported model backend types: "+modelType)
		log.Fatal("Unsupported model backend types", zap.String("-m", modelType))
		return nil, ""
	}
}

// initTools creates the tool registry and registers all tools.
func initTools(cfg *utils.Config, client clients.ChatClient, modelname string,
	skillregistry *skills.SkillRegistry, toolCtx *tools.ToolContext, state *fsm.State,
	permitMgr *permission.Manager) (*tools.Registry, *tools.TodoManager, *compact.CompactManager) {
	log := logger.Log

	registry := tools.NewRegistry()

	// Register basic tools
	registry.Register(tools.WriteFileTool{})
	registry.Register(tools.EditFileTool{})
	registry.Register(tools.ReadFileTool{})
	registry.Register(tools.ListDirectory{})
	registry.Register(tools.RunBashTool{})
	registry.Register(tools.GlobSearch{})

	// Register load_skills tool
	registry.Register(skills.NewLoadSkillTool(skillregistry))

	// Register memory tools
	registry.Register(memory.NewSaveMemoryTool(cfg.Memory.Dir))
	registry.Register(memory.NewSearchMemoryTool(cfg.Memory.Dir))
	registry.Register(memory.NewUpdateMemoryTool(cfg.Memory.Dir))
	registry.Register(memory.NewDeleteMemoryTool(cfg.Memory.Dir))

	// Instantiate and register todo manager
	todoManager := tools.NewTodoManager()
	registry.Register(todoManager)

	// Instantiate and register compact manager
	compactManager := compact.NewCompactManager(compact.DefaultConfig(*cfg), client, modelname)
	registry.Register(compact.NewCompactTool(compactManager))

	// Create subagent and register sub_task tool
	options := map[string]interface{}{
		"temperature": cfg.Subagent.Temperature,
	}
	subAgent := subagent.NewSubAgent(client, modelname, cfg.Subagent.DefaultSystemPrompt, cfg.Subagent.ForkSubtaskPromptPrefix,
		subagent.BuildSubAgentRegistry(), toolCtx, cfg.Subagent.DefaultMaxTurns, options)

	subAgentRunner := func(prompt string, parentMessages []fsm.Message) (string, error) {
		if parentMessages == nil {
			return subAgent.Run(prompt)
		}
		return subAgent.RunWithFork(prompt, parentMessages)
	}
	parentMessagesProvider := func() []fsm.Message {
		return state.Messages
	}
	registry.Register(subagent.NewTaskTool(subAgentRunner, parentMessagesProvider))

	// Share permission policy with subagent's independent registry
	if subAgent.Registry != nil {
		subAgent.Registry.SetPermissionDecider(permitMgr.Decide)
	}

	log.Info("Registered tool list", zap.Any("tools", registry.GetAllNames()))
	return registry, todoManager, compactManager
}

// initPermissionManager creates the permission manager based on output mode.
func initPermissionManager(outputMode string, cfg *utils.Config, webSess *web.Session) *permission.Manager {
	var asker permission.Asker
	switch outputMode {
	case "api":
		asker = permission.NewApiAsker(os.Stdout)
	case "web":
		asker = web.NewWebAsker(webSess)
	default:
		asker = buildAsker(cfg.Permission.Interactive, false, nil)
	}
	return permission.NewManager(
		permission.Mode(cfg.Permission.Mode),
		buildPermissionRules(cfg.Permission.DenyRules),
		buildPermissionRules(cfg.Permission.AllowRules),
		asker,
	)
}

// handleSessionCommand handles commands shared across API and Web REPL modes.
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
	case "/help":
		s.Em.EmitInfo("/exit      - quit")
		s.Em.EmitInfo("/clear     - clear conversation history (system prompt kept)")
		s.Em.EmitInfo("/compact   - manually compact conversation history")
		s.Em.EmitInfo("/setmode   - add/update a model preset (-m name -t type -u url -k key [--model modelname])")
		s.Em.EmitInfo("/selectmode - switch to a configured model (-m name)")
		s.Em.EmitInfo("/models    - list all configured model presets")
		s.Em.EmitInfo("/help      - show this message")
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

// runAPIREPL runs the API mode REPL loop, reading NDJSON commands from stdin.
func runAPIREPL(ctx context.Context, s *AgentSession, webPort int) {
	log := logger.Log
	apiAsker := permission.NewApiAsker(os.Stdout)
	concurrentReader := emitter.NewConcurrentReader(os.Stdin, apiAsker)
	defer concurrentReader.Stop()
	go startHeartbeat(ctx, s.Em)

	for {
		cmd := concurrentReader.Next()
		if cmd == nil {
			log.Info("API mode stdin closed")
			break
		}
		// Validate payload before ACK
		var ackStatus string
		if cmd.Action == "chat" {
			payload, perr := emitter.ParseChatPayload(cmd)
			if perr != nil || payload.Message == "" {
				ackStatus = "rejected"
				if cmd.ID != "" {
					s.Em.EmitSystem("ack", map[string]string{"id": cmd.ID, "status": "rejected", "reason": "invalid or empty message"})
				}
				continue
			}
		}
		// Send ACK
		if cmd.ID != "" && ackStatus == "" {
			s.Em.EmitSystem("ack", map[string]string{"id": cmd.ID, "status": "accepted"})
		}
		switch cmd.Action {
		case "chat":
			payload, _ := emitter.ParseChatPayload(cmd) // already validated above
			s.Em.EmitEmotion("thinking", nil)
			s.State.Messages = append(s.State.Messages, fsm.Message{Role: "user", Content: payload.Message})
			agentLoop(s, nil)
		case "config":
			cfgPayload, _ := emitter.ParseConfigPayload(cmd)
			if cfgPayload.ModelType != "" {
				s.Em.EmitInfo("model_type change not supported at runtime, restart required")
			}
		default:
			if handleSessionCommand(cmd.Action, s) {
				return
			}
			if cmd.Action != "clear" && cmd.Action != "compact" {
				s.Em.EmitInfo("unknown action: " + cmd.Action)
			}
		}
	}
}

// runWebREPL starts the web server and runs the web mode REPL loop.
func runWebREPL(ctx context.Context, s *AgentSession, webSess *web.Session, webPort int, sessMgr *session.Manager) {
	log := logger.Log
	webServer := web.NewServer(webSess, sessMgr, s.ModelRegistry, webPort)
	// run http server
	go func() {
		log.Info("Web server starting", zap.String("addr", webServer.Addr()))
		if err := webServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Web server error", zap.Error(err))
		}
	}()
	// Open browser after short delay
	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(webServer.Addr())
	}()
	log.Info("Web UI available at", zap.String("url", webServer.Addr()))

	// Web REPL loop: read commands from CmdCh, with ctx cancellation support
	firstTurn := true
	for {
		select {
		case <-ctx.Done():
			log.Info("Received interrupt signal, shutting down web server...")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := webServer.Shutdown(shutdownCtx); err != nil {
				log.Warn("Web server shutdown error", zap.Error(err))
			}
			cancel()
			return

		case cmd, ok := <-webSess.CmdCh:
			if !ok {
				log.Info("Web session CmdCh closed")
				return
			}
			switch cmd.Action {
			case "chat":
				webSess.Busy.Store(true)
				s.Em.EmitEmotion("thinking", nil)
				s.State.Messages = append(s.State.Messages, fsm.Message{Role: "user", Content: cmd.Message})
				// Create a fresh stop channel for this generation
				webSess.StopCh = make(chan struct{})
				agentLoop(s, webSess.StopCh)
				webSess.Busy.Store(false)

				// Save session after each turn
				record := &session.SessionRecord{
					ID:        webSess.RecordID,
					Title:     "NewSession",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Model:     s.ModelName,
					TurnCount: s.State.TurnCount,
					Messages:  s.State.Messages,
				}
				if err := sessMgr.Save(record); err != nil {
					log.Warn("save session failed", zap.Error(err))
				}

				// Generate title after first turn
				if firstTurn {
					firstTurn = false
					go func() {
						title, err := sessMgr.GenerateTitle(record)
						if err != nil {
							log.Warn("generate title failed", zap.Error(err))
							return
						}
						if title != "" {
							record.Title = title
							if serr := sessMgr.Save(record); serr != nil {
								log.Warn("save title failed", zap.Error(serr))
							}
							// Push title update via SSE
							if sseEm, ok := s.Em.(*web.SseEmitter); ok {
								sseEm.EmitSessionRenamed(record.ID, title)
							}
						}
					}()
				}

			case "select_model":
				if err := s.SwitchModel(cmd.ModelName); err != nil {
					s.Em.EmitError("model", fmt.Sprintf("switch failed: %s", err.Error()))
				} else {
					s.Em.EmitInfo(fmt.Sprintf("Switched to model %q (%s)", cmd.ModelName, s.ModelName))
					if webSess != nil {
						webSess.Model = s.ModelName
					}
				}

			case "stop":
				// Close the stop channel to interrupt a running agentLoop
				if webSess.StopCh != nil {
					select {
					case <-webSess.StopCh:
						// already closed
					default:
						close(webSess.StopCh)
					}
				}

			default:
				if handleSessionCommand(cmd.Action, s) {
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					if err := webServer.Shutdown(shutdownCtx); err != nil {
						log.Warn("Web server shutdown error", zap.Error(err))
					}
					cancel()
					return
				}
			}
		}
	}
}

// runCLIREPL runs the CLI mode REPL loop, reading input from stdin.
func runCLIREPL(s *AgentSession, view *ui.Renderer) {
	for {
		input, ok := view.PromptUser()
		if !ok {
			s.Em.EmitInfo("EOF, exiting...")
			break
		}
		if input == "" {
			continue
		}
		// Slash command handling
		if strings.HasPrefix(input, "/") {
			if handleSlashCommand(input, s) {
				break // /exit
			}
			continue
		}
		// Append user message and run agentLoop
		s.State.Messages = append(s.State.Messages, fsm.Message{Role: "user", Content: input})
		agentLoop(s, nil)
		s.Em.EmitTurnSeparator()
	}
}

func main() {
	// Parse flags
	flags := parseFlags()

	// Initialize logger
	logger.Init()
	defer logger.Sync()
	log := logger.Log

	// Load Config
	utils.InitConfigWithDir(flags.ConfigDir)
	cfg := utils.GetConfig()

	// Context
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Initialize emitter & UI
	em, view, webSess := initEmitter(flags.OutputMode)

	// Initialize model client
	client, modelname := initClient(flags.ModelType, cfg, em)
	if webSess != nil {
		webSess.Model = modelname
	}

	// Initialize Model Registry
	configDir := flags.ConfigDir
	if configDir == "" {
		configDir = "./config"
	}
	modelRegistry := model.NewRegistry(configDir + "/models.yaml")
	// Register default presets from config.yaml (openai and ollama only)
	if cfg.OpenAI.ApiKey != "" || cfg.OpenAI.BaseURL != "" {
		modelRegistry.RegisterDefault(&model.Preset{
			Name:      "openai",
			Type:      "openai",
			BaseURL:   cfg.OpenAI.BaseURL,
			APIKey:    cfg.OpenAI.ApiKey,
			ModelName: cfg.OpenAI.ModelName,
		})
	}
	if cfg.Ollama.BaseURL != "" {
		modelRegistry.RegisterDefault(&model.Preset{
			Name:      "ollama",
			Type:      "ollama",
			BaseURL:   cfg.Ollama.BaseURL,
			ModelName: cfg.Ollama.ModelName,
		})
	}
	// Set active preset to current model type
	modelRegistry.SetActive(flags.ModelType)

	// Build tool context
	workDir, err := os.Getwd()
	if err != nil {
		log.Fatal("Failed to get working directory", zap.Error(err))
	}
	toolCtx := &tools.ToolContext{
		WorkPath:           workDir,
		Ctx:                ctx,
		DefaultBashTimeout: time.Duration(cfg.Tools.DefaultBashTimeout) * time.Second,
		Logger:             log.Named("tool"),
		SessionID:          uuid.NewString(),
		AppState:           map[string]any{"turn": 0},
	}

	// Load skills
	skillregistry, err := skills.NewSkillRegistry(cfg.Skills.Dir)
	if err != nil {
		log.Warn("Load skills failed, continue with empty registry", zap.Error(err))
		skillregistry, _ = skills.NewSkillRegistry("")
	}

	// Build system prompt pipeline
	promptBuilder := prompt.NewSystemPromptBuilder(skillregistry, cfg.Memory.Dir, modelname, toolCtx.WorkPath)
	pipeline := prompt.NewPipeline(promptBuilder)
	log.Info("MessagePipeline initialized")

	// Initialize Session state
	state := &fsm.State{Messages: []fsm.Message{}, TurnCount: 0}

	// Permission system
	permitMgr := initPermissionManager(flags.OutputMode, cfg, webSess)
	log.Info("Permission system has been enabled",
		zap.String("Mode", string(permitMgr.GetMode())),
		zap.Int("Deny rules", len(cfg.Permission.DenyRules)),
		zap.Int("Allow rules", len(cfg.Permission.AllowRules)),
	)

	// Register tools and subagent
	registry, todoManager, compactManager := initTools(cfg, client, modelname, skillregistry, toolCtx, state, permitMgr)
	registry.SetPermissionDecider(permitMgr.Decide)

	// Hook system
	hookRunner := buildHookRunner()
	log.Info("Hook system has been enabled")

	// Assemble session & start
	sess := &AgentSession{
		State: state, Cfg: cfg, Client: client, ModelName: modelname,
		ModelRegistry: modelRegistry,
		Pipeline:      pipeline, Registry: registry, ToolCtx: toolCtx,
		TodoManager: todoManager, CompactManager: compactManager,
		HookRunner: hookRunner, Em: em,
	}

	hookRunner.Run(hook.EventSessionStart, map[string]any{"model": modelname, "pipeline": "active"})
	em.EmitSessionStart(modelname, logger.LogFilePath)
	log.Info("Agent REPL start")

	// Initialize session manager (Web mode)
	var sessMgr *session.Manager
	if flags.OutputMode == "web" {
		var serr error
		sessMgr, serr = session.NewManager(cfg.Session.Dir, client, modelname)
		if serr != nil {
			log.Warn("Session manager init failed, history disabled", zap.Error(serr))
		} else {
			// Create initial session
			initialRecord := sessMgr.CreateSession()
			webSess.RecordID = initialRecord.ID
			log.Info("Session history enabled", zap.String("dir", cfg.Session.Dir))
		}
	}

	// Run REPL by mode
	switch flags.OutputMode {
	case "api":
		runAPIREPL(ctx, sess, flags.WebPort)
	case "web":
		runWebREPL(ctx, sess, webSess, flags.WebPort, sessMgr)
	default:
		runCLIREPL(sess, view)
	}

	// Session cleanup
	log.Info("Agent REPL End", zap.Int("total_turns", state.TurnCount))
	em.EmitSessionEnd(state.TurnCount)
	hookRunner.Run(hook.EventSessionEnd, map[string]any{"total_turns": state.TurnCount})
}

// startHeartbeat 周期性地输出心跳事件，用于前端检测 sidecar 存活状态。
// 该 goroutine 独立于 agentLoop，即使 LLM 调用阻塞也不会中断心跳。
func startHeartbeat(ctx context.Context, em emitter.Emitter) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			em.EmitSystem("heartbeat", map[string]string{
				"ts": fmt.Sprintf("%d", time.Now().Unix()),
			})
		}
	}
}

// messageContentToString safely converts fsm.Message.Content (interface{}) to a readable string.
func messageContentToString(content any) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// formatArgsPreview compresses tool call arguments into a one-line preview for frontend rendering.
func formatArgsPreview(args map[string]interface{}) string {
	if len(args) == 0 {
		return ""
	}
	b, err := json.Marshal(args)
	if err != nil {
		return fmt.Sprintf("%v", args)
	}
	s := string(b)
	const maxLen = 120
	if len(s) > maxLen {
		s = s[:maxLen] + "…"
	}
	return s
}

// buildPermissionRules converts the PermissionRuleConfig list in config to permission.Rule.
func buildPermissionRules(rawRules []utils.PermissionRuleConfig) []permission.Rule {
	rules := make([]permission.Rule, 0, len(rawRules))
	for _, raw := range rawRules {
		rules = append(rules, permission.Rule{
			Tool:     raw.Tool,
			Behavior: permission.Behavior(raw.Behavior),
			Path:     raw.Path,
			Content:  raw.Content,
		})
	}
	return rules
}

// buildAsker selects the interaction method when ask is triggered based on config.
//   - interactive=true + API mode   → ApiAsker（通过 NDJSON 协议与前端交互）
//   - interactive=true + CLI mode   → StdinAsker（终端文本交互）
//   - interactive=false             → DenyAsker（安全默认值）
func buildAsker(interactive bool, apiMode bool, w io.Writer) permission.Asker {
	if !interactive {
		return permission.DenyAsker{}
	}
	if apiMode {
		return permission.NewApiAsker(w)
	}
	return permission.NewStdinAsker()
}

// buildHookRunner constructs a hook runner
func buildHookRunner() *hook.Runner {
	runner := hook.NewRunner() // Build a new hook runner instance
	runner.Register(hook.EventSessionStart, hook.OnSessionStart)

	runner.Register(hook.EventPreToolUse, hook.PreToolBlockDangerous)
	runner.Register(hook.EventPreToolUse, hook.PreToolRateLimit)
	runner.Register(hook.EventPreToolUse, hook.PreToolSensitiveFileGuard)

	runner.Register(hook.EventPostToolUse, hook.PostToolAuditLog)
	runner.Register(hook.EventToolError, hook.OnToolErrorRecovery)

	runner.Register(hook.EventSessionEnd, hook.OnSessionEnd)
	return runner
}

// filterAllowedCalls filters out tool calls not blocked by hook, and preserves their mapping to original indices.
func filterAllowedCalls(toolCalls []tools.ToolCall, decisions []hook.HookResult) ([]tools.ToolCall, []int) {
	allowedCalls := make([]tools.ToolCall, 0, len(toolCalls))
	allowedIndex := make([]int, 0, len(toolCalls))
	for i, tc := range toolCalls {
		if decisions[i].ExitCode == hook.ExitBlock {
			continue
		}
		allowedCalls = append(allowedCalls, tc)
		allowedIndex = append(allowedIndex, i)
	}
	return allowedCalls, allowedIndex
}

// mergeToolResults merges execution results back in original order; positions blocked by hook are filled with block results.
func mergeToolResults(toolCalls []tools.ToolCall, decisions []hook.HookResult,
	allowedIndex []int, allowedResults []tools.ToolResult) []tools.ToolResult {
	results := make([]tools.ToolResult, len(toolCalls))

	for i, dec := range decisions {
		if dec.ExitCode == hook.ExitBlock {
			results[i] = tools.ToolResult{
				Ok:      false,
				IsError: true,
				Content: "<hook blocked> " + dec.Message,
			}
		}
	}
	for j, r := range allowedResults {
		results[allowedIndex[j]] = r
	}
	return results
}

// runPostToolUseHooks triggers PostToolUse event for each tool result.
// When exit=2, inject Message as OneShot reminder into pipeline.
func runPostToolUseHooks(runner *hook.Runner, toolCalls []tools.ToolCall, results []tools.ToolResult, pipeline *prompt.MessagePipeline) {
	for i, tc := range toolCalls {
		if results[i].IsError {
			errordecision := runner.Run(hook.EventToolError, map[string]any{
				"tool_name": tc.Function.Name,
				"input":     tc.Function.Arguments,
				"output":    results[i].Content,
			})
			if errordecision.ExitCode == hook.ExitInject && errordecision.Message != "" {
				pipeline.AddReminder(prompt.Reminder{
					Content: errordecision.Message,
					Source:  "post_hook",
					OneShot: true,
				})
			}
		}
		runner.Run(hook.EventPostToolUse, map[string]any{
			"tool_name": tc.Function.Name,
			"input":     tc.Function.Arguments,
			"output":    results[i].Content,
		})
	}
}

// openBrowser opens the specified URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		logger.Log.Warn("Failed to open browser", zap.String("url", url), zap.Error(err))
	}
}
