package subagent

import (
	"context"
	"errors"
	"fmt"
	"zoomClient/clients"
	"zoomClient/fsm"
	"zoomClient/logger"
	"zoomClient/tools"

	"go.uber.org/zap"
)

// ErrMaxTurnsReached 子智能体达到最大轮数仍未收敛时返回此错误
var ErrMaxTurnsReached = errors.New("subagent reached max turns without convergence")

// DefaultMaxTurns 子智能体默认最大轮数
const DefaultMaxTurns = 10

const DefaultTemperature = 0.3

// DefaultSystemPrompt 子智能体默认 system prompt
const DefaultSystemPrompt = "You are a focused subagent running inside an isolated context. " +
	"Use the provided tools to complete the user's subtask, then return ONE concise summary (a single sentence or short paragraph) as your final answer. " +
	"Do NOT paste tool call transcripts into the final answer; only return the essential result that the parent agent needs."

type SubAgent struct {
	Client                  clients.ChatClient     // 复用父的 LLM 客户端
	Model                   string                 // 模型名
	SystemPrompt            string                 // subagent 专用 system prompt
	ForkSubtaskPromptPrefix string                 // fork 模式下注入到父消息末尾的子任务引导前缀
	Registry                *tools.Registry        // subagent 可用工具
	ToolCtx                 *tools.ToolContext     // 工具执行上下文
	MaxTurns                int                    // 最大轮数，<=0 时使用 DefaultMaxTurns
	Options                 map[string]interface{} // 采样参数
}

// NewSubAgent 初始化 subagent
func NewSubAgent(client clients.ChatClient, model string, systemPrompt string, forkSubtaskPromptPrefix string,
	registry *tools.Registry, toolCtx *tools.ToolContext, maxTurns int, options map[string]interface{}) *SubAgent {
	return &SubAgent{
		Client:                  client,
		Model:                   model,
		SystemPrompt:            systemPrompt,
		ForkSubtaskPromptPrefix: forkSubtaskPromptPrefix,
		Registry:                registry,
		ToolCtx:                 toolCtx,
		MaxTurns:                maxTurns,
		Options:                 options,
	}
}

// Run 以空白上下文运行子智能体
func (subagent *SubAgent) Run(prompt string) (string, error) {
	// 检查 subagent 依赖
	if err := subagent.validateDependencies(); err != nil {
		return "", err
	}

	// system prompt + 子任务 user prompt
	systemPrompt := subagent.resolveSystemPrompt()
	messages := []fsm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}

	logInfo("subagent init (blank)", zap.String("prompt", prompt))
	return subagent.subagentLoop(messages)
}

// deepCopyMessages 深拷贝消息切片
func deepCopyMessages(messages []fsm.Message) []fsm.Message {
	return append([]fsm.Message(nil), messages...)
}

// RunWithFork 以 fork 模式运行子智能体
// 继承父 agent 的消息上下文，在其末尾追加当前子任务 prompt
func (subagent *SubAgent) RunWithFork(prompt string, parentMessages []fsm.Message) (string, error) {
	if err := subagent.validateDependencies(); err != nil {
		return "", err
	}

	// 字段级深复制 parentMessages，避免子智能体追加/修改污染父消息
	forked := make([]fsm.Message, len(parentMessages))
	for i, m := range parentMessages {
		forked[i] = deepCopyMessage(m)
	}

	// 裁剪末尾的 assistant 消息（它就是触发本次 sub_task 调用的那一轮）
	if len(forked) > 0 && forked[len(forked)-1].Role == "assistant" {
		forked = forked[:len(forked)-1]
	}

	// 末尾追加 fork 子任务 user 消息
	forked = append(forked, fsm.Message{
		Role:    "user",
		Content: subagent.ForkSubtaskPromptPrefix + prompt,
	})

	logInfo("subagent init (fork)",
		zap.String("prompt", prompt),
		zap.Int("parent_message_count", len(parentMessages)),
		zap.Int("forked_message_count", len(forked)),
	)
	return subagent.subagentLoop(forked)
}

// validateDependencies 校验subagent依赖
func (subagent *SubAgent) validateDependencies() error {
	if subagent.Client == nil {
		return fmt.Errorf("subagent client is nil")
	}
	if subagent.Registry == nil {
		return fmt.Errorf("subagent registry is nil")
	}
	return nil
}

// resolveSystemPrompt 返回 system prompt
func (subagent *SubAgent) resolveSystemPrompt() string {
	if subagent.SystemPrompt == "" {
		return DefaultSystemPrompt
	}
	return subagent.SystemPrompt
}

// resolveMaxTurns 返回生效的最大轮数
func (subagent *SubAgent) resolveMaxTurns() int {
	if subagent.MaxTurns <= 0 {
		return DefaultMaxTurns
	}
	return subagent.MaxTurns
}

func (subagent *SubAgent) resolveTemperature() map[string]interface{} {
	if subagent.Options == nil {
		return map[string]interface{}{
			"temperature": DefaultTemperature,
		}
	}
	return subagent.Options
}

// runLoop 子智能体 loop
func (subagent *SubAgent) subagentLoop(messages []fsm.Message) (string, error) {
	maxTurns := subagent.resolveMaxTurns()
	toolList := subagent.Registry.GetAll()
	options := subagent.resolveTemperature()

	// 从 ToolCtx 取 context，用于响应外部取消
	var ctx context.Context
	if subagent.ToolCtx != nil && subagent.ToolCtx.Ctx != nil {
		ctx = subagent.ToolCtx.Ctx
	}

	logInfo("subagent loop start",
		zap.Int("max_turns", maxTurns),
		zap.Int("tool_count", len(toolList)),
		zap.Int("initial_message_count", len(messages)),
	)

	// 记录最后一次 assistant 文本，达到 MaxTurns 时作为兜底摘要的一部分
	var lastAssistantText string

	for turn := 0; turn < maxTurns; turn++ {
		// 检查 context 是否已取消
		if ctx != nil {
			select {
			case <-ctx.Done():
				logWarn("subagent cancelled by context",
					zap.Int("turn", turn),
					zap.Error(ctx.Err()),
				)
				return "", fmt.Errorf("subagent cancelled: %w", ctx.Err())
			default:
			}
		}

		// chat with llm
		response, err := subagent.Client.Chat(subagent.Model, messages, toolList, options)
		if err != nil {
			return "", fmt.Errorf("subagent chat error: %w", err)
		}

		// 安全断言
		if contentStr, ok := response.Message.Content.(string); ok && contentStr != "" {
			lastAssistantText = contentStr
		}

		messages = append(messages, fsm.Message{
			Role:             "assistant",
			Content:          response.Message.Content,
			ToolCalls:        response.Message.ToolCalls,
			ToolCallID:       response.Message.ToolCallID,
			ReasoningContent: response.Message.ReasoningContent,
		})

		if len(response.Message.ToolCalls) == 0 {
			summary, ok := response.Message.Content.(string)
			if !ok || summary == "" {
				summary = lastAssistantText
				logWarn("subagent end with empty summary", zap.Int("turn", turn))
			}
			logInfo("subagent end", zap.Int("turn", turn), zap.String("summary", summary))
			return summary, nil
		}

		logInfo("subagent execve tool", zap.Int("turn", turn), zap.Int("tool_call_count", len(response.Message.ToolCalls)))

		results := tools.ExecuteToolCalls(response.Message.ToolCalls, subagent.Registry, subagent.ToolCtx)

		for i, result := range results {
			messages = append(messages, fsm.Message{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: response.Message.ToolCalls[i].ID,
			})
		}
	}

	// 达到 MaxTurns 仍未收敛，优雅截断并返回 ErrMaxTurnsReached
	truncatedSummary := fmt.Sprintf("<subagent truncated: reached max turns (%d)>", maxTurns)
	if lastAssistantText != "" {
		truncatedSummary += " last assistant text: " + lastAssistantText
	}
	logWarn("subagent has reached the maximum number of turns", zap.Int("max_turns", maxTurns))
	return truncatedSummary, ErrMaxTurnsReached
}

// BuildSubAgentRegistry 构建子智能体工具注册表
func BuildSubAgentRegistry() *tools.Registry {
	reg := tools.NewRegistry()
	reg.Register(tools.ReadFileTool{})
	reg.Register(tools.ListDirectory{})
	reg.Register(tools.RunBashTool{})
	reg.Register(&tools.GlobSearch{})
	return reg
}

// logInfo 封装：logger 未初始化时（如单元测试环境）静默跳过，避免 nil panic
func logInfo(msg string, fields ...zap.Field) {
	if logger.Log != nil {
		logger.Log.Info(msg, fields...)
	}
}

// logWarn 封装：logger 未初始化时（如单元测试环境）静默跳过，避免 nil panic
func logWarn(msg string, fields ...zap.Field) {
	if logger.Log != nil {
		logger.Log.Warn(msg, fields...)
	}
}

// deepCopyMessage 对 fsm.Message 做字段级深拷贝
func deepCopyMessage(m fsm.Message) fsm.Message {
	copied := m
	// 深拷贝 ToolCalls 切片
	if len(m.ToolCalls) > 0 {
		copied.ToolCalls = make([]tools.ToolCall, len(m.ToolCalls))
		copy(copied.ToolCalls, m.ToolCalls)
	}
	// 深拷贝 Content：如果是 []interface{}，做切片复制
	if slice, ok := m.Content.([]interface{}); ok {
		copiedContent := make([]interface{}, len(slice))
		copy(copiedContent, slice)
		copied.Content = copiedContent
	}
	return copied
}
