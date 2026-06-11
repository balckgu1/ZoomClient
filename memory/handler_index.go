package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	return false
}

// parseIndex 解析 MEMORY.md 索引内容，返回所有有效条目
func parseIndex(indexData string) []MemoryFrontMatter {
	var entries []MemoryFrontMatter

	lines := strings.Split(indexData, "\n")
	for _, line := range lines {
		line := strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-") {
			continue
		}
		// 去掉 "- "
		line = strings.TrimPrefix(line, "- ")

		// 提取 [type]
		typIdx := strings.LastIndex(line, "[")
		typEnd := strings.LastIndex(line, "]")
		if typIdx < 0 || typEnd < 0 || typEnd <= typIdx {
			continue
		}
		typ := strings.TrimSpace(line[typIdx+1 : typEnd])
		remaining := strings.TrimSpace(line[:typIdx])

		// 找到第一个 ":" 的位置
		colonIdx := strings.Index(remaining, ":")
		if colonIdx < 0 {
			continue
		}
		name := strings.TrimSpace(remaining[:colonIdx])
		description := strings.TrimSpace(remaining[colonIdx+1:])

		if name == "" || typ == "" {
			continue
		}
		entries = append(entries, MemoryFrontMatter{
			Name:        name,
			Description: description,
			Type:        typ,
		})
	}

	return entries
}

// sortByPriority 按 MemoryPriority 对entry排序
func sortByPriority(entries []MemoryFrontMatter) {
	sort.SliceStable(entries, func(i, j int) bool {
		pi := getPriority(entries[i].Type)
		pj := getPriority(entries[j].Type)
		return pi < pj
	})
}

// getPriority 返回type对应优先级
func getPriority(typ string) int {
	priority, ok := MemoryPriority[typ]
	if ok {
		return priority
	}
	return 99
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

// findEntryLineIndex 在原始索引行中找到对应 name 的行号
func findEntryLineIndex(lines []string, targetName string) int {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "-") {
			continue
		}
		entry := parseLine(trimmed)
		if entry.Name == targetName {
			return i
		}
	}
	return -1
}

// parseLine 解析单行索引，返回 MemoryFrontMatter
func parseLine(trimmed string) MemoryFrontMatter {
	if !strings.HasPrefix(trimmed, "-") {
		return MemoryFrontMatter{}
	}
	line := strings.TrimPrefix(trimmed, "- ")
	typIdx := strings.LastIndex(line, "[")
	typEnd := strings.LastIndex(line, "]")
	if typIdx < 0 || typEnd < 0 || typEnd <= typIdx {
		return MemoryFrontMatter{}
	}
	typ := strings.TrimSpace(line[typIdx+1 : typEnd])
	remaining := strings.TrimSpace(line[:typIdx])
	colonIdx := strings.Index(remaining, ":")
	if colonIdx < 0 {
		return MemoryFrontMatter{}
	}
	name := strings.TrimSpace(remaining[:colonIdx])
	description := strings.TrimSpace(remaining[colonIdx+1:])
	return MemoryFrontMatter{Name: name, Description: description, Type: typ}
}

// upsertIndex 增量更新索引：添加或更新单条条目，使用 parseLine 精确匹配行，避免含冒号 name 的前缀误判。
func upsertIndex(memoryDir, name, description, typ string) error {
	indexPath := filepath.Join(memoryDir, "MEMORY.md")
	newLine := fmt.Sprintf("- %s: %s [%s]", name, description, typ)

	data, err := os.ReadFile(indexPath)
	if err != nil {
		// 索引不存在，创建新索引
		content := "# Memory Index\n\n" + newLine + "\n"
		return os.WriteFile(indexPath, []byte(content), 0644)
	}

	lines := strings.Split(string(data), "\n")
	lineIdx := findEntryLineIndex(lines, name)
	if lineIdx >= 0 {
		lines[lineIdx] = newLine
	} else {
		// 在末尾添加新条目（去除末尾空行后追加）
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, newLine)
	}

	return os.WriteFile(indexPath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

// removeFromIndex 从索引中删除指定 name 的条目，使用 parseLine 精确匹配行，避免含冒号 name 的前缀误判。
func removeFromIndex(memoryDir, name string) error {
	indexPath := filepath.Join(memoryDir, "MEMORY.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	lineIdx := findEntryLineIndex(lines, name)
	if lineIdx < 0 {
		return nil // 不存在，无需操作
	}

	var result []string
	for i, line := range lines {
		if i != lineIdx {
			result = append(result, line)
		}
	}

	return os.WriteFile(indexPath, []byte(strings.Join(result, "\n")), 0644)
}
