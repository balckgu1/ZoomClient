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
	return "Search for memories by keyword. Returns matching memory entries from previous sessions."
}

func (s *SearchMemoryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"keyword": map[string]any{
				"type":        "string",
				"description": "The keyword to search for in memory name and description.",
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

	// check memoryDir
	if s.memoryDir == "" {
		return tools.ToolResult{Ok: false, Content: "Error: MemoryDir is not configured", IsError: true}
	}

	// read MEMORY.md
	indexPath := filepath.Join(s.memoryDir, "MEMORY.md")
	if !fileExists(indexPath) {
		return tools.ToolResult{Ok: false, Content: "Error: MEMORY.md does not exist", IsError: true}
	}
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return tools.ToolResult{Ok: false, Content: "Error: failed to read MEMORY.md", IsError: true}
	}

	entries := parseIndex(string(indexData))

	var results []string
	for _, entry := range entries {
		// 搜索 name、description 是否匹配，都不匹配则跳过
		nameLower := strings.ToLower(entry.Name)
		descLower := strings.ToLower(entry.Description)

		if !strings.Contains(nameLower, keyword) && !strings.Contains(descLower, keyword) {
			continue
		}

		// 格式化结果
		formatted := fmt.Sprintf("### [%s] %s\n", entry.Type, entry.Name)
		if entry.Description != "" {
			formatted += fmt.Sprintf("_%s_\n", entry.Description)
		}

		// 按需读取 body（使用 sanitizeFilename 保证路径正确）
		safeName := sanitizeFilename(entry.Name)
		memoryPath := filepath.Join(s.memoryDir, safeName+".md")
		if bodyData, err := os.ReadFile(memoryPath); err == nil {
			// 解析 frontmatter 只取 body 内容
			doc := ParseFrontMatter(string(bodyData))
			body := doc.Body
			runes := []rune(body)
			if len(runes) > MaxBodyPreviewChars {
				body = string(runes[:MaxBodyPreviewChars]) + "..."
			}
			if body != "" {
				formatted += fmt.Sprintf("\n%s\n", body)
			}
		}
		// 如果文件不存在，仍返回索引中的摘要（不报错）
		results = append(results, formatted)
	}

	toolCtx.Logger.Info("Search memory", zap.String("keyword", keyword), zap.Int("matches", len(results)))

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
