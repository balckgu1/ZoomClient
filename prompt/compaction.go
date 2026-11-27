package prompt

// CompactionMarker 是 compact 包用摘要替换历史时写入摘要消息的开头标记。
const CompactionMarker = "This conversation was compacted for continuity."

// compactionSection 上下文连续性段。告诉模型如何识别被压缩的历史，以及以摘要为准、缺细节重新读文件。
const compactionSection = `Context
- Long conversations get compacted: a user message starting with "` + CompactionMarker + `" replaces the earlier turns and carries a summary of them.
- Treat that summary as the authoritative record of what already happened. Do not reconstruct details it omits; re-read the files or re-run the commands you need.`

// buildCompaction 生成上下文连续性段，作为 Build 拼接的其中一个段。
func (b *SystemPromptBuilder) buildCompaction() string {
	return compactionSection
}
