package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"zoomClient/tools"

	"go.uber.org/zap"
)

type SearchMemoryTool struct {
	memoryDir string
}

func NewSearchMemoryTool(memoryDir string) *SearchMemoryTool {
	return &SearchMemoryTool{memoryDir: memoryDir}
}

func (s *SearchMemoryTool) Name() string {
	return "search_memory"
}

func (s *SearchMemoryTool) Description() string {
	return "Search for memories by keyword. Matches name and description by default; set full_text=true to also search body content. Returns matching memory entries from previous sessions."
}

func (s *SearchMemoryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"keyword": map[string]any{
				"type":        "string",
				"description": "The keyword to search for in memory name and description (and body when full_text is true).",
			},
			"full_text": map[string]any{
				"type":        "boolean",
				"description": "When true, also match the keyword against memory body content (full-text search). Default false matches only name and description.",
			},
		},
		"required": []string{"keyword"},
	}
}

func (s *SearchMemoryTool) Call(args map[string]any, toolCtx *tools.ToolContext) tools.ToolResult {
	keywordRaw, exist := args["keyword"]
	if !exist {
		return tools.ToolResult{Ok: false, Content: "Error: keyword parameter is required", IsError: true}
	}
	keyword, ok := keywordRaw.(string)
	if !ok || strings.TrimSpace(keyword) == "" {
		return tools.ToolResult{Ok: false, Content: "Error: keyword parameter must be a non-empty string", IsError: true}
	}
	keyword = strings.ToLower(strings.TrimSpace(keyword))

	// 检查 memoryDir
	if s.memoryDir == "" {
		return tools.ToolResult{Ok: false, Content: "Error: MemoryDir is not configured", IsError: true}
	}

	// 读取 MEMORY.md
	indexPath := filepath.Join(s.memoryDir, "MEMORY.md")
	if !fileExists(indexPath) {
		return tools.ToolResult{Ok: false, Content: "Error: MEMORY.md does not exist", IsError: true}
	}
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return tools.ToolResult{Ok: false, Content: "Error: failed to read MEMORY.md", IsError: true}
	}

	// full_text 默认 false：仅匹配 name/description，省 IO；
	// 为 true 时读取正文参与匹配，实现全文检索。
	fullText, _ := args["full_text"].(bool)

	entries := parseIndex(string(indexData))

	var results []string
	for _, entry := range entries {
		nameLower := strings.ToLower(entry.Name)
		descLower := strings.ToLower(entry.Description)
		matched := strings.Contains(nameLower, keyword) || strings.Contains(descLower, keyword)

		// 定位正文文件（sanitizeFilename 保证路径安全）
		safeName := sanitizeFilename(entry.Name)
		memoryPath := filepath.Join(s.memoryDir, safeName+".md")

		// full_text 需要读所有正文参与匹配；否则仅在元数据命中后才读正文（省 IO）
		var body string
		bodyExists := false
		if fullText || matched {
			if bodyData, err := os.ReadFile(memoryPath); err == nil {
				body = ParseFrontMatter(string(bodyData)).Body
				bodyExists = true
			}
		}

		// full_text 开启时正文也参与匹配
		if fullText && !matched && bodyExists {
			matched = strings.Contains(strings.ToLower(body), keyword)
		}

		if !matched {
			continue
		}

		// 格式化结果
		formatted := fmt.Sprintf("### [%s] %s\n", entry.Type, entry.Name)
		if entry.Description != "" {
			formatted += fmt.Sprintf("_%s_\n", entry.Description)
		}
		if body != "" {
			formatted += fmt.Sprintf("\n%s\n", truncateBody(body, s.memoryDir, safeName))
		}
		// 如果文件不存在，仍返回索引中的摘要（不报错）
		results = append(results, formatted)
	}

	toolCtx.Logger.Info("Search memory", zap.String("keyword", keyword), zap.Bool("full_text", fullText), zap.Int("matches", len(results)))

	if len(results) == 0 {
		return tools.ToolResult{
			Ok:      true,
			Content: fmt.Sprintf("No memories found matching keyword %q.", keyword),
			IsError: false,
		}
	}

	header := fmt.Sprintf("Found %d memory(ies) matching keyword %q:\n\n", len(results), keyword)
	return tools.ToolResult{
		Ok:      true,
		Content: header + strings.Join(results, "\n"),
		IsError: false,
	}
}

// truncateBody 将正文按 rune 截断到 MaxBodyChars，超出时追加提示，
// 指引模型改用 read_file 读取完整 memory 文件。
func truncateBody(body, memoryDir, safeName string) string {
	runes := []rune(body)
	if len(runes) <= MaxBodyChars {
		return body
	}
	note := fmt.Sprintf("...[truncated at %dKB; use read_file on %s/%s.md for the full memory]",
		MaxBodyChars/1024, memoryDir, safeName)
	return string(runes[:MaxBodyChars]) + note
}
