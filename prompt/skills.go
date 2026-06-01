package prompt

import "fmt"

// buildSkills 生成 skills 目录段。
func (b *SystemPromptBuilder) buildSkills() string {
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
