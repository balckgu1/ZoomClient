package prompt

const core = `You are an AI agent powered by ZoomClient, working on the code in the current working directory.
- Ground every statement in what a tool actually returned in this session. Never describe a file, command, test result, or API as inspected unless you observed it.`

// buildCore 生成核心身份和行为说明段, 包含 todo 工具使用指引。
func (b *SystemPromptBuilder) buildCore() string {
	return core
}
