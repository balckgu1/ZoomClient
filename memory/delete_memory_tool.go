package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"zoomClient/tools"

	"go.uber.org/zap"
)

type DeleteMemoryTool struct {
	memoryDir string
}

func NewDeleteMemoryTool(memoryDir string) *DeleteMemoryTool {
	return &DeleteMemoryTool{memoryDir: memoryDir}
}

func (d *DeleteMemoryTool) Name() string {
	return "delete_memory"
}

func (d *DeleteMemoryTool) Description() string {
	return "Delete memory based on the provided name"
}

func (d *DeleteMemoryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The name of memory",
			},
		},
		"required": []string{"name"},
	}
}

func (d *DeleteMemoryTool) Call(args map[string]any, toolCtx *tools.ToolContext) tools.ToolResult {
	nameRaw, exist := args["name"]
	if !exist {
		return tools.ToolResult{Ok: false, Content: "Error: name parameter is required", IsError: true}
	}
	name, ok := nameRaw.(string)
	if !ok || strings.TrimSpace(name) == "" {
		return tools.ToolResult{Ok: false, Content: "Error: name parameter must be a non-empty string", IsError: true}
	}

	// Check Memory Directory parameter
	if d.memoryDir == "" {
		return tools.ToolResult{Ok: false, Content: "Error: MemoryDir is not configured", IsError: true}
	}
	//从MEMORY.md中查找该name，若没找到，说明不存在，返回错误
	indexPath := filepath.Join(d.memoryDir, "MEMORY.md")
	exist, err := MemoryExists(indexPath, name)
	if err != nil {
		return tools.ToolResult{Ok: false, Content: "Error: " + err.Error(), IsError: true}
	}
	if !exist {
		return tools.ToolResult{Ok: false, Content: "Error: The memory does not exist.", IsError: true}
	}
	toolCtx.Logger.Info("Delete memory", zap.String("session", toolCtx.SessionID), zap.String("name", name))

	// 删除该memory文件
	safeName := sanitizeFilename(name)
	filePath := filepath.Join(d.memoryDir, safeName+".md")

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return tools.ToolResult{Ok: false, Content: "Error: memory file not found on disk", IsError: true}
		}
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: failed to delete memory file: %v", err),
			IsError: true,
		}
	}

	// 重建 MEMORY.md 索引
	err = rebuildIndex(d.memoryDir)
	if err != nil {
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: failed to rebuild memory index: %v", err),
			IsError: true,
		}
	}

	return tools.ToolResult{
		Ok:      true,
		Content: fmt.Sprintf("Memory '%s' deleted successfully.", name),
		IsError: false,
	}
}
