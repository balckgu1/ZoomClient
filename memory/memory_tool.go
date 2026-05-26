package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"zoomClient/tools"
)

type SaveMemoryTool struct {
	MemoryDir string
}

func NewSaveMemoryTool(memorypath string) *SaveMemoryTool {
	return &SaveMemoryTool{
		MemoryDir: memorypath,
	}
}

func (m *SaveMemoryTool) Name() string {
	return "save_memory"
}

func (m *SaveMemoryTool) Description() string {
	return "Save memory to file"
}

func (m *SaveMemoryTool) Parameters() map[string]interface{} {
	parameters := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The name of the memory file to save to.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "The description of the memory",
			},
			"type": map[string]any{
				"type":        "string",
				"description": "The type of the memory, It is must be one of the following: 'user' | 'feedback' | 'project' | 'reference'",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The content of the memory file to save to.",
			},
		},
		"required": []string{"name", "description", "type", "content"},
	}

	return parameters
}

func (m *SaveMemoryTool) Call(args map[string]interface{}, toolCtx *tools.ToolContext) tools.ToolResult {
	nameRaw, exist := args["name"]
	if !exist {
		return tools.ToolResult{Ok: false, Content: "Error: name parameter is required", IsError: true}
	}
	name, ok := nameRaw.(string)
	if !ok || strings.TrimSpace(name) == "" {
		return tools.ToolResult{Ok: false, Content: "Error: name parameter must be a non-empty string", IsError: true}
	}
	descriptionRaw, exist := args["description"]
	if !exist {
		return tools.ToolResult{Ok: false, Content: "Error: description parameter is required", IsError: true}
	}
	description, ok := descriptionRaw.(string)
	if !ok || strings.TrimSpace(description) == "" {
		return tools.ToolResult{Ok: false, Content: "Error: description parameter must be a non-empty string", IsError: true}
	}
	typeRaw, exist := args["type"]
	if !exist {
		return tools.ToolResult{Ok: false, Content: "Error: type parameter is required", IsError: true}
	}
	typ, ok := typeRaw.(string)
	if !ok || strings.TrimSpace(typ) == "" {
		return tools.ToolResult{Ok: false, Content: "Error: type parameter must be a non-empty string", IsError: true}
	}
	content, exist := args["content"].(string)
	if !exist {
		return tools.ToolResult{Ok: false, Content: "Error: content parameter is required", IsError: true}
	}

	// 构建 Front Matter + Content
	fileContent := fmt.Sprintf(`---
		name: %s
		description: %s
		type: %s
		---
		%s
		`, name, description, typ, content)

	// 确保存储目录存在
	if m.MemoryDir == "" {
		return tools.ToolResult{Ok: false, Content: "Error: MemoryDir is not configured", IsError: true}
	}

	if err := os.MkdirAll(m.MemoryDir, 0755); err != nil {
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: failed to create memory directory: %v", err),
			IsError: true,
		}
	}

	// 生成安全的文件路径
	safeName := sanitizeFilename(name)
	filePath := filepath.Join(m.MemoryDir, safeName+".md")

	// 写入文件
	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: failed to write memory file: %v", err),
			IsError: true,
		}
	}

	return tools.ToolResult{
		Ok:      true,
		Content: fmt.Sprintf("Memory saved successfully to %s", filePath),
		IsError: false,
	}
}

// sanitizeFilename 清理文件名，移除不安全字符
func sanitizeFilename(name string) string {
	// 替换常见的不安全字符
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	safe := replacer.Replace(name)
	// 防止空文件名
	if safe == "" {
		safe = "unnamed_memory"
	}
	return safe
}
