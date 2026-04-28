package main

import (
	"encoding/json"
	"zoomClient/clients"
	"zoomClient/fsm"
	"zoomClient/logger"
	"zoomClient/tools"

	"go.uber.org/zap"
)

// agentLoop Agent主循环
// todoManager 用于维护当前会话的计划状态，实现计划外显与提醒机制
func agentLoop(client *clients.OllamaClient, state *fsm.State, model string, systemPrompt string, registry *tools.Registry, toolCtx *tools.ToolContext, todoManager *tools.TodoManager) {
	log := logger.Log
	// 添加系统提示作为第一条消息
	if len(state.Messages) == 0 || state.Messages[0].Role != "system" {
		state.Messages = append([]fsm.Message{{Role: "system", Content: systemPrompt}}, state.Messages...)
	}

	// 将工具列表转换为 Ollama API 格式
	ollamaTools := clients.BuildOllamaTools(registry.GetAll())

	// 无限循环，直到没有工具调用或达到最大轮次限制
	for {
		// 调用Ollama API
		response, err := client.Chat(model, state.Messages, ollamaTools, map[string]interface{}{
			"temperature": 0.7,
		})
		if err != nil {
			log.Error("调用 Ollama 失败", zap.Error(err))
			break
		}

		// 将助手的响应添加到消息历史
		state.Messages = append(state.Messages, fsm.Message{
			Role:      "assistant",
			Content:   response.Message.Content,
			ToolCalls: response.Message.ToolCalls,
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

		// 第三步：执行所有批次
		results := tools.ExecuteBatches(batches, registry, toolCtx)

		// 第四步：按原始顺序将结果写回消息历史，而非按完成顺序
		for resultIndex, result := range results {
			log.Info("工具执行完成",
				zap.String("工具名称", toolCalls[resultIndex].Function.Name),
				zap.String("执行结果", result.Content),
			)
			state.Messages = append(state.Messages, fsm.Message{
				Role:    "tool",
				Content: result.Content,
			})
		}

		// 第五步：维护会话计划状态
		// 若本轮未使用 todo 工具，增加未更新计数；超过阈值时将提醒注入到消息历史，
		// 使模型在下一轮对话开始时看到提醒并刷新计划
		if !usedTodo {
			todoManager.IncrementRoundsSinceUpdate()
			if reminder := todoManager.Reminder(); reminder != "" {
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

		// 限制最大轮次，避免无限循环
		if state.TurnCount >= 10 {
			log.Warn("已达最大轮次限制，停止循环", zap.Int("max_turns", 10))
			break
		}
	}
}

func main() {
	// 初始化全局日志记录器
	logger.Init()
	defer logger.Sync()
	log := logger.Log

	// 创建Ollama客户端
	client := clients.NewOllamaClient("http://127.0.0.1:11434")
	modelname := "modelscope.cn/Qwen/Qwen3-8B-GGUF:latest"
	userPrompt := "创建一个hello.py文件，内容为'print(\"Hello, World!\")'。并读取该文件内容。之后修改hello.py，修改为'print(\"Hello, China!\")'"
	// 系统提示中告知模型使用 todo 工具规划多步骤任务，并保持计划持续更新
	systemPrompt := "You are a helpful AI assistant. Use the todo tool to plan multi-step work. Keep exactly one step in_progress when a task has multiple steps. Refresh the plan as work advances. Prefer tools over prose."
	toolCtx := &tools.ToolContext{
		WorkPath: "./",
	}

	// 创建会话计划管理器
	todoManager := tools.NewTodoManager()

	// 创建工具注册表并注册所有工具（含 todo 计划管理工具）
	registry := tools.NewRegistry()
	registry.Register(tools.WriteFileTool{})
	registry.Register(tools.EditFileTool{})
	registry.Register(tools.ReadFileTool{})
	registry.Register(tools.RunBashTool{})
	registry.Register(todoManager)
	log.Info("已注册的工具列表", zap.Any("tools", registry.GetAllNames()))

	// 初始化状态
	state := &fsm.State{
		Messages: []fsm.Message{
			{Role: "user", Content: userPrompt},
		},
		TurnCount: 0,
	}

	// 运行Agent循环
	log.Info("Agent 循环启动")
	agentLoop(client, state, modelname, systemPrompt, registry, toolCtx, todoManager)

	log.Info("Agent 循环结束", zap.Int("total_turns", state.TurnCount))
	log.Debug("完整消息历史", zap.Any("messages", state.Messages))
	for _, msg := range state.Messages {
		role := msg.Role
		content := ""
		toolsname := []string{}
		for _, tc := range msg.ToolCalls {
			toolsname = append(toolsname, tc.Function.Name)
		}
		switch v := msg.Content.(type) {
		case string:
			content = v
		default:
			contentBytes, _ := json.Marshal(v)
			content = string(contentBytes)
		}
		log.Info("消息记录",
			zap.String("role", role),
			zap.String("content", content),
			zap.Strings("tools", toolsname),
		)
	}
}
