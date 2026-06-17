package prompt

import (
	"strings"
	"zoomClient/skills"
)

// SystemPromptBuilder Prompt Builder
type SystemPromptBuilder struct {
	skillRegistry *skills.SkillRegistry
	memoryDir     string
	model         string
	workDir       string
}

// NewSystemPromptBuilder 构造 Builder，通过构造函数注入所有外部依赖。
func NewSystemPromptBuilder(skillRegistry *skills.SkillRegistry,
	memoryDir string, model string, workDir string) *SystemPromptBuilder {
	return &SystemPromptBuilder{
		skillRegistry: skillRegistry,
		memoryDir:     memoryDir,
		model:         model,
		workDir:       workDir,
	}
}

// Build 按序调用 6 个 build 方法，收集非空段，用 "\n\n" 拼接返回最终的 system prompt。
func (b *SystemPromptBuilder) Build() string {
	parts := []string{
		b.buildCore(),
		b.buildSkills(),
		b.buildMemory(),
		b.buildClaudeMD(),
		b.buildDynamic(),
	}

	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, "\n\n")
}

// UpdateModel 热更新 Builder 使用的模型名
func (b *SystemPromptBuilder) UpdateModel(model string) {
	b.model = model
}
