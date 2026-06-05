package skills

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// parseFrontmatter 解析 SKILL.md 文本
// 支持格式：
//
//		---
//		name: code-review
//		description: Checklist for reviewing code changes
//	    version: "v1.0"
//		author: your-name
//		compatibility: Python 3.10+
//		---
//		# content
//		...
//
// 若不存在 frontmatter，返回零值 manifest 和原文 body
func parseFrontmatter(raw string) (SkillManifest, string, error) {
	var manifest SkillManifest

	// 统一换行符
	raw = strings.ReplaceAll(raw, "\r\n", "\n")

	// 严格校验 frontmatter 边界
	if !strings.HasPrefix(raw, "---\n") {
		return manifest, raw, nil
	}

	rest := strings.TrimPrefix(raw, "---\n")
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		// 找不到闭合分隔符，视为无有效 frontmatter
		return manifest, raw, nil
	}

	yamlContent := rest[:endIdx]
	body := strings.TrimLeft(rest[endIdx+len("\n---"):], "\n")

	// 使用标准库解析
	if err := yaml.Unmarshal([]byte(yamlContent), &manifest); err != nil {
		return manifest, "", fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	return manifest, body, nil
}
