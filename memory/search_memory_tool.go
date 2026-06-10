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
	return "Search for memories by keyword or type. Returns matching memory entries from previous sessions."
}

func (s *SearchMemoryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"keyword": map[string]any{
				"type":        "string",
				"description": "The keyword to search for in memory name, description, and body content.",
			},
			"type": map[string]any{
				"type":        "string",
				"description": "Optional: filter by memory category. Must be one of: 'user', 'feedback', 'project', or 'reference'.",
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

	// check type
	var typeFilter string
	if typeRaw, exist := args["type"]; exist {
		if t, ok := typeRaw.(string); ok && strings.TrimSpace(t) != "" {
			typeFilter = strings.TrimSpace(t)
			typeValid := map[string]bool{"user": true, "feedback": true, "project": true, "reference": true}
			if !typeValid[typeFilter] {
				return tools.ToolResult{
					Ok:      false,
					Content: fmt.Sprintf("Error: type must be one of: user, feedback, project, reference, got: %q", typeFilter),
					IsError: true,
				}
			}
		}
	}

	// check memoryDir
	if s.memoryDir == "" {
		return tools.ToolResult{Ok: false, Content: "Error: MemoryDir is not configured", IsError: true}
	}

	// scan memorydir
	entries, err := os.ReadDir(s.memoryDir)
	if err != nil {
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: failed to read memory directory: %v", err),
			IsError: true,
		}
	}
	var results []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") || name == "MEMORY.md" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.memoryDir, name))
		if err != nil {
			continue
		}

		doc := ParseFrontMatter(string(data))
		if doc.FrontMatter.Name == "" {
			continue
		}

		// check type
		if typeFilter != "" && doc.FrontMatter.Type != typeFilter {
			continue
		}

		// 搜索 name、description、body是否匹配，都不匹配则跳过
		nameLower := strings.ToLower(doc.FrontMatter.Name)
		descLower := strings.ToLower(doc.FrontMatter.Description)
		bodyLower := strings.ToLower(doc.Body)

		if !strings.Contains(nameLower, keyword) && !strings.Contains(descLower, keyword) && !strings.Contains(bodyLower, keyword) {
			continue
		}

		// 格式化结果
		formatted := fmt.Sprintf("### [%s] %s\n", doc.FrontMatter.Type, doc.FrontMatter.Name)
		if doc.FrontMatter.Description != "" {
			formatted += fmt.Sprintf("_%s_\n", doc.FrontMatter.Description)
		}
		if doc.Body != "" {
			// 截取 body 前 200 个字符
			body := doc.Body
			runes := []rune(body)
			if len(runes) > 200 {
				body = string(runes[:200]) + "..."
			}
			formatted += fmt.Sprintf("\n%s\n", body)
		}
		results = append(results, formatted)
	}

	toolCtx.Logger.Info("Search memory",
		zap.String("keyword", keyword),
		zap.String("typeFilter", typeFilter),
		zap.Int("matches", len(results)),
	)

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
