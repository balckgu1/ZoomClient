package main

// 本文件实现 Agent 核心推理循环 agentLoop，以及配套的工具调用过滤、
// 结果合并、hook 触发等辅助函数。

import (
	"encoding/json"
	"fmt"

	"zoomClient/clients"
	"zoomClient/fsm"
	"zoomClient/hook"
	"zoomClient/logger"
	"zoomClient/prompt"
	"zoomClient/tools"

	"go.uber.org/zap"
)

// agentLoop is the main agent reasoning loop.
// stopCh is optional (nil for CLI/API mode). When closed, the loop aborts at the next safe point.
func agentLoop(s *AgentSession, stopCh <-chan struct{}) {

	cfg, client, state, model := s.Cfg, s.Client, s.State, s.ModelName
	pipeline, registry, toolCtx := s.Pipeline, s.Registry, s.ToolCtx
	todoManager, compactManager := s.TodoManager, s.CompactManager
	hookRunner, em := s.HookRunner, s.Em
	logs := logger.Log

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

		// Clear OneShot reminder
		pipeline.ClearOneShotReminders()

		// Add system prompt at the beginning of state.messages
		fullMessages := append([]fsm.Message{{Role: "system", Content: payload.SystemPrompt}}, payload.Messages...)

		// LLM chat（带 hook 干预：PreChat / LLMError / PostChat，失败或空回复时自动重试）
		response, err := chatWithHooks(hookRunner, client, model, fullMessages, toolList, map[string]interface{}{"temperature": 0.7}, cfg.AgentLoop.MaxLLMRetries, pipeline)
		if err != nil {
			logs.Error("call llm failed", zap.Error(err))
			em.EmitError("LLM", err.Error())
			break
		}

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
			// 本轮结束前推送上下文占用快照，覆盖最终 assistant 回复的增量
			emitContextUsageWeb(s, payload.SystemPrompt, toolList)
			em.EmitDone()
			state.TransitionReason = nil
			break
		}

		logs.Info("Model requested tool call", zap.Int("tool count", len(response.Message.ToolCalls)))

		// If reasoning is not empty, render to user
		if response.Message.ReasoningContent != "" {
			em.EmitReasoning(response.Message.ReasoningContent)
		}

		// Batch tools based on concurrency safety
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

		// Render all tool calls
		for _, tc := range toolCalls {
			if tc.Name == "sub_task" {
				prompt, _ := tc.Arguments["prompt"].(string)
				em.EmitSubAgent(prompt)
				continue
			}
			em.EmitToolCall(tc.Name, formatArgsPreview(tc.Arguments))
		}

		// Check if the Todo tool was called in this round
		usedTodo := false
		for _, tc := range toolCalls {
			if tc.Name == "todo" {
				usedTodo = true
				break
			}
		}

		// Hook: Trigger EventPreToolUse for each tool before tool execution
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

		// Execute all batches
		allowedCalls, allowedIndex := filterAllowedCalls(toolCalls, preDecisions)
		allowedBatches := tools.PartitionToolCalls(allowedCalls)
		allowedResults := tools.ExecuteBatches(allowedBatches, registry, toolCtx)
		results := mergeToolResults(toolCalls, preDecisions, allowedIndex, allowedResults)

		// Write Tool Result back to state.messages in the order of call
		for resultIndex, result := range results {
			logs.Info("tool call finished",
				zap.String("tool name", toolCalls[resultIndex].Name),
				zap.String("args", formatArgsPreview(toolCalls[resultIndex].Arguments)),
				zap.String("result", result.Content),
			)

			// Render tool result summary
			if preDecisions[resultIndex].ExitCode != hook.ExitBlock {
				em.EmitToolResult(toolCalls[resultIndex].Name, result.Content, result.IsError)
			}

			// If todo tool was called and succeeded, render the latest plan panel to user
			if toolCalls[resultIndex].Name == "todo" && result.Ok {
				em.EmitTodoPanel(todoManager.Render(), todoManager.Items())
			}

			// Context compact: Layer 1 (large output persistence): when a single tool result is too large, write full content to disk and keep only a preview in the message
			persistedContent := compactManager.PersistLargeOutput(toolCalls[resultIndex].ID, result.Content)
			if persistedContent != result.Content {
				logs.Info("tool result content too large, persisted to disk and replaced with preview",
					zap.String("tool name", toolCalls[resultIndex].Name),
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

		// Full Context compact
		// Determine whether to trigger full compression after all tool results have been appended to state.messages
		// After successful compression, state.Messages will be replaced with system + a continuity summary
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

		// Limit the maximum number of rounds to avoid infinite loops
		if state.TurnCount >= cfg.AgentLoop.MaxTurns {
			logs.Warn("Reaching the maximum round, stop the loop", zap.Int("max_turns", cfg.AgentLoop.MaxTurns))
			em.EmitInfo(fmt.Sprintf("reached max turns (%d), stop", cfg.AgentLoop.MaxTurns))
			break
		}
	}
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
	fullMessages []fsm.Message, toolList []tools.Tool, options map[string]interface{},
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

		// 2. Chat with LLM
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
func estimateTokens(messages []fsm.Message) int {
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
