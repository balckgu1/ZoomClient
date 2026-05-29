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
	return "Save a persistent memory that survives across sessions."
}

func (m *SaveMemoryTool) Parameters() map[string]interface{} {
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

	content, exist := args["content"].(string)
	if !exist {
		return tools.ToolResult{Ok: false, Content: "Error: content parameter is required", IsError: true}
	}

	// Front Matter + Content
	fileContent := fmt.Sprintf("---\nname: %s\ndescription: %s\ntype: %s\n---\n%s\n", name, description, typ, content)

	// Check Memory Directory parameter
	if m.MemoryDir == "" {
		return tools.ToolResult{Ok: false, Content: "Error: MemoryDir is not configured", IsError: true}
	}

	// Create Memory Directory if not exists
	if err := os.MkdirAll(m.MemoryDir, 0755); err != nil {
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: failed to create memory directory: %v", err),
			IsError: true,
		}
	}

	// Generate a secure file path
	safeName := sanitizeFilename(name)
	filePath := filepath.Join(m.MemoryDir, safeName+".md")

	toolCtx.Logger.Info("Saving memory", zap.String("session", toolCtx.SessionID), zap.String("path: ", filePath))

	// Write file to disk
	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: failed to write memory file: %v", err),
			IsError: true,
		}
	}

	// rebuild MEMORY.md index
	rebuildIndex(m.MemoryDir)

	return tools.ToolResult{
		Ok:      true,
		Content: fmt.Sprintf("Memory saved successfully to %s", filePath),
		IsError: false,
	}
}

// rebuildIndex Rebuild the MEMORY.md index file, listing all valid memory entries in memoryDir.
// Format: - name: description [type]
func rebuildIndex(memoryDir string) {
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		return
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
	_ = os.WriteFile(indexPath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

// sanitizeFilename clean the filename by removing unsafe characters
func sanitizeFilename(name string) string {
	// Replace common unsafe characters
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
	// Prevent empty file names
	if safe == "" {
		safe = "unnamed_memory"
	}
	return safe
}
