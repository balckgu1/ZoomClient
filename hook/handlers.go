package hook

import (
	"fmt"
	"strings"
	"zoomClient/logger"
	"zoomClient/permission"

	"go.uber.org/zap"
)

// OnSessionStart Print session start information
func OnSessionStart(payload map[string]any) HookResult {
	log := logger.Log
	model := payload["model"].(string)
	log.Info("[hook] Session start", zap.String("model", model))
	return HookResult{ExitCode: ExitContinue}
}

// PreToolBlockDangerous 在工具执行前拦截危险的 bash 命令。
//
// 检测到危险命令，返回 exit=1 阻止执行。
func PreToolBlockDangerous(payload map[string]any) HookResult {
	toolName, _ := payload["tool_name"].(string)

	// if not run_bash tool, continue
	if toolName != "run_bash" {
		return HookResult{ExitCode: ExitContinue}
	}

	input, _ := payload["input"].(map[string]any)
	cmd, _ := input["command"].(string)
	lowered := strings.ToLower(cmd)

	// dangerous command patterns（统一数据源，大小写不敏感匹配）
	for _, pattern := range permission.DangerousBashPatterns() {
		if strings.Contains(lowered, pattern) {
			return HookResult{
				ExitCode: ExitBlock,
				Message:  fmt.Sprintf("dangerous command blocked by hook: %s", cmd),
			}
		}
	}

	return HookResult{ExitCode: ExitContinue}
}

// PreToolRateLimit 限制单轮工具调用数量，防止模型失控循环
func PreToolRateLimit(payload map[string]any) HookResult {
	callIndex, ok := payload["call_index"].(int)
	maxTools := payload["max_tools"].(int)
	if !ok {
		return HookResult{ExitCode: ExitContinue}
	}
	if callIndex >= maxTools {
		return HookResult{
			ExitCode: ExitBlock,
			Message:  fmt.Sprintf("This round of tool calls has reached the maximum limit(%d), and the remaining calls have been blocked by hooks", maxTools),
		}
	}
	return HookResult{ExitCode: ExitContinue}
}

// PreToolSensitiveFileGuard 阻止访问敏感文件
func PreToolSensitiveFileGuard(payload map[string]any) HookResult {
	input, ok := payload["input"].(map[string]any)
	if !ok {
		return HookResult{ExitCode: ExitContinue}
	}
	sensitiveFiles, ok := payload["sensitive_files"].([]string)
	if !ok {
		log := logger.Log
		log.Warn("[hook] sensitive_files not found")
		return HookResult{ExitCode: ExitContinue}
	}
	filename, ok := input["filename"].(string)
	if !ok {
		return HookResult{ExitCode: ExitContinue}
	}

	for _, file := range sensitiveFiles {
		if strings.Contains(filename, file) {
			return HookResult{
				ExitCode: ExitBlock,
				Message:  fmt.Sprintf("access to sensitive file %s blocked by hook", file),
			}
		}
	}

	return HookResult{ExitCode: ExitContinue}
}

// PostToolAuditLog 在工具执行后打印审计日志
func PostToolAuditLog(payload map[string]any) HookResult {
	log := logger.Log
	toolName, _ := payload["tool_name"].(string)
	output, _ := payload["output"].(string)

	// 截取前 100 字符作为摘要，避免日志过长
	summary := output
	if len(summary) > 100 {
		summary = summary[:100] + "..."
	}

	log.Debug("[hook] Tool execution completed",
		zap.String("tool", toolName),
		zap.String("output_preview", summary),
	)
	return HookResult{ExitCode: ExitContinue}
}

// PostToolErrorRecovery 在工具执行出错后给模型进行提示
func OnToolErrorRecovery(payload map[string]any) HookResult {
	return HookResult{
		ExitCode: ExitInject,
		Message:  "<internal> The last operation executed error, please consider executing the task in a different way. </internal>",
	}
}

// PreChatAuditLog 在每次 LLM 调用前记录审计信息（模型、消息数、估算 token 数）
func PreChatAuditLog(payload map[string]any) HookResult {
	log := logger.Log
	model, _ := payload["model"].(string)
	messagesCount, _ := payload["messages_count"].(int)
	estTokens, _ := payload["est_tokens"].(int)

	log.Debug("[hook] LLM call started",
		zap.String("model", model),
		zap.Int("messages_count", messagesCount),
		zap.Int("est_tokens", estTokens),
	)
	return HookResult{ExitCode: ExitContinue}
}

// OnLLMErrorRetry 处理 LLM 调用失败：
// 重试预算内返回 ExitRetry 驱动主循环重试；
// 预算耗尽返回 ExitContinue，由主循环按原逻辑上报错误并终止。
func OnLLMErrorRetry(payload map[string]any) HookResult {
	log := logger.Log
	model, _ := payload["model"].(string)
	errMsg, _ := payload["error"].(string)
	retryCount, _ := payload["retry_count"].(int)
	maxRetries, _ := payload["max_retries"].(int)

	if retryCount < maxRetries {
		return HookResult{
			ExitCode: ExitRetry,
			Message:  fmt.Sprintf("<internal> LLM call failed (%s), retrying (%d/%d) </internal>", errMsg, retryCount+1, maxRetries),
		}
	}

	log.Warn("[hook] LLM call failed, retry budget exhausted",
		zap.String("model", model),
		zap.String("error", errMsg),
	)
	return HookResult{ExitCode: ExitContinue}
}

// PostChatValidate 校验 LLM 回复质量：
// 空回复（无文本且无工具调用）在重试预算内返回 ExitRetry 驱动主循环重试；
// 预算耗尽则放行，由主循环按既有逻辑自然结束，避免死循环。
func PostChatValidate(payload map[string]any) HookResult {
	log := logger.Log
	model, _ := payload["model"].(string)
	content, _ := payload["content"].(string)
	toolCallsCount, _ := payload["tool_calls_count"].(int)
	retryCount, _ := payload["retry_count"].(int)
	maxRetries, _ := payload["max_retries"].(int)

	// 空回复 + 无工具调用，判定为异常回复，尝试重试
	if strings.TrimSpace(content) == "" && toolCallsCount == 0 {
		log.Warn("[hook] LLM returned empty response, retrying",
			zap.String("model", model),
			zap.Int("retry_count", retryCount),
			zap.Int("max_retries", maxRetries),
		)
		// 重试次数内返回 ExitRetry 驱动主循环重试
		if retryCount < maxRetries {
			return HookResult{
				ExitCode: ExitRetry,
				Message:  "LLM returned an empty response and did not invoke any tools, retrying",
			}
		}
		// 重试次数耗尽后放行，避免无限重试
		log.Warn("[hook] LLM returned empty response, retry budget exhausted, pass through",
			zap.String("model", model),
			zap.Int("retry_count", retryCount),
			zap.Int("max_retries", maxRetries),
		)
		return HookResult{ExitCode: ExitContinue}
	}

	log.Debug("[hook] LLM call completed",
		zap.String("model", model),
		zap.Int("content_len", len(content)),
		zap.Int("tool_calls_count", toolCallsCount),
	)
	return HookResult{ExitCode: ExitContinue}
}

// // PreToolInjectReminder 在写文件前注入一条提醒消息给模型。
// func PreToolInjectReminder(payload map[string]any) HookResult {
// 	toolName, _ := payload["tool_name"].(string)
// 	if toolName != "write_file" {
// 		return HookResult{ExitCode: ExitContinue}
// 	}

// 	return HookResult{
// 		ExitCode: ExitInject,
// 		Message:  "[hook reminder] Please ensure that the content is correct and the path is secure when writing files.",
// 	}
// }

func OnSessionEnd(payload map[string]any) HookResult {
	log := logger.Log
	turns, _ := payload["total_turns"].(int)
	log.Info("[hook] Session end", zap.Int("total_turns", turns))
	return HookResult{ExitCode: ExitContinue}
}
