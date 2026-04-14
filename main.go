package main

import (
	"encoding/json"
	"fmt"
	"zoomClient/clients"
	"zoomClient/fsm"
	"zoomClient/tools"
)

// agentLoop Agent主循环
func agentLoop(client *clients.OllamaClient, state *fsm.State, model string, systemPrompt string, registry *tools.Registry, WorkPath string) {
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
			fmt.Printf("Error calling Ollama: %v\n", err)
			break
		}

		// 将助手的响应添加到消息历史
		state.Messages = append(state.Messages, fsm.Message{
			Role:      "assistant",
			Content:   response.Message.Content,
			ToolCalls: response.Message.ToolCalls,
		})

		// fmt.Printf("Assistant: %s\n================================================\n", response.Message.Content)

		// 检查是否有工具调用
		if len(response.Message.ToolCalls) == 0 {
			state.TransitionReason = nil
			break
		}

		fmt.Printf("Model requested %d tool call(s)\n", len(response.Message.ToolCalls))

		// 执行工具调用，并将每个结果以 role:"tool" 消息回传
		for _, tc := range response.Message.ToolCalls {
			fmt.Printf("  -> Calling tool: %s, args: %v\n", tc.Function.Name, tc.Function.Arguments)
			output := registry.RunTool(tc.Function.Name, tc.Function.Arguments, WorkPath)
			fmt.Printf("  <- Tool result: %s\n", output)

			state.Messages = append(state.Messages, fsm.Message{
				Role:    "tool",
				Content: output,
			})
		}

		state.TurnCount++
		reason := "tool_result"
		state.TransitionReason = &reason

		// 限制最大轮次，避免无限循环
		if state.TurnCount >= 10 {
			fmt.Println("Max turns reached, stopping...")
			break
		}
	}
}

func main() {
	// 创建Ollama客户端
	client := clients.NewOllamaClient("http://127.0.0.1:11434")
	modelname := "modelscope.cn/Qwen/Qwen3-8B-GGUF:latest"
	userPrompt := "创建一个hello.py文件，内容为'print(\"Hello, World!\")'，并运行它"
	systemPrompt := "You are a helpful AI assistant. You can use tools when needed."
	WorkPath := "./"

	// 创建工具注册表并注册工具
	registry := tools.NewRegistry()
	registry.Register(tools.WriteFileTool{})
	registry.Register(tools.EditFileTool{})
	registry.Register(tools.ReadFileTool{})
	registry.Register(tools.RunBashTool{})

	// 初始化状态
	state := &fsm.State{
		Messages: []fsm.Message{
			{Role: "user", Content: userPrompt},
		},
		TurnCount: 0,
	}

	// 运行Agent循环
	fmt.Println("Starting agent loop...")
	agentLoop(client, state, modelname, systemPrompt, registry, WorkPath)

	fmt.Printf("\nFinal state after %d turns:\n", state.TurnCount)
	fmt.Printf("%+v\n--------------------\n", state.Messages)
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
		fmt.Printf("%s: %s, Tool: %+v", role, content, toolsname)
	}
}
