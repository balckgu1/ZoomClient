package hook

import (
	"zoomClient/logger"

	"go.uber.org/zap"
)

const (
	EventSessionStart = "SessionStart" // Session start
	EventPreToolUse   = "PreToolUse"   // Tool execution before
	EventPostToolUse  = "PostToolUse"  // Tool execution after
	EventToolError    = "ToolError"    // Tool execution error
	EventSessionEnd   = "SessionEnd"   // Session end
)

const (
	ExitContinue = 0 // Continue
	ExitBlock    = 1 // Block
	ExitInject   = 2 // Inject
	ExitRetry    = 3 // Retry
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

// HookRegister Register a hook handler for a specific event
func (r *Runner) HookRegister(event string, handler HookHandlerFunc) {
	r.handlers[event] = append(r.handlers[event], handler)
}

// HookRun Run all hook handlers for a specific event
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
