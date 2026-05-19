package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
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
	"zoomClient/utils"

	"go.uber.org/zap"
)

// agentLoop Agent主循环
func agentLoop(cfg *utils.Config, client clients.ChatClient, state *fsm.State, model string,
	systemPrompt string, registry *tools.Registry, toolCtx *tools.ToolContext, todoManager *tools.TodoManager,
	compactManager *compact.CompactManager, hookRunner *hook.Runner) {
	log := logger.Log

	// 添加系统提示作为第一条消息
	if len(state.Messages) == 0 || state.Messages[0].Role != "system" {
		state.Messages = append([]fsm.Message{{Role: "system", Content: systemPrompt}}, state.Messages...)
	}

	// Hook 时机 1：SessionStart
	hookRunner.Run(hook.EventSessionStart, map[string]any{"model": model, "system_prompt": systemPrompt})

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
			log.Error("调用 LLM 失败", zap.Error(err))
			break
		}

		// 将助手的响应添加到消息历史
		// DeepSeek 在 thinking 模式下返回的 reasoning_content 必须原样透传回 API，
		// 否则下一轮请求会被服务端以 invalid_request_error 拒绝。
		state.Messages = append(state.Messages, fsm.Message{
			Role:             "assistant",
			Content:          response.Message.Content,
			ToolCalls:        response.Message.ToolCalls,
			ReasoningContent: response.Message.ReasoningContent,
		})

		// 检查是否有工具调用
		if len(response.Message.ToolCalls) == 0 {
			state.TransitionReason = nil
			break
		}

		log.Info("模型请求工具调用", zap.Int("工具数量", len(response.Message.ToolCalls)))

		// 第一步：按并发安全性将工具调用分批，并打印分批信息（便于观察调度策略）
		toolCalls := response.Message.ToolCalls
		batches := tools.PartitionToolCalls(toolCalls)
		log.Info("工具调用分批情况", zap.Int("批次数量", len(batches)))
		for batchIndex, batch := range batches {
			batchToolNames := make([]string, 0, len(batch.Tools))
			for _, tracked := range batch.Tools {
				batchToolNames = append(batchToolNames, tracked.Name)
			}
			log.Info("批次详情",
				zap.Int("批次序号", batchIndex),
				zap.Bool("可并发执行", batch.IsConcurrencySafe),
				zap.Strings("工具列表", batchToolNames),
			)
		}

		// 第二步：检查本轮是否调用了 todo 工具
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
		}

		// 第三步：执行所有批次（被 hook 阻止的工具会跳过执行，由 mergeToolResults 填充阻止结果）
		allowedCalls, allowedIndex := filterAllowedCalls(toolCalls, preDecisions)
		allowedBatches := tools.PartitionToolCalls(allowedCalls)
		allowedResults := tools.ExecuteBatches(allowedBatches, registry, toolCtx)
		results := mergeToolResults(toolCalls, preDecisions, allowedIndex, allowedResults)

		// === Hook 时机 3：PostToolUse ===
		// 工具执行后
		runPostToolUseHooks(hookRunner, toolCalls, results, state)

		// 第四步：按原始顺序将结果写回消息历史，而非按完成顺序
		for resultIndex, result := range results {
			log.Info("工具执行完成",
				zap.String("工具名称", toolCalls[resultIndex].Function.Name),
				zap.String("执行结果", result.Content),
			)

			// 上下文压缩 第 1 层（大输出落盘），单条工具结果太大时，把全文写到磁盘，消息里只保留预览
			persistedContent := compactManager.PersistLargeOutput(toolCalls[resultIndex].ID, result.Content)
			if persistedContent != result.Content {
				log.Info("tool result content too large, persisted to disk and replaced with preview",
					zap.String("工具名称", toolCalls[resultIndex].Function.Name),
					zap.Int("原始字节数", len(result.Content)),
				)
			}

			// ToolCallID 在 OpenAI 协议中必须回填；Ollama 忽略该字段不影响
			state.Messages = append(state.Messages, fsm.Message{
				Role:       "tool",
				Content:    persistedContent,
				ToolCallID: toolCalls[resultIndex].ID,
			})
		}

		// 第五步：维护会话计划状态
		// 若本轮未使用 todo 工具，增加未更新计数；超过阈值时将提醒注入到消息历史，
		// 使模型在下一轮对话开始时看到提醒并刷新计划
		if !usedTodo {
			todoManager.IncrementRoundsSinceUpdate()
			if reminder := todoManager.Reminder(cfg.AgentLoop.TodoRoundsThreshold); reminder != "" {
				log.Info("计划长时间未更新，注入提醒",
					zap.Int("连续未更新轮次", todoManager.PlanningState.RoundsSinceUpdate),
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
				log.Warn("完整压缩失败，保留原始消息历史继续", zap.Error(cerr))
			} else {
				afterSize := compactManager.EstimateSize(newMessages)
				log.Info("已完成整体压缩",
					zap.Int("压缩前字节数", beforeSize),
					zap.Int("压缩后字节数", afterSize),
					zap.Int("消息条数", len(newMessages)),
				)
				state.Messages = newMessages
			}
		}

		// 限制最大轮次，避免无限循环
		if state.TurnCount >= cfg.AgentLoop.MaxTurns {
			log.Warn("已达最大轮次限制，停止循环", zap.Int("max_turns", cfg.AgentLoop.MaxTurns))
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
	flag.StringVar(&modelType, "m", "deepseek", "模型后端类型: ollama 或 deepseek, 默认deepseek")
	flag.Parse()

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
			log.Fatal("使用 deepseek 后端时必须设置API密钥 DEEPSEEK_API_KEY")
		}
		client = clients.NewDeepSeekClient("https://api.deepseek.com", apiKey)
		modelname = "deepseek-v4-flash"
		log.Info("已选择 DeepSeek 后端", zap.String("model", modelname))
	case "ollama", "":
		client = clients.NewOllamaClient("http://127.0.0.1:11434")
		modelname = "modelscope.cn/Qwen/Qwen3-8B-GGUF:latest"
		log.Info("已选择 Ollama 后端", zap.String("model", modelname))
	default:
		log.Fatal("不支持的模型后端类型", zap.String("-m", modelType))
	}

	log.Debug("Skill Dir", zap.String("dir", cfg.Skills.Dir))
	var userPrompt string
	fmt.Println("> 请输入您的问题")
	// userPrompt = "在./workdir目录下创建hello.py，内容为打印hello字符串。之后运行hello.py"
	userPrompt, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	userPrompt = strings.TrimSpace(userPrompt)

	// 系统提示中告知模型使用todotool规划多步骤任务，并保持计划持续更新
	systemPrompt := "You are a helpful assistant. Use the todo tool to plan multi-step work. Keep exactly one step in_progress when a task has multiple steps. Refresh the plan as work advances. Prefer tools over prose."
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

	// 先初始化空 messages，便于后续 parentMessagesProvider 闭包捕获 state 指针
	// agentLoop 运行时会往 state.Messages 追加消息，provider 每次调用都能拿到最新快照
	state := &fsm.State{
		Messages: []fsm.Message{
			{Role: "user", Content: userPrompt},
		},
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
	log.Info("权限系统已启用",
		zap.String("模式", string(permitMgr.GetMode())),
		zap.Int("deny规则数", len(cfg.Permission.DenyRules)),
		zap.Int("allow规则数", len(cfg.Permission.AllowRules)),
		zap.Bool("交互式询问", cfg.Permission.Interactive),
	)

	log.Info("已注册的工具列表", zap.Any("tools", registry.GetAllNames()))

	// 装配 hook 系统：Runner 集中调度所有事件，handler 按事件名注册到 Runner 上
	hookRunner := buildHookRunner()
	log.Info("hook 系统已启用",
		zap.Int("SessionStart handler数", hookRunner.HandlerCount(hook.EventSessionStart)),
		zap.Int("PreToolUse handler数", hookRunner.HandlerCount(hook.EventPreToolUse)),
		zap.Int("PostToolUse handler数", hookRunner.HandlerCount(hook.EventPostToolUse)),
	)

	// 运行Agent循环
	log.Info("Agent 循环启动")
	agentLoop(cfg, client, state, modelname, systemPrompt, registry, toolCtx, todoManager, compactManager, hookRunner)

	log.Info("Agent 循环结束", zap.Int("total_turns", state.TurnCount))

	// 会话结束，触发EventSessionEnd
	hookRunner.Run(hook.EventSessionEnd, map[string]any{"total_turns": state.TurnCount})
	// // log.Debug("完整消息历史", zap.Any("messages", state.Messages))
	// for _, msg := range state.Messages {
	// 	role := msg.Role
	// 	content := ""
	// 	toolsname := []string{}
	// 	for _, tc := range msg.ToolCalls {
	// 		toolsname = append(toolsname, tc.Function.Name)
	// 	}
	// 	switch v := msg.Content.(type) {
	// 	case string:
	// 		content = v
	// 	default:
	// 		contentBytes, _ := json.Marshal(v)
	// 		content = string(contentBytes)
	// 	}
	// 	log.Debug("消息记录",
	// 		zap.String("role", role),
	// 		zap.String("content", content),
	// 		zap.Strings("tools", toolsname),
	// 	)
	// }
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
