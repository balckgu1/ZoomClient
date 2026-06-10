package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"zoomClient/tools"

	"go.uber.org/zap"
)

type SaveMemoryTool struct {
	memoryDir string
}

func NewSaveMemoryTool(memorypath string) *SaveMemoryTool {
	return &SaveMemoryTool{
		memoryDir: memorypath,
	}
}

func (m *SaveMemoryTool) Name() string {
	return "save_memory"
}

func (m *SaveMemoryTool) Description() string {
	return "Save a persistent memory that survives across sessions."
}

func (m *SaveMemoryTool) Parameters() map[string]any {
	parameters := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The name of the memory file to save to. Short identifier (e.g. prefer_tabs, db_schema)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "One-line summary of what this memory captures",
			},
			"type": map[string]any{
				"type":        "string",
				"description": "Memory category. Must be one of: 'user' (preferences), 'feedback' (corrections), 'project' (conventions), or 'reference' (resources).",
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

	// Check typ parameter
	validTypes := map[string]bool{"user": true, "feedback": true, "project": true, "reference": true}
	if !validTypes[typ] {
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: type must be one of: user, feedback, project, reference, got: %q", typ),
			IsError: true,
		}
	}

	content, ok := args["content"].(string)
	if !ok {
		return tools.ToolResult{Ok: false, Content: "Error: content parameter is required", IsError: true}
	}

	// Front Matter + Content
	// 对 name 和 description 进行 YAML 安全引用，防止含冒号、特殊字符时解析错误
	fileContent := fmt.Sprintf("---\nname: %s\ndescription: %s\ntype: %s\n---\n%s\n",
		yamlQuote(name), yamlQuote(description), typ, content)

	// Check Memory Directory parameter
	if m.memoryDir == "" {
		return tools.ToolResult{Ok: false, Content: "Error: MemoryDir is not configured", IsError: true}
	}

	// Check if a memory with the same name already exists
	indexPath := filepath.Join(m.memoryDir, "MEMORY.md")
	if exists, _ := MemoryExists(indexPath, name); exists {
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: memory %q already exists. Please use update_memory to modify it.", name),
			IsError: true,
		}
	}

	// Create Memory Directory if not exists
	if err := os.MkdirAll(m.memoryDir, 0755); err != nil {
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: failed to create memory directory: %v", err),
			IsError: true,
		}
	}

	// Generate a secure file path
	safeName := sanitizeFilename(name)
	filePath := filepath.Join(m.memoryDir, safeName+".md")

	toolCtx.Logger.Info("Saving memory", zap.String("session", toolCtx.SessionID), zap.String("path", filePath))

	// Write file to disk
	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: failed to write memory file: %v", err),
			IsError: true,
		}
	}

	// rebuild MEMORY.md index
	err := rebuildIndex(m.memoryDir)
	if err != nil {
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: failed to rebuild memory index: %v", err),
			IsError: true,
		}
	}

	return tools.ToolResult{
		Ok:      true,
		Content: fmt.Sprintf("Memory saved successfully to %s", filePath),
		IsError: false,
	}
}

// rebuildIndex Rebuild the MEMORY.md index file, listing all valid memory entries in memoryDir.
// Format: - name: description [type]
func rebuildIndex(memoryDir string) error {
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		return err
	}

	lines := []string{"# Memory Index\n"}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") || name == "MEMORY.md" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(memoryDir, name))
		if err != nil {
			continue
		}
		doc := ParseFrontMatter(string(data))
		if doc.FrontMatter.Name != "" {
			lines = append(lines, fmt.Sprintf("- %s: %s [%s]",
				doc.FrontMatter.Name, doc.FrontMatter.Description, doc.FrontMatter.Type))
		}
	}

	indexPath := filepath.Join(memoryDir, "MEMORY.md")
	err = os.WriteFile(indexPath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
	if err != nil {
		return err
	}
	return nil
}

// yamlQuote 对 frontmatter value 做安全引用。
func yamlQuote(val string) string {
	const specialChars = `:#{}[],&*!|>'"%@` + "`"
	needsQuote := strings.ContainsAny(val, specialChars) ||
		strings.Contains(val, "\n") ||
		strings.HasPrefix(val, " ") ||
		strings.HasSuffix(val, " ")
	if !needsQuote {
		return val
	}
	// 转义内部的反斜杠和双引号
	escaped := strings.ReplaceAll(val, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	return `"` + escaped + `"`
}

// sanitizeFilename 清理文件名，移除不安全字符并防止路径遍历攻击。
func sanitizeFilename(name string) string {
	// 取 base name，防止路径遍历（如 "../../etc/passwd"）
	name = filepath.Base(name)

	// 替换常见不安全字符
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
		".", "_",
	)
	safe := replacer.Replace(name)

	// 移除连续下划线
	for strings.Contains(safe, "__") {
		safe = strings.ReplaceAll(safe, "__", "_")
	}
	safe = strings.Trim(safe, "_")

	// 检测 Windows 保留文件名
	reserved := map[string]bool{
		"CON": true, "PRN": true, "AUX": true, "NUL": true,
		"COM1": true, "COM2": true, "COM3": true, "COM4": true,
		"COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
		"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true,
		"LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
	}
	if reserved[strings.ToUpper(safe)] {
		safe = "_" + safe
	}

	// 限制文件名最大长度
	const maxLen = 130
	if len(safe) > maxLen {
		safe = safe[:maxLen]
	}

	// 防止空文件名
	if safe == "" {
		safe = "unnamed_memory"
	}
	return safe
}
