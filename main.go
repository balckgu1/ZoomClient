package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"zoomClient/clients"
	"zoomClient/compact"
	"zoomClient/fsm"
	"zoomClient/hook"
	"zoomClient/logger"
	"zoomClient/permission"
	"zoomClient/skills"
	"zoomClient/subagent"
	"zoomClient/tools"
	"zoomClient/ui"
	"zoomClient/utils"

	"go.uber.org/zap"
)

// agentLoop Agent主循环
func agentLoop(cfg *utils.Config, client clients.ChatClient, state *fsm.State, model string,
	systemPrompt string, registry *tools.Registry, toolCtx *tools.ToolContext, todoManager *tools.TodoManager,
	compactManager *compact.CompactManager, hookRunner *hook.Runner, view *ui.Renderer) {
	log := logger.Log

	// 添加系统提示作为第一条消息
	if len(state.Messages) == 0 || state.Messages[0].Role != "system" {
		state.Messages = append([]fsm.Message{{Role: "system", Content: systemPrompt}}, state.Messages...)
	}

	// 取出已注册的工具列表（由具体客户端内部负责转换为各自的协议格式）
	toolList := registry.GetAll()

	// 无限循环，直到没有工具调用或达到最大轮次限制
	for {
		// 上下文压缩 · 第 2 层（微压缩）: 每次调模型前，把更早的 tool result替换为占位
		state.Messages = compactManager.MicroCompact(state.Messages)

		// 调用上游 LLM API
		response, err := client.Chat(model, state.Messages, toolList, map[string]interface{}{
			"temperature": 0.7,
		})
		if err != nil {
			log.Error("call llm failed", zap.Error(err))
			view.PrintError("LLM", err.Error())
			break
		}

		// 将模型 assistant 的响应内容添加到 []state.Messages
		// DeepSeek 在 thinking 模式下返回的 reasoning_content 须原样透传回 API
		state.Messages = append(state.Messages, fsm.Message{
			Role:             "assistant",
			Content:          response.Message.Content,
			ToolCalls:        response.Message.ToolCalls,
			ReasoningContent: response.Message.ReasoningContent,
		})

		// 检查是否有工具调用，若没有工具调用，则将 reasoning + assistant 文本渲染给用户
		if len(response.Message.ToolCalls) == 0 {
			// 渲染 reasoning
			if response.Message.ReasoningContent != "" {
				view.PrintReasoning(response.Message.ReasoningContent)
			}
			// 渲染 assistant
			view.PrintAssistant(messageContentToString(response.Message.Content))
			state.TransitionReason = nil
			break
		}

		log.Info("Model requested tool call", zap.Int("tool count", len(response.Message.ToolCalls)))

		// 若本轮 reasoning 有内容，渲染给用户
		if response.Message.ReasoningContent != "" {
			view.PrintReasoning(response.Message.ReasoningContent)
		}

		// 按并发安全性将工具调用分批，日志打印分批信息
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

		// 渲染本轮所有工具调用
		for _, tc := range toolCalls {
			if tc.Function.Name == "sub_task" {
				prompt, _ := tc.Function.Arguments["prompt"].(string)
				view.PrintSubAgent(prompt)
				continue
			}
			view.PrintToolCall(tc.Function.Name, formatArgsPreview(tc.Function.Arguments))
		}

		// 检查本轮是否调用了 todo 工具
		usedTodo := false
		for _, tc := range toolCalls {
			if tc.Function.Name == "todo" {
				usedTodo = true
				break
			}
		}

		// Hook 时机2: 在工具执行前，对每个工具触发 EventPreToolUse
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
				state.Messages = append(state.Messages, fsm.Message{
					Role:    "user",
					Content: preDecisions[i].Message,
				})
			}
			if preDecisions[i].ExitCode == hook.ExitBlock {
				view.PrintHookBlocked(tc.Function.Name, preDecisions[i].Message)
			}
		}

		// 执行所有批次
		allowedCalls, allowedIndex := filterAllowedCalls(toolCalls, preDecisions)
		allowedBatches := tools.PartitionToolCalls(allowedCalls)
		allowedResults := tools.ExecuteBatches(allowedBatches, registry, toolCtx)
		results := mergeToolResults(toolCalls, preDecisions, allowedIndex, allowedResults)

		// 按原始调用顺序将Tool Call结果写回消息历史
		for resultIndex, result := range results {
			log.Info("tool call finished",
				zap.String("tool name", toolCalls[resultIndex].Function.Name),
				zap.String("args", formatArgsPreview(toolCalls[resultIndex].Function.Arguments)),
				zap.String("result", result.Content),
			)

			// 渲染工具执行结果摘要
			if preDecisions[resultIndex].ExitCode != hook.ExitBlock {
				view.PrintToolResult(toolCalls[resultIndex].Function.Name, result.Content, result.IsError)
			}

			// 如果 todo 工具被调用且成功，渲染最新计划面板给用户
			if toolCalls[resultIndex].Function.Name == "todo" && result.Ok {
				view.PrintTodoPanel(todoManager.Render())
			}

			// 上下文压缩 第 1 层（大输出落盘），单条工具结果太大时，把全文写到磁盘，消息里只保留预览
			persistedContent := compactManager.PersistLargeOutput(toolCalls[resultIndex].ID, result.Content)
			if persistedContent != result.Content {
				log.Info("tool result content too large, persisted to disk and replaced with preview",
					zap.String("tool name", toolCalls[resultIndex].Function.Name),
					zap.Int("origin bytes", len(result.Content)),
				)
			}

			// ToolCallID 在 OpenAI 协议中必须回填；Ollama 忽略该字段不影响
			state.Messages = append(state.Messages, fsm.Message{
				Role:       "tool",
				Content:    persistedContent,
				ToolCallID: toolCalls[resultIndex].ID,
			})
		}

		// === Hook 时机 3：PostToolUse ===
		runPostToolUseHooks(hookRunner, toolCalls, results, state)

		// 第五步：维护会话计划状态
		// 若本轮未使用 todo 工具，增加未更新计数；超过阈值时将提醒注入到[]state.Messages中, 使模型在下一轮对话开始时看到提醒并刷新计划
		if !usedTodo {
			todoManager.IncrementRoundsSinceUpdate()
			if reminder := todoManager.Reminder(cfg.AgentLoop.TodoRoundsThreshold); reminder != "" {
				log.Info("Plan has not been updated for a long time, injecting reminders",
					zap.Int("Rounds since update", todoManager.PlanningState.RoundsSinceUpdate),
				)
				state.Messages = append(state.Messages, fsm.Message{
					Role:    "user",
					Content: reminder,
				})
			}
		}

		state.TurnCount++
		reason := "tool_result"
		state.TransitionReason = &reason

		// === 上下文压缩 · 第 3 层（整体摘要）===
		// 在本轮所有 tool 结果都已 append 之后再判断是否触发完整压缩。
		// 触发条件：
		//   1) 模型/用户调用了 compact 工具，标记了 pendingManualCompact；
		//   2) 估算的整体上下文体积超过 Config.ContextLimit
		// 压缩成功后，state.Messages 会被替换为 system + 一条连续性摘要
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
				view.PrintCompact(beforeSize, afterSize)
				state.Messages = newMessages
			}
		}

		// 限制最大轮次，避免无限循环
		if state.TurnCount >= cfg.AgentLoop.MaxTurns {
			log.Warn("Reaching the maximum round, stop the loop", zap.Int("max_turns", cfg.AgentLoop.MaxTurns))
			view.PrintInfo(fmt.Sprintf("reached max turns (%d), stop", cfg.AgentLoop.MaxTurns))
			break
		}
	}
}

func main() {
	// 初始化全局日志记录器
	logger.Init()
	defer logger.Sync()
	log := logger.Log

	// 读取配置文件
	utils.InitConfig()
	cfg := utils.GetConfig()
	apiKey := cfg.ApiKey.Deepseek

	// 解析命令行参数：-m 指定模型后端类型（默认 deepseek）
	var modelType string
	flag.StringVar(&modelType, "m", "deepseek", "Model backend type: ollama | deepseek, default: deepseek")
	flag.Parse()

	// 初始化前端渲染器
	view := ui.New()

	// 根据 -m 参数选择对应的 ChatClient 实现与模型名称
	var (
		client    clients.ChatClient
		modelname string
	)
	switch strings.ToLower(modelType) {
	case "deepseek":
		if apiKey == "" {
			// DeepSeek 客户端需要从环境变量读取 API Key，避免密钥硬编码
			apiKey = os.Getenv("DEEPSEEK_API_KEY")
		}
		if apiKey == "" {
			view.PrintError("config", "Please set the API key")
			log.Fatal("No API key DEEPSEEK_API_KEY")
		}
		client = clients.NewDeepSeekClient("https://api.deepseek.com", apiKey)
		modelname = "deepseek-v4-flash"
		log.Info("DeepSeek backend has been selected", zap.String("model", modelname))
	case "ollama", "":
		client = clients.NewOllamaClient("http://127.0.0.1:11434")
		modelname = "modelscope.cn/Qwen/Qwen3-8B-GGUF:latest"
		log.Info("Olama backend has been selected", zap.String("model", modelname))
	default:
		view.PrintError("config", "Unsupported model backend types: "+modelType)
		log.Fatal("Unsupported model backend types", zap.String("-m", modelType))
	}

	log.Debug("Skill Dir", zap.String("dir", cfg.Skills.Dir))

	// 系统提示中告知模型使用todotool规划多步骤任务，并保持计划持续更新
	// 注入宿主 OS 信息，引导模型生成平台兼容的命令
	systemPrompt := fmt.Sprintf(
		"You are a helpful assistant running on %s. "+
			"Use the todo tool to plan multi-step work. "+
			"Keep exactly one step in_progress when a task has multiple steps. "+
			"Refresh the plan as work advances. Prefer tools over prose.",
		runtime.GOOS,
	)

	// 实例化工具上下文
	toolCtx := &tools.ToolContext{
		WorkPath: "./workdir",
	}

	skillregistry, err := skills.NewRegistry(cfg.Skills.Dir)
	if err != nil {
		log.Warn("Load skills failed, continue with empty registry", zap.Error(err))
		skillregistry, _ = skills.NewRegistry("")
	}

	systemPromptSuffix := skillregistry.DescribeAvailable()
	if systemPromptSuffix != "" {
		systemPrompt += "\n\nSkills available (call the load_skill tool to load the full body on demand):\n" + systemPromptSuffix
		log.Info("Added skills to system prompt", zap.Int("count", skillregistry.Count()), zap.Strings("names", skillregistry.Names()))
	} else {
		log.Info("No skills available, skip adding skills to system prompt")
	}

	// 创建工具注册表并注册所有工具
	registry := tools.NewRegistry()
	registry.Register(tools.WriteFileTool{})
	registry.Register(tools.EditFileTool{})
	registry.Register(tools.ReadFileTool{})
	registry.Register(tools.RunBashTool{})

	// 将load_skills tool 注册到工具注册表
	registry.Register(skills.NewLoadSkillTool(skillregistry))

	// 创建会话计划管理器
	todoManager := tools.NewTodoManager()
	//将todoManager注册为工具
	registry.Register(todoManager)

	// 创建上下文压缩管理器
	compactManager := compact.NewCompactManager(compact.DefaultConfig(*cfg), client, modelname)
	// 注册 compact 工具，模型/用户可以主动请求一次完整压缩
	registry.Register(compact.NewCompactTool(compactManager))

	// 初始化会话状态：system 提示作为首条，后续 REPL 每轮 append user/assistant/tool 消息
	state := &fsm.State{
		Messages:  []fsm.Message{},
		TurnCount: 0,
	}

	// 创建 subagent
	subAgent := subagent.NewSubAgent(client, modelname, cfg.Subagent.DefaultSystemPrompt, cfg.Subagent.ForkSubtaskPromptPrefix,
		subagent.BuildSubAgentRegistry(), toolCtx, cfg.Subagent.DefaultMaxTurns)

	// 子智能体统一运行器
	subAgentRunner := func(prompt string, parentMessages []fsm.Message) (string, error) {
		if parentMessages == nil {
			return subAgent.Run(prompt)
		}
		return subAgent.RunWithFork(prompt, parentMessages)
	}

	// 父消息提供者：fork=true 时由 TaskTool 回调，取最新的 state.Messages 快照
	// 闭包通过指针引用 state，保证每次调用都读取当时最新的 messages
	parentMessagesProvider := func() []fsm.Message {
		return state.Messages
	}

	// 注册 sub_task 工具
	registry.Register(subagent.NewTaskTool(subAgentRunner, parentMessagesProvider))

	// 装配 permission 系统
	permitMgr := permission.NewManager(
		permission.Mode(cfg.Permission.Mode),
		buildPermissionRules(cfg.Permission.DenyRules),
		buildPermissionRules(cfg.Permission.AllowRules),
		buildAsker(cfg.Permission.Interactive),
	)
	// 注入 permission 阀门
	registry.SetPermissionDecider(permitMgr.Decide)
	// 子智能体使用独立的工具注册表，但应共享同一套权限策略
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

	// 装配 hook 系统：Runner 集中调度所有事件，handler 按事件名注册到 Runner 上
	hookRunner := buildHookRunner()
	log.Info("Hook system has been enabled",
		zap.Int("SessionStart handler count", hookRunner.HandlerCount(hook.EventSessionStart)),
		zap.Int("PreToolUse handler count", hookRunner.HandlerCount(hook.EventPreToolUse)),
		zap.Int("PostToolUse handler count", hookRunner.HandlerCount(hook.EventPostToolUse)),
	)

	// Hook EventSessionStart
	hookRunner.Run(hook.EventSessionStart, map[string]any{"model": modelname, "system_prompt": systemPrompt})

	// 渲染会话欢迎横幅
	view.PrintSessionStart(modelname, logger.LogFilePath)
	log.Info("Agent REPL start")

	// REPL 主循环：每轮读一行输入 → 处理斜杠命令 或 调用 agentLoop → 渲染分隔
	for {
		input, ok := view.PromptUser()
		if !ok {
			view.PrintInfo("EOF, exiting...")
			break
		}
		if input == "" {
			continue
		}

		// 斜杠命令处理
		if strings.HasPrefix(input, "/") {
			if handleSlashCommand(input, state, view, compactManager) {
				break // /exit
			}
			continue
		}

		// 追加用户消息并运行 agentLoop
		state.Messages = append(state.Messages, fsm.Message{Role: "user", Content: input})
		agentLoop(cfg, client, state, modelname, systemPrompt, registry, toolCtx, todoManager, compactManager, hookRunner, view)
		view.PrintTurnSeparator()
	}

	log.Info("Agent REPL End", zap.Int("total_turns", state.TurnCount))
	view.PrintSessionEnd(state.TurnCount)

	// 会话结束，触发EventSessionEnd
	hookRunner.Run(hook.EventSessionEnd, map[string]any{"total_turns": state.TurnCount})
}

// handleSlashCommand 处理 REPL 斜杠命令。返回 true 表示要退出主循环。
func handleSlashCommand(input string, state *fsm.State, view *ui.Renderer, cm *compact.CompactManager) bool {
	cmd := strings.TrimSpace(strings.ToLower(input))
	switch cmd {
	case "/exit", "/quit":
		return true
	case "/clear":
		// 保留 system 消息，清空其他历史
		if len(state.Messages) > 0 && state.Messages[0].Role == "system" {
			state.Messages = state.Messages[:1]
		} else {
			state.Messages = state.Messages[:0]
		}
		state.TurnCount = 0
		view.PrintInfo("history cleared (system prompt kept)")
	case "/compact":
		if len(state.Messages) <= 1 {
			view.PrintInfo("no history to compact")
			return false
		}
		before := cm.EstimateSize(state.Messages)
		newMsgs, cerr := cm.CompactHistory(state.Messages)
		if cerr != nil {
			view.PrintError("compact", cerr.Error())
			return false
		}
		state.Messages = newMsgs
		after := cm.EstimateSize(newMsgs)
		view.PrintCompact(before, after)
	case "/help":
		view.PrintInfo("/exit  - quit")
		view.PrintInfo("/clear - clear conversation history (system prompt kept)")
		view.PrintInfo("/compact - manually compact conversation history")
		view.PrintInfo("/help  - show this message")
	default:
		view.PrintInfo("unknown command: " + input + "  (try /help)")
	}
	return false
}

// messageContentToString 将 fsm.Message.Content (interface{}) 安全地转为可读字符串。
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

// formatArgsPreview 将工具调用参数压缩为一行预览，用于前端渲染。
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

// buildPermissionRules 把配置中的 PermissionRuleConfig 列表转换为 permission.Rule。
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

// buildAsker 根据配置选择命中 ask 时的交互方式。
//   - interactive=true  从 stdin 询问用户
//   - interactive=false 一律拒绝，适合 CI / 后台作业场景的安全默认
func buildAsker(interactive bool) permission.Asker {
	if interactive {
		return permission.NewStdinAsker()
	}
	return permission.DenyAsker{}
}

// ===================== Hook 装配与主循环辅助函数 =====================

// buildHookRunner 构造一个 hook runner
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

// filterAllowedCalls 筛选出未被 hook 阻止的工具调用，并保留它们到原始下标的映射。
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

// mergeToolResults 把执行结果按原始顺序合并回去；被 hook 阻止的位置用阻止结果填充。
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

// runPostToolUseHooks 对每一个工具结果触发 PostToolUse 事件。
// exit=2 时把 Message 作为 user 消息注入历史。
func runPostToolUseHooks(runner *hook.Runner, toolCalls []tools.ToolCall, results []tools.ToolResult, state *fsm.State) {
	for i, tc := range toolCalls {
		if results[i].IsError {
			errordecision := runner.Run(hook.EventToolError, map[string]any{
				"tool_name": tc.Function.Name,
				"input":     tc.Function.Arguments,
				"output":    results[i].Content,
			})
			if errordecision.ExitCode == hook.ExitInject && errordecision.Message != "" {
				state.Messages = append(state.Messages, fsm.Message{
					Role:    "user",
					Content: errordecision.Message,
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
