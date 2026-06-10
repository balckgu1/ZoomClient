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

// LoadMemorySection 扫描 memoryDir 中的所有 memory 文件，
// 返回格式化好的 memory section 字符串，可直接追加到 system prompt。
func LoadMemorySection(memoryDir string) string {
	if memoryDir == "" {
		return ""
	}
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		return ""
	}

	var docs []MemoryDocument
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
			docs = append(docs, doc)
		}
	}

	if len(docs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Memories from previous sessions\n\n")
	for _, doc := range docs {
		sb.WriteString(fmt.Sprintf("### [%s] %s\n", doc.FrontMatter.Type, doc.FrontMatter.Name))
		if doc.FrontMatter.Description != "" {
			sb.WriteString(fmt.Sprintf("_%s_\n\n", doc.FrontMatter.Description))
		}
		if doc.Body != "" {
			sb.WriteString(doc.Body)
			sb.WriteString("\n\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}
