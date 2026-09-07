package prompt

import "fmt"

// SkillsSection 生成 skills 目录段文本（导出供上下文占用统计复用）。
// 无 skills 注册表或无可用 skill 时返回空字符串。
func (b *SystemPromptBuilder) SkillsSection() string {
	if b.skillRegistry == nil {
		return ""
	}
	desc := b.skillRegistry.DescribeAvailable()
	if desc == "" {
		return ""
	}
	return fmt.Sprintf(
		"Skills available (call the load_skill tool to load the full body on demand):\n%s",
		desc,
	)
}

// buildSkills 生成 skills 目录段，作为 Build 拼接的其中一个段。
func (b *SystemPromptBuilder) buildSkills() string {
	return b.SkillsSection()
}

// SkillCount 返回当前已加载的 skill 数量（无 skills 注册表时为 0）。
func (b *SystemPromptBuilder) SkillCount() int {
	if b.skillRegistry == nil {
		return 0
	}
	return b.skillRegistry.Count()
}
