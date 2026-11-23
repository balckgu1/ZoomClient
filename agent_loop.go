package main

// 本文件实现 Agent 核心推理循环 agentLoop，以及配套的工具调用过滤、
// 结果合并、hook 触发等辅助函数。

import (
	"encoding/json"
	"fmt"

	"zoomClient/clients"
	"zoomClient/hook"
	"zoomClient/logger"
	"zoomClient/prompt"
	"zoomClient/tools"

	"go.uber.org/zap"
)

// agentLoop 是主 Agent 推理循环。
// stopCh 是可选的（CLI/API 模式下为 nil）。当它被关闭时，循环会在下一个安全点中止。
func agentLoop(s *AgentSession, stopCh <-chan struct{}) {

	cfg, client, state, model := s.Cfg, s.Client, s.State, s.ModelName
	pipeline, registry, toolCtx := s.Pipeline, s.Registry, s.ToolCtx
	todoManager, compactManager := s.TodoManager, s.CompactManager
	hookRunner, em := s.HookRunner, s.Em
	temp := s.Temperature
	reasonLevel := s.ReasonLevel
	logs := logger.Log

	// 获取工具列表
	toolList := registry.GetAll()

	// 无限循环，直到没有工具调用或达到最大回合数
	for {
		// 检查停止信号（Web 模式）
		select {
		case <-stopCh:
			em.EmitInfo("Generation stopped by user")
			return
		default:
		}
		// 上下文压缩：微压缩，用占位符替换较旧的工具结果
		state.Messages = compactManager.MicroCompact(state.Messages)

		// 流水线组装：系统提示词 + state.messages + 提醒 + 附件
		payload := pipeline.AssemblePayload(state.Messages)

		// 清除 OneShot 提醒
		pipeline.ClearOneShotReminders()

		// 在 state.messages 开头添加系统提示词
		fullMessages := append([]clients.Message{{Role: "system", Content: payload.SystemPrompt}}, payload.Messages...)

		// LLM chat（带 hook 干预：PreChat / LLMError / PostChat，失败或空回复时自动重试）
		response, err := chatWithHooks(hookRunner, client, model, fullMessages, toolList,
			map[string]interface{}{
				"temperature":      temp,
				"reasoning_effort": reasonLevel,
			},
			cfg.AgentLoop.MaxLLMRetries, pipeline)

		if err != nil {
			logs.Error("call llm failed", zap.Error(err))
			em.EmitError("LLM", err.Error())
			break
		}

		// 将 assistant 的回复添加到 state.messages
		state.Messages = append(state.Messages, clients.Message{
			Role:             "assistant",
			Content:          response.Message.Content,
			ToolCalls:        response.Message.ToolCalls,
			ReasoningContent: response.Message.ReasoningContent,
		})

		// 检查是否有任何工具调用，如果没有，则把 reasoning+assistant 文本渲染给用户
		if len(response.Message.ToolCalls) == 0 {
			// 渲染推理内容
			if response.Message.ReasoningContent != "" {
				em.EmitReasoning(response.Message.ReasoningContent)
			}
			// 渲染 assistant 回复
			em.EmitAssistant(messageContentToString(response.Message.Content))
			// 本轮结束前推送上下文占用快照，覆盖最终 assistant 回复的增量
			emitContextUsageWeb(s, payload.SystemPrompt, toolList)
			// done 事件由 REPL 在同步记忆抽取完成后统一补发（回合真正结束）
			state.TransitionReason = nil
			break
		}

		logs.Info("Model requested tool call", zap.Int("tool count", len(response.Message.ToolCalls)))

		// 如果推理内容非空，渲染给用户
		if response.Message.ReasoningContent != "" {
			em.EmitReasoning(response.Message.ReasoningContent)
		}

		// 根据并发安全性对工具进行分批
		toolCalls := response.Message.ToolCalls
		batches := tools.PartitionToolCalls(toolCalls)
		logs.Info("Batch calling of tools", zap.Int("Batch number", len(batches)))
		for batchIndex, batch := range batches {
			batchToolNames := make([]string, 0, len(batch.Tools))
			for _, tracked := range batch.Tools {
				batchToolNames = append(batchToolNames, tracked.Name)
			}
			logs.Info("Batch details", zap.Int("Batch number", batchIndex),
				zap.Bool("Is concurrency safe", batch.IsConcurrencySafe),
				zap.Strings("tool list", batchToolNames),
			)
		}

		// 渲染所有工具调用
		for _, tc := range toolCalls {
			if tc.Name == "sub_task" {
				prompt, _ := tc.Arguments["prompt"].(string)
				em.EmitSubAgent(prompt)
				continue
			}
			em.EmitToolCall(tc.Name, formatArgsPreview(tc.Arguments))
		}

		// 检查本轮是否调用了 Todo 工具
		usedTodo := false
		for _, tc := range toolCalls {
			if tc.Name == "todo" {
				usedTodo = true
				break
			}
		}

		// Hook：在工具执行前为每个工具触发 EventPreToolUse
		preDecisions := make([]hook.HookResult, len(toolCalls))
		for i, tc := range toolCalls {
			preDecisions[i] = hookRunner.HookRun(hook.EventPreToolUse, map[string]any{
				"tool_name":       tc.Name,
				"input":           tc.Arguments,
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
				em.EmitHookBlocked(tc.Name, preDecisions[i].Message)
			}
		}

		// 过滤掉被hook阻止的工具调用
		allowedCalls, allowedIndex := filterAllowedCalls(toolCalls, preDecisions)

		// 执行所有的工具批次
		allowedBatches := tools.PartitionToolCalls(allowedCalls)
		allowedResults := tools.ExecuteBatches(allowedBatches, registry, toolCtx)
		results := mergeToolResults(toolCalls, preDecisions, allowedIndex, allowedResults)

		// 按调用顺序将工具结果写回 Messages
		for resultIndex, result := range results {
			logs.Info("tool call finished",
				zap.String("tool name", toolCalls[resultIndex].Name),
				zap.String("args", formatArgsPreview(toolCalls[resultIndex].Arguments)),
				zap.String("result", result.Content),
			)

			// 渲染工具结果摘要
			if preDecisions[resultIndex].ExitCode != hook.ExitBlock {
				em.EmitToolResult(toolCalls[resultIndex].Name, result.Content, result.IsError)
			}

			// 如果调用了 todo 工具且成功，将最新计划面板渲染给用户
			if toolCalls[resultIndex].Name == "todo" && result.Ok {
				em.EmitTodoPanel(todoManager.Render(), todoManager.Items())
			}

			// 上下文压缩：第一层（大输出持久化）：当单个工具结果过大时，将完整内容写入磁盘，只在消息中保留预览
			persistedContent := compactManager.PersistLargeOutput(toolCalls[resultIndex].ID, result.Content)
			if persistedContent != result.Content {
				logs.Info("tool result content too large, persisted to disk and replaced with preview",
					zap.String("tool name", toolCalls[resultIndex].Name),
					zap.Int("origin bytes", len(result.Content)),
				)
			}

			// 将 toolCallID 添加到 state.messages
			state.Messages = append(state.Messages, clients.Message{
				Role:       "tool",
				Content:    persistedContent,
				ToolCallID: toolCalls[resultIndex].ID,
			})
		}

		// Hook：PostToolUse
		runPostToolUseHooks(hookRunner, toolCalls, results, pipeline)

		// 如果本轮未使用 Todo 工具，则增加计数，并在超过阈值时通过流水线注入提醒
		if !usedTodo {
			todoManager.IncrementRoundsSinceUpdate()
			if reminder := todoManager.Reminder(cfg.AgentLoop.TodoRoundsThreshold); reminder != "" {
				logs.Info("Plan has not been updated for a long time, injecting reminders",
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

		// 完整上下文压缩
		// 在所有工具结果追加到 state.messages 之后，判断是否触发完整压缩
		// 压缩成功后，state.Messages 将被替换为 system + 一段连续性摘要
		if compactManager.ShouldAutoCompact(state.Messages) {
			beforeSize := compactManager.EstimateSize(state.Messages)
			newMessages, cerr := compactManager.CompactHistory(state.Messages)
			if cerr != nil {
				logs.Warn("Complete compression failed, keep the original message history to continue", zap.String("session", toolCtx.SessionID), zap.Error(cerr))
			} else {
				afterSize := compactManager.EstimateSize(newMessages)
				logs.Info("Complete compression completed",
					zap.String("Session", toolCtx.SessionID),
					zap.Int("Bytes before compression", beforeSize),
					zap.Int("Bytes after compression", afterSize),
					zap.Int("Message count", len(newMessages)),
				)
				em.EmitCompact(beforeSize, afterSize)
				state.Messages = newMessages
			}
		}

		// 推送上下文占用快照：反映本轮工具结果追加与完整压缩后的真实占用
		emitContextUsageWeb(s, payload.SystemPrompt, toolList)

		// 限制最大回合数以避免无限循环
		if state.TurnCount >= cfg.AgentLoop.MaxTurns {
			logs.Warn("Reaching the maximum round, stop the loop", zap.Int("max_turns", cfg.AgentLoop.MaxTurns))
			em.EmitInfo(fmt.Sprintf("reached max turns (%d), stop", cfg.AgentLoop.MaxTurns))
			break
		}
	}
}

// filterAllowedCalls 过滤掉被hook阻止的工具调用，只留下可以运行的工具。
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

// mergeToolResults 按原始顺序合并执行结果；被 hook 阻止的位置用阻止结果填充。
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

// runPostToolUseHooks 为每个工具结果触发 PostToolUse 事件。
// 当 exit=2 时，将 Message 作为 OneShot 提醒注入流水线。
func runPostToolUseHooks(runner *hook.Runner, toolCalls []tools.ToolCall, results []tools.ToolResult, pipeline *prompt.MessagePipeline) {
	for i, tc := range toolCalls {
		if results[i].IsError {
			errordecision := runner.HookRun(hook.EventToolError, map[string]any{
				"tool_name": tc.Name,
				"input":     tc.Arguments,
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
		runner.HookRun(hook.EventPostToolUse, map[string]any{
			"tool_name": tc.Name,
			"input":     tc.Arguments,
			"output":    results[i].Content,
			"is_error":  results[i].IsError,
		})
	}
}

// chatWithHooks 在 LLM 调用前后触发 hook（PreChat / LLMError / PostChat），并按 hook 决策自动重试。
//
// 行为约定：
//   - PreChat 返回 ExitBlock 时中止调用并返回错误；
//   - LLMError 返回 ExitRetry 时重试调用（失败路径）；
//   - PostChat 返回 ExitRetry 时重试调用（回复校验路径）；
//   - 两条重试路径共享同一个 maxRetries 预算，防止无限重试；
//   - PreChat / PostChat 返回 ExitInject 时，消息以 OneShot reminder 注入，在下一轮组装提示词时生效。
func chatWithHooks(hookRunner *hook.Runner, client clients.ChatClient, model string,
	fullMessages []clients.Message, toolList []tools.Tool, options map[string]interface{},
	maxRetries int, pipeline *prompt.MessagePipeline) (*clients.ChatResponse, error) {

	logs := logger.Log
	// attempt 从 0 开始
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 1. PreChat：调用前观察/拦截/注入
		preDecision := hookRunner.HookRun(hook.EventPreChat, map[string]any{
			"model":          model,
			"messages_count": len(fullMessages),
			"est_tokens":     estimateTokens(fullMessages),
		})
		if preDecision.ExitCode == hook.ExitBlock {
			return nil, fmt.Errorf("LLM call blocked by pre-chat hook: %s", preDecision.Message)
		}

		// 2. 调用 LLM 对话
		response, err := client.Chat(model, fullMessages, toolList, options)

		// 3. LLMError：调用失败时重试
		if err != nil {
			decision := hookRunner.HookRun(hook.EventLLMError, map[string]any{
				"model":          model,
				"messages_count": len(fullMessages),
				"est_tokens":     estimateTokens(fullMessages),
				"retry_count":    attempt,
				"max_retries":    maxRetries,
				"error":          err.Error(),
			})
			if decision.ExitCode == hook.ExitRetry && attempt < maxRetries {
				logs.Info("[hook] LLM call failed, retrying",
					zap.Int("attempt", attempt+1),
					zap.Int("max_retries", maxRetries),
					zap.String("error", err.Error()),
				)
				continue
			}
			return nil, err
		}

		// 4. EventPostChat LLM 回复成功：交给 PostChat hook 校验回复质量
		postDecision := hookRunner.HookRun(hook.EventPostChat, map[string]any{
			"model":            model,
			"content":          messageContentToString(response.Message.Content),
			"tool_calls_count": len(response.Message.ToolCalls),
			"usage":            response.Usage,
			"retry_count":      attempt,
			"max_retries":      maxRetries,
		})

		if postDecision.ExitCode == hook.ExitRetry && attempt < maxRetries {
			logs.Info("[hook] LLM response rejected, retrying",
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries),
				zap.String("reason", postDecision.Message),
			)
			continue
		}

		if postDecision.ExitCode == hook.ExitInject && postDecision.Message != "" {
			pipeline.AddReminder(prompt.Reminder{
				Content: postDecision.Message,
				Source:  "post_chat_hook",
				OneShot: true,
			})
		}

		return response, nil
	}
	return nil, fmt.Errorf("LLM call failed after %d retries", maxRetries)
}

// estimateTokens 以字符数/4 的启发式粗略估算消息序列的 token 数。
// 仅用于 PreChat 审计日志与预算提示，不参与任何拦截决策，允许存在误差。
func estimateTokens(messages []clients.Message) int {
	totalChars := 0
	for _, m := range messages {
		switch c := m.Content.(type) {
		case string:
			totalChars += len(c)
		case nil:
			// 空内容不计入
		default:
			totalChars += len(fmt.Sprintf("%v", c))
		}
	}
	if totalChars == 0 {
		return 0
	}
	return totalChars/4 + 1
}

// messageContentToString 将 clients.Message.Content（interface{}）安全地转换为可读字符串。
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
