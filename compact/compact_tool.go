package compact

import (
	"zoomClient/tools"

	"go.uber.org/zap"
)

// CompactTool 把 触发完整压缩 封装成tool
type CompactTool struct {
	manager *CompactManager
}

func NewCompactTool(m *CompactManager) *CompactTool {
	return &CompactTool{manager: m}
}

func (t *CompactTool) Name() string {
	return "compact"
}

func (t *CompactTool) Description() string {
	return "Manually request a full conversation compaction. " +
		"Use this when the dialog has grown long and you want to free up active context " +
		"while preserving continuity (current goal, completed actions, touched files, " +
		"key decisions, next step). The actual compaction happens at the end of this turn."
}

func (t *CompactTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

// Call 标记一次手动压缩请求
func (t *CompactTool) Call(args map[string]interface{}, ctx *tools.ToolContext) tools.ToolResult {
	t.manager.RequestManualCompact()
	ctx.Logger.Info("complete compression", zap.String("session", ctx.SessionID))
	return tools.ToolResult{
		Ok: true,
		Content: "The manual compression request has been marked. " +
			"At the end of this round, complete compression will be performed and the session history will be replaced with a continuity summary.",
	}
}
