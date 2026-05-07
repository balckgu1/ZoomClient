package subagent

import (
	"zoomClient/fsm"
	"zoomClient/tools"
)

// SubAgentRunner 子智能体运行器签名
type SubAgentRunner func(prompt string, parentMessages []fsm.Message) (string, error)

// ParentMessagesProvider 父消息提供者
type ParentMessagesProvider func() []fsm.Message

// TaskTool 实现 Tool 接口，把子任务委托给子智能体执行
type TaskTool struct {
	runFn                  SubAgentRunner         // 子智能体运行器
	parentMessagesProvider ParentMessagesProvider // 父消息提供者（fork=true 时必需）
}

// NewTaskTool 创建 subtask tool
//
//   - runFn：子智能体运行器，必须非 nil
//   - parentMessagesProvider：父消息提供者，可为 nil；若为 nil 则 fork=true 的调用会返回错误
func NewTaskTool(runFn SubAgentRunner, parentMessagesProvider ParentMessagesProvider) *TaskTool {
	return &TaskTool{
		runFn:                  runFn,
		parentMessagesProvider: parentMessagesProvider,
	}
}

// Name 工具名称
func (t *TaskTool) Name() string {
	return "sub_task"
}

// Description 工具描述, 说明 fork 的语义，引导模型在需要父context时显式传 fork=true
func (t *TaskTool) Description() string {
	return "Delegate a focused subtask to a subagent running in an isolated context; returns a concise summary of the subagent's result. " +
		"Use this for narrow, self-contained questions (e.g., reading multiple files and summarizing one fact) so the parent context stays clean. " +
		"By default the subagent starts with a BLANK context and cannot see the parent conversation, so the prompt must be fully self-contained. " +
		"Set fork=true when the subtask must build upon the ongoing parent conversation (e.g., 'based on the plan we just discussed, write tests for it'); " +
		"in fork mode the subagent inherits the parent messages and then receives the subtask prompt."
}

// Parameters 工具参数 JSON Schema
func (t *TaskTool) Parameters() map[string]any {
	parameters := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "Subtask description. When fork=false it must be a self-contained instruction independent of the parent context; when fork=true it can reference the prior conversation.",
			},
			"fork": map[string]any{
				"type":        "boolean",
				"description": "When true, the subagent inherits the parent conversation context before executing the subtask. Default false (blank context).",
				"default":     false,
			},
		},
		"required": []string{"prompt"},
	}
	return parameters
}

// Call 工具执行入口
func (t *TaskTool) Call(args map[string]any, ctx *tools.ToolContext) tools.ToolResult {
	if t.runFn == nil {
		return tools.ToolResult{Ok: false, Content: "Error: task tool has no runner configured", IsError: true}
	}

	// 解析 prompt
	promptRaw, exists := args["prompt"]
	if !exists {
		return tools.ToolResult{Ok: false, Content: "Error: missing prompt parameter", IsError: true}
	}
	prompt, ok := promptRaw.(string)
	if !ok || prompt == "" {
		return tools.ToolResult{Ok: false, Content: "Error: prompt parameter must be a non-empty string", IsError: true}
	}

	// 解析 fork
	fork := parseBoolArg(args, "fork")

	// fork 模式下取父消息快照
	var parentMessages []fsm.Message
	if fork {
		if t.parentMessagesProvider == nil {
			return tools.ToolResult{
				Ok:      false,
				Content: "Error: fork=true requested but no parent messages provider is configured",
				IsError: true,
			}
		}
		parentMessages = t.parentMessagesProvider()
		if len(parentMessages) == 0 {
			return tools.ToolResult{
				Ok:      false,
				Content: "Error: fork=true requested but parent messages are empty",
				IsError: true,
			}
		}
	}

	summary, err := t.runFn(prompt, parentMessages)
	if err != nil {
		return tools.ToolResult{Ok: false, Content: "Error: " + err.Error(), IsError: true}
	}

	return tools.ToolResult{Ok: true, Content: summary, IsError: false}
}

// parseBoolArg 从参数 map 中安全解析布尔字段
func parseBoolArg(args map[string]any, key string) bool {
	raw, exists := args[key]
	if !exists {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "True" || v == "TRUE"
	default:
		return false
	}
}
