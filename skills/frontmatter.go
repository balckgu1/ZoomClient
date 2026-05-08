package skills

import (
	"bufio"
	"strings"
)

// parseFrontmatter 解析 SKILL.md 文本
// 支持的格式：
//
//	---
//	name: code-review
//	description: Checklist for reviewing code changes
//	---
//	# 正文
//	...
//
// 若不存在 frontmatter，返回空 map，body 返回原文
func parseFrontmatter(raw string) (map[string]string, string) {
	meta := make(map[string]string, 0)

	raw = strings.ReplaceAll(raw, "\r\n", "\n")

	if !strings.HasPrefix(raw, "---\n") {
		return nil, raw
	}

	// 去掉首个 --- 行后，寻找下一个 --- 作为 frontmatter 结束
	rest := strings.TrimPrefix(raw, "---\n")
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		// 找不到闭合分隔符则视为无 frontmatter，保持正文原样返回
		return nil, raw
	}

	header := rest[:endIdx]
	// 跳过闭合的 "\n---" 以及其后的换行
	afterHeader := rest[endIdx+len("\n---"):]
	afterHeader = strings.TrimPrefix(afterHeader, "\n")

	// 解析 key: value 形式，忽略空行与注释
	scanner := bufio.NewScanner(strings.NewReader(header))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		// 支持双引号包裹
		value = strings.Trim(value, `"'`)
		if key != "" {
			meta[key] = value
		}
	}

	return meta, afterHeader
}
