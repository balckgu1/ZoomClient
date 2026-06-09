package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"
	"zoomClient/clients"
	"zoomClient/compact"
	"zoomClient/emitter"
	"zoomClient/fsm"
	"zoomClient/hook"
	"zoomClient/logger"
	"zoomClient/memory"
	"zoomClient/permission"
	"zoomClient/prompt"
	"zoomClient/skills"
	"zoomClient/subagent"
	"zoomClient/tools"
	"zoomClient/ui"
	"zoomClient/utils"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// agentLoop is the main agent reasoning loop.
func agentLoop(cfg *utils.Config, client clients.ChatClient, state *fsm.State, model string,
	pipeline *prompt.MessagePipeline, registry *tools.Registry, toolCtx *tools.ToolContext, todoManager *tools.TodoManager,
	compactManager *compact.CompactManager, hookRunner *hook.Runner, em emitter.Emitter) {
	log := logger.Log

	// Get tool list
	toolList := registry.GetAll()

	// Infinite loop until no tool call or max turn count reached
	for {
		// Context compact: Layer 2 (micro-compact): replace older tool results with placeholders
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
					zap.Int("Rounds since update", todoManager.PlanningState.RoundsSinceUpdate),
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

		// Context compact (full summary)
		// Determine whether to trigger full compression after all tool results have been appended to state.messages
		// Trigger conditions:
		//   1) The model/user called the compact tool, marking pendingManualCompact;
		//   2) The estimated total context size exceeds Config.ContextLimit
		// After successful compression, state.Messages will be replaced with system + a continuity summary
		if compactManager.ShouldAutoCompact(state.Messages) {
			beforeSize := compactManager.EstimateSize(state.Messages)
			newMessages, cerr := compactManager.CompactHistory(state.Messages)
			if cerr != nil {
				log.Warn("Complete compression failed, keep the original message history to continue", zap.Error(cerr))
			} else {
				afterSize := compactManager.EstimateSize(newMessages)
				log.Info("Complete compression completed",
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

func main() {
	// Initialize global logger
	logger.Init()
	defer logger.Sync()
	log := logger.Log

	// Parse command line parameter: -m specifies the backend type of the model (default openai)
	var modelType string
	flag.StringVar(&modelType, "m", "openai", "Model backend type: ollama | openai | anthropic | gemini, default: openai")
	var outputMode string
	flag.StringVar(&outputMode, "mode", "cli", "Output mode: cli (terminal) | api (NDJSON for Tauri sidecar)")
	var configDir string
	flag.StringVar(&configDir, "config-dir", "", "Config directory path (default: ./config)")
	flag.Parse()

	// Read configuration file
	utils.InitConfigWithDir(configDir)
	cfg := utils.GetConfig()
	apiKey := ""

	// Create a context with signal monitoring
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Initialize front-end emitter based on mode
	var em emitter.Emitter
	var view *ui.Renderer // nil in API mode, used for CLI input only
	switch strings.ToLower(outputMode) {
	case "api":
		em = emitter.NewApiEmitter(os.Stdout)
	case "cli", "":
		view = ui.New()
		em = view
	default:
		log.Fatal("Unsupported output mode", zap.String("--mode", outputMode))
	}

	// Select the corresponding ChatClient and model
	var (
		client    clients.ChatClient
		modelname string
	)
	switch strings.ToLower(modelType) {
	case "openai":
		apiKey = cfg.OpenAI.ApiKey
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
		modelname = cfg.OpenAI.ModelName
		if modelname == "" {
			modelname = "gpt-4o"
		}
		client = clients.NewOpenAIClient(baseURL, apiKey)
		log.Info("OpenAI backend has been selected", zap.String("model", modelname), zap.String("base_url", baseURL))
	case "ollama", "":
		if cfg.Ollama.BaseURL == "" {
			log.Fatal("BaseURL is empty", zap.String("base_url", cfg.Ollama.BaseURL))
		}
		client = clients.NewOllamaClient(cfg.Ollama.BaseURL)
		if cfg.Ollama.ModelName == "" {
			log.Fatal("modelName is empty", zap.String("model_name", cfg.Ollama.ModelName))
		}
		modelname = cfg.Ollama.ModelName
		log.Info("Olama backend has been selected", zap.String("model", modelname))
	case "anthropic":
		apiKey = cfg.Anthropic.ApiKey
		if apiKey == "" {
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
		if apiKey == "" {
			em.EmitError("config", "Please set the API key ANTHROPIC_API_KEY")
			log.Fatal("No API key ANTHROPIC_API_KEY")
		}
		client = clients.NewAnthropicClient(apiKey)
		modelname = cfg.Anthropic.ModelName
		if modelname == "" {
			modelname = "claude-opus-4-8"
		}
		log.Info("Anthropic backend has been selected", zap.String("model", modelname))
	case "gemini":
		apiKey = cfg.Gemini.ApiKey
		if apiKey == "" {
			apiKey = os.Getenv("GEMINI_API_KEY")
		}
		if apiKey == "" {
			em.EmitError("config", "Please set the API key GEMINI_API_KEY")
			log.Fatal("No API key GEMINI_API_KEY")
		}
		client = clients.NewGeminiClient(apiKey)
		modelname = cfg.Gemini.ModelName
		if modelname == "" {
			modelname = "gemini-3.5-flash"
		}
		log.Info("Gemini backend has been selected", zap.String("model", modelname))
	default:
		em.EmitError("config", "Unsupported model backend types: "+modelType)
		log.Fatal("Unsupported model backend types", zap.String("-m", modelType))
	}

	log.Debug("Skill Dir", zap.String("dir", cfg.Skills.Dir))

	workDir, err := os.Getwd()
	if err != nil {
		log.Fatal("Failed to get working directory", zap.Error(err))
	}
	// Instantiate tool context
	toolCtx := &tools.ToolContext{
		WorkPath:           workDir,
		Ctx:                ctx,
		DefaultBashTimeout: time.Duration(cfg.Tools.DefaultBashTimeout) * time.Second,
		Logger:             log.Named("tool"),
		SessionID:          uuid.NewString(),
		AppState:           map[string]any{"turn": 0},
	}

	// create a skill registry
	skillregistry, err := skills.NewSkillRegistry(cfg.Skills.Dir)
	if err != nil {
		log.Warn("Load skills failed, continue with empty registry", zap.Error(err))
		skillregistry, _ = skills.NewSkillRegistry("")
	}

	// Assemble System Prompt with pipeline
	promptBuilder := prompt.NewSystemPromptBuilder(skillregistry, cfg.Memory.Dir, modelname, toolCtx.WorkPath)
	pipeline := prompt.NewPipeline(promptBuilder)
	log.Info("MessagePipeline initialized")

	// Create a tool registry
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

	// Register save_memory tool
	registry.Register(memory.NewSaveMemoryTool(cfg.Memory.Dir))
	// Register search_memory tool
	registry.Register(memory.NewSearchMemoryTool(cfg.Memory.Dir))

	// Instantiate todo manager
	todoManager := tools.NewTodoManager()
	// Register todo tool
	registry.Register(todoManager)

	// Instantiate compact manager
	compactManager := compact.NewCompactManager(compact.DefaultConfig(*cfg), client, modelname)
	// Register compact tool
	registry.Register(compact.NewCompactTool(compactManager))

	// Initialize session state.message
	state := &fsm.State{
		Messages:  []fsm.Message{},
		TurnCount: 0,
	}

	// Create a subagent
	subAgent := subagent.NewSubAgent(client, modelname, cfg.Subagent.DefaultSystemPrompt, cfg.Subagent.ForkSubtaskPromptPrefix,
		subagent.BuildSubAgentRegistry(), toolCtx, cfg.Subagent.DefaultMaxTurns)

	// Subagent runner function
	subAgentRunner := func(prompt string, parentMessages []fsm.Message) (string, error) {
		if parentMessages == nil {
			return subAgent.Run(prompt)
		}
		return subAgent.RunWithFork(prompt, parentMessages)
	}

	// Called by TaskTool when fork=true, retrieves the latest snapshot of state.Messages
	parentMessagesProvider := func() []fsm.Message {
		return state.Messages
	}

	// Register sub_task tool
	registry.Register(subagent.NewTaskTool(subAgentRunner, parentMessagesProvider))

	// Permission system
	var permitMgr *permission.Manager
	var apiAsker *permission.ApiAsker
	if outputMode == "api" {
		apiAsker = permission.NewApiAsker(os.Stdout)
		permitMgr = permission.NewManager(
			permission.Mode(cfg.Permission.Mode),
			buildPermissionRules(cfg.Permission.DenyRules),
			buildPermissionRules(cfg.Permission.AllowRules),
			apiAsker,
		)
	} else {
		permitMgr = permission.NewManager(
			permission.Mode(cfg.Permission.Mode),
			buildPermissionRules(cfg.Permission.DenyRules),
			buildPermissionRules(cfg.Permission.AllowRules),
			buildAsker(cfg.Permission.Interactive, false, nil),
		)
	}
	// Inject permission valve
	registry.SetPermissionDecider(permitMgr.Decide)
	// Subagent uses an independent tool registry but should share the same permission policy
	if subAgent.Registry != nil {
		subAgent.Registry.SetPermissionDecider(permitMgr.Decide)
	}
	log.Info("Permission system has been enabled",
		zap.String("Mode", string(permitMgr.GetMode())),
		zap.Int("Deny rule count", len(cfg.Permission.DenyRules)),
		zap.Int("Allow rule count", len(cfg.Permission.AllowRules)),
		zap.Bool("Interactive inquiry", cfg.Permission.Interactive),
	)

	log.Info("Registered tool list", zap.Any("tools", registry.GetAllNames()))

	// Assemble hook system: Runner centrally dispatches all events, handlers are registered by event name
	hookRunner := buildHookRunner()
	log.Info("Hook system has been enabled",
		zap.Int("SessionStart handler count", hookRunner.HandlerCount(hook.EventSessionStart)),
		zap.Int("PreToolUse handler count", hookRunner.HandlerCount(hook.EventPreToolUse)),
		zap.Int("PostToolUse handler count", hookRunner.HandlerCount(hook.EventPostToolUse)),
	)

	// Hook EventSessionStart
	hookRunner.Run(hook.EventSessionStart, map[string]any{"model": modelname, "pipeline": "active"})

	// Render session welcome banner
	em.EmitSessionStart(modelname, logger.LogFilePath)
	log.Info("Agent REPL start")

	// API mode: start heartbeat goroutine + concurrent reader
	var concurrentReader *emitter.ConcurrentReader
	if outputMode == "api" {
		concurrentReader = emitter.NewConcurrentReader(os.Stdin, apiAsker)
		defer concurrentReader.Stop()
		go startHeartbeat(ctx, em)
	}

	if outputMode == "api" {
		// ─── API Mode REPL: read NDJSON commands from ConcurrentReader ───
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
						em.EmitSystem("ack", map[string]string{"id": cmd.ID, "status": "rejected", "reason": "invalid or empty message"})
					}
					continue
				}
			}
			// Send ACK
			if cmd.ID != "" && ackStatus == "" {
				em.EmitSystem("ack", map[string]string{"id": cmd.ID, "status": "accepted"})
			}
			switch cmd.Action {
			case "chat":
				payload, _ := emitter.ParseChatPayload(cmd) // already validated above
				// Emit thinking emotion
				em.EmitEmotion("thinking", nil)
				state.Messages = append(state.Messages, fsm.Message{Role: "user", Content: payload.Message})
				agentLoop(cfg, client, state, modelname, pipeline, registry, toolCtx, todoManager, compactManager, hookRunner, em)
			case "config":
				cfgPayload, _ := emitter.ParseConfigPayload(cmd)
				if cfgPayload.ModelType != "" {
					em.EmitInfo("model_type change not supported at runtime, restart required")
				}
			case "clear":
				if len(state.Messages) > 0 && state.Messages[0].Role == "system" {
					state.Messages = state.Messages[:1]
				} else {
					state.Messages = state.Messages[:0]
				}
				state.TurnCount = 0
				em.EmitInfo("history cleared")
			case "compact":
				if len(state.Messages) <= 1 {
					em.EmitInfo("no history to compact")
					continue
				}
				before := compactManager.EstimateSize(state.Messages)
				newMsgs, cerr := compactManager.CompactHistory(state.Messages)
				if cerr != nil {
					em.EmitError("compact", cerr.Error())
					continue
				}
				state.Messages = newMsgs
				after := compactManager.EstimateSize(newMsgs)
				em.EmitCompact(before, after)
			case "exit":
				goto sessionEnd
			default:
				em.EmitInfo("unknown action: " + cmd.Action)
			}
		}
	} else {
		// ─── CLI Mode REPL: read line from stdin ───
		for {
			input, ok := view.PromptUser()
			if !ok {
				em.EmitInfo("EOF, exiting...")
				break
			}
			if input == "" {
				continue
			}
			// Slash command handling
			if strings.HasPrefix(input, "/") {
				if handleSlashCommand(input, state, em, compactManager) {
					break // /exit
				}
				continue
			}
			// Append user message and run agentLoop
			state.Messages = append(state.Messages, fsm.Message{Role: "user", Content: input})
			agentLoop(cfg, client, state, modelname, pipeline, registry, toolCtx, todoManager, compactManager, hookRunner, em)
			em.EmitTurnSeparator()
		}
	}

sessionEnd:

	log.Info("Agent REPL End", zap.Int("total_turns", state.TurnCount))
	em.EmitSessionEnd(state.TurnCount)

	// Session end, trigger EventSessionEnd
	hookRunner.Run(hook.EventSessionEnd, map[string]any{"total_turns": state.TurnCount})
}

// handleSlashCommand handles REPL slash commands. Returns true to exit the main loop.
func handleSlashCommand(input string, state *fsm.State, em emitter.Emitter, cm *compact.CompactManager) bool {
	cmd := strings.TrimSpace(strings.ToLower(input))
	switch cmd {
	case "/exit", "/quit":
		return true
	case "/clear":
		// Keep system message, clear other history
		if len(state.Messages) > 0 && state.Messages[0].Role == "system" {
			state.Messages = state.Messages[:1]
		} else {
			state.Messages = state.Messages[:0]
		}
		state.TurnCount = 0
		em.EmitInfo("history cleared (system prompt kept)")
	case "/compact":
		if len(state.Messages) <= 1 {
			em.EmitInfo("no history to compact")
			return false
		}
		before := cm.EstimateSize(state.Messages)
		newMsgs, cerr := cm.CompactHistory(state.Messages)
		if cerr != nil {
			em.EmitError("compact", cerr.Error())
			return false
		}
		state.Messages = newMsgs
		after := cm.EstimateSize(newMsgs)
		em.EmitCompact(before, after)
	case "/help":
		em.EmitInfo("/exit  - quit")
		em.EmitInfo("/clear - clear conversation history (system prompt kept)")
		em.EmitInfo("/compact - manually compact conversation history")
		em.EmitInfo("/help  - show this message")
	default:
		em.EmitInfo("unknown command: " + input + "  (try /help)")
	}
	return false
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
