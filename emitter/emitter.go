// emitter/emitter.go
//
// Emitter 定义了 agentLoop 输出事件的统一接口。
// CLI 模式由 ui.Renderer 实现（终端彩色渲染），API 模式由 ApiEmitter 实现（NDJSON stdout）。
package emitter

// Emitter 是 agentLoop 所有用户可见事件的输出抽象
type Emitter interface {
	// Session生命周期

	// EmitSessionStart 通知会话启动
	EmitSessionStart(model, logPath string)
	// EmitSessionEnd 通知会话结束
	EmitSessionEnd(totalTurns int)
	// EmitTurnSeparator 在两轮对话之间画分隔
	EmitTurnSeparator()

	// Agent 输出

	// EmitAssistant 输出助手最终文本。
	EmitAssistant(text string)
	// EmitReasoning 输出 thinking/reasoning 内容。
	EmitReasoning(text string)
	// EmitDone 标记当前回合的 agent 输出结束。
	EmitDone()

	// 工具事件

	// EmitToolCall 通知即将调用某个工具。
	EmitToolCall(name, argsPreview string)
	// EmitToolResult 通知工具执行结果。
	EmitToolResult(name, content string, isError bool)
	// EmitSubAgent 通知子智能体调用。
	EmitSubAgent(promptPreview string)
	// EmitHookBlocked 通知 hook 拒绝了某个工具调用。
	EmitHookBlocked(toolName, reason string)

	// 计划 / 压缩

	// EmitTodoPanel 输出当前任务计划面板。
	EmitTodoPanel(rendered string)
	// EmitCompact 通知上下文压缩已触发。
	EmitCompact(beforeBytes, afterBytes int)

	// 系统 / 控制

	// EmitError 输出用户可见错误
	EmitError(scope, msg string)
	// EmitInfo 输出普通提示信息。
	EmitInfo(msg string)
	// EmitEmotion 输出状态变更事件, API、Web 模式用，CLI 模式 no-op
	EmitEmotion(state string, meta map[string]string)
	// EmitSystem 输出系统级事件（heartbeat/ready/ack 等，API 模式专用，CLI 模式 no-op）。
	EmitSystem(event string, data map[string]string)
}
