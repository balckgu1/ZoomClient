package subagent

import (
	"fmt"
	"zoomClient/clients"
	"zoomClient/fsm"
	"zoomClient/logger"
	"zoomClient/tools"

	"go.uber.org/zap"
)

// DefaultMaxTurns 子智能体默认最大轮数，防止无限循环
const DefaultMaxTurns = 5

// DefaultSystemPrompt 子智能体默认 system prompt
const DefaultSystemPrompt = "You are a focused subagent running inside an isolated context. " +
	"Use the provided tools to complete the user's subtask, then return ONE concise summary (a single sentence or short paragraph) as your final answer. " +
	"Do NOT paste tool call transcripts into the final answer; only return the essential result that the parent agent needs."

type SubAgent struct {
	Client                  clients.ChatClient // 复用父的 LLM 客户端
	Model                   string             // 模型名
	SystemPrompt            string             // 子专用 system prompt，为空则使用 DefaultSystemPrompt
	ForkSubtaskPromptPrefix string             // fork 模式下注入到父消息末尾的子任务引导前缀
	Registry                *tools.Registry    // 子专用工具注册表（白名单）
	ToolCtx                 *tools.ToolContext // 工具执行上下文（可与父共享工作目录）
	MaxTurns                int                // 最大轮数，<=0 时使用 DefaultMaxTurns
}

// Run 以空白上下文运行子智能体
func (subagent *SubAgent) Run(prompt string) (string, error) {
	if err := subagent.validateDependencies(); err != nil {
		return "", err
	}

	// 空白上下文：system prompt + 子任务 user prompt
	systemPrompt := subagent.resolveSystemPrompt()
	messages := []fsm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}

	logInfo("subagent init (blank)",
		zap.String("prompt", prompt),
	)
	return subagent.runLoop(messages)
}

// RunWithFork 以 fork 模式运行子智能体
// 继承父 agent 的消息上下文，在其末尾追加当前子任务 prompt
//
//  1. 深复制父消息切片，子智能体的追加不会写回父
//  2. 若父最后一条是 assistant（即触发本次 sub_task 调用的那一轮），将其裁剪掉
//     —— 避免子智能体"看见自己正在被调用"，并规避 DeepSeek 协议下 tool_calls
//     未配对 tool 结果就跟随 user 消息的违规序列
//  3. 末尾追加 fork 子任务 user 消息，使用 ForkSubtaskPromptPrefix 强化引导
//  4. fork 模式不再额外插入子智能体自己的 system prompt：父消息已包含 system
func (subagent *SubAgent) RunWithFork(prompt string, parentMessages []fsm.Message) (string, error) {
	if err := subagent.validateDependencies(); err != nil {
		return "", err
	}

	// 深复制parentAgent消息切片
	forked := make([]fsm.Message, 0, len(parentMessages)+1)
	forked = append(forked, parentMessages...)

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
	return subagent.runLoop(forked)
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

// resolveSystemPrompt 返回生效的 system prompt
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

// runLoop 子智能体核心循环
// 入参 messages 为本次运行的初始消息列表（已由调用方构造完成）
func (subagent *SubAgent) runLoop(messages []fsm.Message) (string, error) {
	maxTurns := subagent.resolveMaxTurns()
	toolList := subagent.Registry.GetAll()

	logInfo("subagent loop start",
		zap.Int("max_turns", maxTurns),
		zap.Int("tool_count", len(toolList)),
		zap.Int("initial_message_count", len(messages)),
	)

	// 记录最后一次 assistant 文本，达到 MaxTurns 时作为兜底摘要的一部分
	var lastAssistantText string

	for turn := 0; turn < maxTurns; turn++ {
		response, err := subagent.Client.Chat(subagent.Model, messages, toolList, map[string]interface{}{"temperature": 0.3})
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
			summary, _ := response.Message.Content.(string)
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

	// 达到 MaxTurns 仍未收敛，优雅截断
	truncatedSummary := fmt.Sprintf("<subagent truncated: reached max turns (%d)>", maxTurns)
	if lastAssistantText != "" {
		truncatedSummary += " last assistant text: " + lastAssistantText
	}
	logWarn("subagent has reached the maximum number of turns", zap.Int("max_turns", maxTurns))
	return truncatedSummary, nil
}

// NewSubAgent 创建子智能体
func NewSubAgent(client clients.ChatClient, model string, systemPrompt string, forkSubtaskPromptPrefix string,
	registry *tools.Registry, toolCtx *tools.ToolContext, maxTurns int) *SubAgent {
	return &SubAgent{
		Client:                  client,
		Model:                   model,
		SystemPrompt:            systemPrompt,
		ForkSubtaskPromptPrefix: forkSubtaskPromptPrefix,
		Registry:                registry,
		ToolCtx:                 toolCtx,
		MaxTurns:                maxTurns,
	}
}

// BuildSubAgentRegistry 构建子智能体工具注册表
// 明确包含：
//   - read_file：只读文件，最常用的子任务工具
//   - run_bash：按文档建议提供 shell 能力（项目未实现"只读 bash"，使用方可视需要收紧）
//
// 明确排除：
//   - task：防止子智能体继续派生子智能体，避免无限递归
//   - write_file / edit_file：防止子智能体产生副作用打穿隔离
//   - todo：子任务通常足够简短，不鼓励子智能体自行规划
func BuildSubAgentRegistry() *tools.Registry {
	reg := tools.NewRegistry()
	reg.Register(tools.ReadFileTool{})
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

// logInfo 封装：logger 未初始化时（如单元测试环境）静默跳过，避免 nil panic
func logWarn(msg string, fields ...zap.Field) {
	if logger.Log != nil {
		logger.Log.Warn(msg, fields...)
	}
}
