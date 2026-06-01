package prompt

import (
	"strings"
	"zoomClient/memory"
)

// memorySaveRules 是 memory 保存规则文本，告诉模型何时应该保存 memory、何时不应该。
const memorySaveRules = `**Save Memories:**
- **User Preference:** Explicit likes/dislikes (e.g., "I like tabs", "use pytest"). -> type: user
- **Feedback:** User corrections or constraints (e.g., "don't do X", "that was wrong"). -> type: feedback
- **Project Context:** Non-obvious facts not inferable from code (e.g., compliance rules, legacy constraints). -> type: project
- **References:** Locations of external resources (e.g., ticket boards, dashboards, docs URLs). -> type: reference

**Do NOT Save:**
- Code-derivable info (signatures, file structure).
- Temporary state (current branch, open PRs, TODOs).
- Secrets (API keys, passwords).`

// buildMemory 生成 memory 段。
// 包含两部分：
//  1. 历史 memory
//  2. memory 保存规则
func (b *SystemPromptBuilder) buildMemory() string {
	var parts []string

	// 1. 加载历史 memory
	if b.memoryDir != "" {
		if memSection := memory.LoadMemorySection(b.memoryDir); memSection != "" {
			parts = append(parts, memSection)
		}
	}

	// 2. 追加 memory 保存规则
	parts = append(parts, memorySaveRules)

	return strings.Join(parts, "\n\n")
}
