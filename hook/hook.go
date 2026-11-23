package hook

import (
	"zoomClient/logger"

	"go.uber.org/zap"
)

const (
	EventSessionStart = "SessionStart" // 会话开始
	EventPreChat      = "PreChat"      // LLM 调用前
	EventPostChat     = "PostChat"     // LLM 调用后
	EventLLMError     = "LLMError"     // LLM 调用失败
	EventPreToolUse   = "PreToolUse"   // 工具执行前
	EventPostToolUse  = "PostToolUse"  // 工具执行后
	EventToolError    = "ToolError"    // 工具执行错误
	EventSessionEnd   = "SessionEnd"   // 会话结束
)

const (
	ExitContinue = 0 // 继续
	ExitBlock    = 1 // 阻断
	ExitInject   = 2 // 注入
	ExitRetry    = 3 // 重试
)

type HookHandlerFunc = func(payload map[string]any) HookResult

type HookResult struct {
	ExitCode int
	Message  string
}

type Runner struct {
	handlers map[string][]HookHandlerFunc
}

func NewRunner() *Runner {
	return &Runner{
		handlers: make(map[string][]HookHandlerFunc),
	}
}

// HookRegister 为特定事件注册一个钩子处理器
func (r *Runner) HookRegister(event string, handler HookHandlerFunc) {
	r.handlers[event] = append(r.handlers[event], handler)
}

// HookRun 运行特定事件的所有钩子处理器
func (r *Runner) HookRun(event string, args map[string]any) HookResult {

	handlers, exists := r.handlers[event]
	if !exists || len(handlers) == 0 {
		return HookResult{ExitCode: ExitContinue}
	}

	for _, handlerFuncs := range r.handlers[event] {
		result := handlerFuncs(args)
		if result.ExitCode != ExitContinue {
			log := logger.Log
			log.Debug("[hook] Handler returns non-zero exit code",
				zap.String("event", event),
				zap.Int("exit_code", result.ExitCode),
				zap.String("message", result.Message),
			)
			return result
		}
	}

	return HookResult{ExitCode: ExitContinue}
}

func (r *Runner) HandlerCount(event string) int {
	return len(r.handlers[event])
}
