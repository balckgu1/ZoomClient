package hook

import (
	"zoomClient/logger"

	"go.uber.org/zap"
)

const (
	ExitContinue = 0 // Continue
	ExitBlock    = 1 // Block
	ExitInject   = 2 // Inject
	ExitRetry    = 3 // Retry
)

const (
	EventSessionStart = "SessionStart" // Session start
	EventPreToolUse   = "PreToolUse"   // Tool execution before
	EventPostToolUse  = "PostToolUse"  // Tool execution after
	EventToolError    = "ToolError"    // Tool execution error
	EventSessionEnd   = "SessionEnd"   // Session end
)

type Handler func(payload map[string]any) HookResult

// HookResult Define the hook handler result structure
type HookResult struct {
	ExitCode      int
	Message       string
	ModifiedInput map[string]any // When ExitCode = 3, modify the input
}

// Runner Define the Runner class
type Runner struct {
	handlers map[string][]Handler // map{event: []Handler}
}

// NewRunner Construction method of runner
func NewRunner() *Runner {
	return &Runner{
		handlers: make(map[string][]Handler),
	}
}

// Register Mount an event to a specified handler
//
// Same event can mount multiple handlers, executed in the order of registration.
func (r *Runner) Register(event string, handler Handler) {
	r.handlers[event] = append(r.handlers[event], handler)
}

// Run Trigger specified event and execute all handlers mounted to that event
func (r *Runner) Run(event string, payload map[string]any) HookResult {
	log := logger.Log

	handlers, exists := r.handlers[event]
	if !exists || len(handlers) == 0 {
		return HookResult{ExitCode: ExitContinue}
	}

	for _, handler := range r.handlers[event] {
		result := handler(payload)
		if result.ExitCode != ExitContinue {
			log.Debug("[hook] handler 返回非零退出码",
				zap.String("event", event),
				zap.Int("exit_code", result.ExitCode),
				zap.String("message", result.Message),
			)
			return result
		}
	}
	return HookResult{ExitCode: ExitContinue}
}

// HandlerCount Return number of handlers mounted to specified event
func (r *Runner) HandlerCount(event string) int {
	return len(r.handlers[event])
}
