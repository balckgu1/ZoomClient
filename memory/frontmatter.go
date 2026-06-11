package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// yamlUnquote 解析 YAML 值中的引号包裹，如果值被双引号包裹，则去除引号并还原转义字符。
func yamlUnquote(val string) string {
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		inner := val[1 : len(val)-1]
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, `\n`, "\n")
		inner = strings.ReplaceAll(inner, `\\`, `\`)
		return inner
	}
	return val
}

// ParseFrontMatter 解析 memory markdown 文件中的 YAML frontmatter。
// 文件格式:
//
//	---
//	name: xxx
//	description: xxx
//	type: xxx
//	---
//	body content
func ParseFrontMatter(content string) MemoryDocument {
	lines := strings.Split(content, "\n")
	fm := MemoryFrontMatter{}
	inFM := false
	endLine := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if !inFM {
				inFM = true
				continue
			}
			// 遇到第二个 --- 表示 frontmatter 结束
			endLine = i + 1
			break
		}
		if inFM {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := yamlUnquote(strings.TrimSpace(parts[1]))
				switch key {
				case "name":
					fm.Name = val
				case "description":
					fm.Description = val
				case "type":
					fm.Type = val
				}
			}
		}
	}

	body := ""
	if endLine > 0 && endLine < len(lines) {
		body = strings.TrimSpace(strings.Join(lines[endLine:], "\n"))
	}

	return MemoryDocument{FrontMatter: fm, Body: body}
}

// LoadMemorySection 索引驱动，仅加载 memory 摘要到 system prompt
func LoadMemorySection(memoryDir string) string {
	if memoryDir == "" {
		return ""
	}

	// 读索引文件（单次轻量 IO）
	indexPath := filepath.Join(memoryDir, "MEMORY.md")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return ""
	}

	// 解析索引，获取条目列表
	entries := parseIndex(string(indexData))
	if len(entries) == 0 {
		return ""
	}

	// 按优先级排序
	sortByPriority(entries)

	// 输出摘要列表 name + description + type（限制条目数）
	var sb strings.Builder
	sb.WriteString("## Memories from previous sessions\n")
	sb.WriteString("Use `search_memory` tool to retrieve full content when needed.\n\n")

	displayCount := len(entries)
	truncated := false
	if displayCount > MaxMemoryEntries {
		displayCount = MaxMemoryEntries
		truncated = true
	}

	for _, entry := range entries[:displayCount] {
		sb.WriteString("- **[")
		sb.WriteString(entry.Type)
		sb.WriteString("]** ")
		sb.WriteString(entry.Name)
		if entry.Description != "" {
			sb.WriteString(": ")
			sb.WriteString(entry.Description)
		}
		sb.WriteString("\n")
	}

	if truncated {
		remaining := len(entries) - displayCount
		sb.WriteString(fmt.Sprintf("\n_...and %d more entries. Use `search_memory` to find specific ones._\n", remaining))
	}

	return strings.TrimRight(sb.String(), "\n")
}
