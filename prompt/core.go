package prompt

import (
	"fmt"
)

// buildCore 生成核心身份和行为说明段, 包含 todo 工具使用指引
func (b *SystemPromptBuilder) buildCore() string {
	return fmt.Sprintf(
		"You are a helpful assistant." +
			"Use the todo tool to plan multi-step work. " +
			"Keep exactly one step in_progress when a task has multiple steps. " +
			"Refresh the plan as work advances. Prefer tools over prose.")
}
