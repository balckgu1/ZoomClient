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
func agentLoop(client *clients.OllamaClient, state *fsm.State, model string, systemPrompt string, registry *tools.Registry, toolCtx *tools.ToolContext) {
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

		log.Info("模型请求工具调用", zap.Int("count", len(response.Message.ToolCalls)))

		// 执行工具调用，并将每个结果以 role:"tool" 消息回传
		for _, tc := range response.Message.ToolCalls {
			log.Info("调用工具",
				zap.String("tool", tc.Function.Name),
				zap.Any("args", tc.Function.Arguments),
			)
			output := registry.RunTool(tc.Function.Name, tc.Function.Arguments, toolCtx)
			log.Info("工具返回结果", zap.String("output", output.Content))

			state.Messages = append(state.Messages, fsm.Message{
				Role:    "tool",
				Content: output.Content,
			})
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
	userPrompt := "创建一个hello.py文件，内容为'print(\"Hello, World!\")'，并运行它"
	systemPrompt := "You are a helpful AI assistant. You can use tools when needed."
	toolCtx := &tools.ToolContext{
		WorkPath: "./",
	}

	// 创建工具注册表并注册工具
	registry := tools.NewRegistry()
	registry.Register(tools.WriteFileTool{})
	registry.Register(tools.EditFileTool{})
	registry.Register(tools.ReadFileTool{})
	registry.Register(tools.RunBashTool{})
	log.Info("已注册的工具列表", zap.Any("tools", registry.GetAll()))

	// 初始化状态
	state := &fsm.State{
		Messages: []fsm.Message{
			{Role: "user", Content: userPrompt},
		},
		TurnCount: 0,
	}

	// 运行Agent循环
	log.Info("Agent 循环启动")
	agentLoop(client, state, modelname, systemPrompt, registry, toolCtx)

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
