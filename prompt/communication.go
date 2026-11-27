package prompt

// communicationSection 交互风格段。
const communicationSection = `Communication
- Before the first tool call of a turn, say in one sentence what you are about to do.
- At key moments - you found something important, changed direction, or got blocked - give a one-sentence update instead of continuing silently.
- Do not narrate internal deliberation or restate a step you just finished; keep visible text short and limited to what the user needs to follow along.`

// buildCommunication 生成交互风格段，作为 Build 拼接的其中一个段。
func (b *SystemPromptBuilder) buildCommunication() string {
	return communicationSection
}
