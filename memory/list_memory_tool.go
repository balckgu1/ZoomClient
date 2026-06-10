package memory

import (
	"os"
	"path/filepath"
	"zoomClient/tools"
)

type ListMemoryTool struct {
	memoryDir string
}

func NewListMemoryTool(memoryDir string) *ListMemoryTool {
	return &ListMemoryTool{memoryDir: memoryDir}
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
	// Check Memory Directory parameter
	if l.memoryDir == "" {
		return tools.ToolResult{Ok: false, Content: "Error: MemoryDir is not configured", IsError: true}
	}

	// read MEMORY.md
	indexPath := filepath.Join(l.memoryDir, "MEMORY.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return tools.ToolResult{Ok: false, Content: "Error: failed to read MEMORY.md", IsError: true}
	}
	return tools.ToolResult{Ok: true, Content: string(data), IsError: false}
}
