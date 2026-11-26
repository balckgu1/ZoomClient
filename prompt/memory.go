package prompt

import (
	"strings"
	"zoomClient/memory"
)

// memorySaveRules 是 memory 保存规则文本，告诉模型何时应该保存 memory、何时不应该。
const memorySaveRules = `Save Memories:
- User Preference: Explicit likes/dislikes (e.g., "I like tabs", "use pytest"). -> type: user
- Feedback: User corrections or constraints (e.g., "don't do X", "that was wrong"). -> type: feedback
- Project Context: Non-obvious facts not inferable from code (e.g., compliance rules, legacy constraints). -> type: project
- References: Locations of external resources (e.g., ticket boards, dashboards, docs URLs). -> type: reference

**Do NOT Save:**
- Code-derivable info (signatures, file structure).
- Temporary state (current branch, open PRs, TODOs).
- Secrets (API keys, passwords).`

// memoryUsageNote 是 memory 的使用约束：memory 只是写入瞬间的快照，
// 引用具体文件/函数/命令前必须回到当前代码验证，冲突时以现状为准。
const memoryUsageNote = `Using memories:
- A memory is a snapshot from the moment it was written. Before acting on one that names a file, function, command, or flag, verify it still exists in the current code.
- If a memory conflicts with what you observe now, trust the current state and update the memory instead of following the stale entry.`

// buildMemory 生成 memory 段。
// 包含三部分：
//  1. 历史 memory
//  2. memory 使用约束（快照可能过期）
//  3. memory 保存规则
func (b *SystemPromptBuilder) buildMemory() string {
	var parts []string

	// 加载 memory
	if b.memoryDir != "" {
		memSection := memory.LoadMemorySection(b.memoryDir)
		if memSection != "" {
			parts = append(parts, memSection)
		}
	}

	// 追加使用约束：memory 是快照，落地前先校验
	parts = append(parts, memoryUsageNote)

	// 追加 memory 保存规则
	parts = append(parts, memorySaveRules)

	return strings.Join(parts, "\n\n")
}
