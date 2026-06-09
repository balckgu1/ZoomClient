package memory

import "zoomClient/tools"

type ListMemoryTool struct {
	memoryDir string
}

func (l *ListMemoryTool) Name() string {
	return "list_memoey"
}

func (l *ListMemoryTool) Description() string {
	return "List all memories saved across sessions."
}

func (l *ListMemoryTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (l *ListMemoryTool) Call(args map[string]any, toolCtx *tools.ToolContext) tools.ToolResult {
	return tools.ToolResult{Ok: true, Content: "List memory tool called", IsError: false}
}
