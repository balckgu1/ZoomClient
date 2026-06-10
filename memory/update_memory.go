package memory

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"zoomClient/tools"

	"go.uber.org/zap"
)

type UpdateMemoryTool struct {
	memoryDir string
}

func NewUpdateMemoryTool(memoryDir string) *UpdateMemoryTool {
	return &UpdateMemoryTool{memoryDir: memoryDir}
}

func (u *UpdateMemoryTool) Name() string {
	return "update_memory"
}

func (u *UpdateMemoryTool) Description() string {
	return "Update the specified memory with the given content."
}

func (u *UpdateMemoryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The name of memory",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "the new body content, if not provided the original content is retained.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "the new description, if not provided the original description is retained.",
			},
			"type": map[string]any{
				"type":        "string",
				"description": "the new category, if not provided the original category is retained. Must be one of: 'user' (preferences), 'feedback' (corrections), 'project' (conventions), or 'reference' (resources).",
			},
		},
		"required": []string{"name"},
	}
}

func (u *UpdateMemoryTool) Call(args map[string]any, toolCtx *tools.ToolContext) tools.ToolResult {
	nameRaw, exist := args["name"]
	if !exist {
		return tools.ToolResult{Ok: false, Content: "Error: name parameter is required", IsError: true}
	}
	name, ok := nameRaw.(string)
	if !ok || strings.TrimSpace(name) == "" {
		return tools.ToolResult{Ok: false, Content: "Error: name parameter must be a non-empty string", IsError: true}
	}

	// 提取可选参数
	var content, desc, typ string

	if contentRaw, exists := args["content"]; exists {
		c, ok := contentRaw.(string)
		if !ok || strings.TrimSpace(c) == "" {
			return tools.ToolResult{Ok: false, Content: "Error: content parameter must be a non-empty string", IsError: true}
		}
		content = c
	}
	if descRaw, exists := args["description"]; exists {
		d, ok := descRaw.(string)
		if !ok || strings.TrimSpace(d) == "" {
			return tools.ToolResult{Ok: false, Content: "Error: description parameter must be a non-empty string", IsError: true}
		}
		desc = d
	}
	if typeRaw, exists := args["type"]; exists {
		t, ok := typeRaw.(string)
		if !ok || strings.TrimSpace(t) == "" {
			return tools.ToolResult{Ok: false, Content: "Error: type parameter must be a non-empty string", IsError: true}
		}
		// Check typ parameter
		validTypes := map[string]bool{"user": true, "feedback": true, "project": true, "reference": true}
		if !validTypes[t] {
			return tools.ToolResult{
				Ok:      false,
				Content: fmt.Sprintf("Error: type must be one of: user, feedback, project, reference, got: %q", t),
				IsError: true,
			}
		}
		typ = t
	}

	// Check Memory Directory parameter
	if u.memoryDir == "" {
		return tools.ToolResult{Ok: false, Content: "Error: MemoryDir is not configured", IsError: true}
	}
	//从MEMORY.md中查找该name，若没找到，说明不存在，返回错误
	indexPath := filepath.Join(u.memoryDir, "MEMORY.md")
	exist, err := MemoryExists(indexPath, name)
	if err != nil {
		return tools.ToolResult{Ok: false, Content: "Error: " + err.Error(), IsError: true}
	}
	if !exist {
		return tools.ToolResult{Ok: false, Content: "Error: The memory does not exist, please use save_memory to save it directly.", IsError: true}
	}

	// Generate a secure file path
	safeName := sanitizeFilename(name)
	filePath := filepath.Join(u.memoryDir, safeName+".md")
	//check memory file exists

	toolCtx.Logger.Info("Updating memory", zap.String("session", toolCtx.SessionID), zap.String("path", filePath))

	// Read origin memory
	originFileContent, err := os.ReadFile(filePath)
	if err != nil {
		return tools.ToolResult{Ok: false, Content: "Error: " + err.Error(), IsError: true}
	}
	// parse origin memory
	doc := ParseFrontMatter(string(originFileContent))
	// Update memory
	if content != "" {
		doc.Body = content
	}
	if desc != "" {
		doc.FrontMatter.Description = desc
	}
	if typ != "" {
		doc.FrontMatter.Type = typ
	}
	// Front Matter + Content
	fileContent := fmt.Sprintf("---\nname: %s\ndescription: %s\ntype: %s\n---\n%s\n", name, doc.FrontMatter.Description, doc.FrontMatter.Type, doc.Body)
	// Write file to disk
	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: failed to write memory file: %v", err),
			IsError: true,
		}
	}

	// rebuild MEMORY.md index
	err = rebuildIndex(u.memoryDir)
	if err != nil {
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: failed to rebuild memory index: %v", err),
			IsError: true,
		}
	}

	return tools.ToolResult{
		Ok:      true,
		Content: fmt.Sprintf("Memory updated successfully to %s", filePath),
		IsError: false,
	}
}

func MemoryExists(filePath string, name string) (bool, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return false, fmt.Errorf("failed to open MEMORY.md file: %w", err)
	}
	defer f.Close()

	// build prefix
	prefix := "- " + name + ":"

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, prefix) {
			return true, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("failed to read MEMORY.md file: %w", err)
	}

	return false, nil

}
