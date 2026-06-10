package memory

import (
	"sort"
	"strings"
)

// MemoryIndexEntry 表示索引文件中解析出的一条 memory 条目。
type MemoryIndexEntry struct {
	Name        string
	Description string
	Type        string
}

// parseIndex 解析 MEMORY.md 索引内容，返回所有有效条目
func parseIndex(indexData string) []MemoryIndexEntry {
	var entries []MemoryIndexEntry

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
		entries = append(entries, MemoryIndexEntry{
			Name:        name,
			Description: description,
			Type:        typ,
		})
	}

	return entries
}

// sortByPriority 按 MemoryPriority 对entry排序
func sortByPriority(entries []MemoryIndexEntry) {
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
