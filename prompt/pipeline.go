package prompt

import "zoomClient/fsm"

// APIPayload 最终组装的模型输入载荷。
type APIPayload struct {
	SystemPrompt string
	Messages     []fsm.Message // normalize + reminders + attachments
}

// MessagePipeline 模型输入的完整组装管道 ==> prompt blocks / normalized messages / reminders / attachments。
type MessagePipeline struct {
	builder     *SystemPromptBuilder
	reminders   []Reminder
	attachments []Attachment
}

// NewPipeline 构造管道实例。
func NewPipeline(builder *SystemPromptBuilder) *MessagePipeline {
	return &MessagePipeline{
		builder:     builder,
		reminders:   []Reminder{},
		attachments: []Attachment{},
	}
}

// AssemblePayload 执行完整组装流水线，返回可直接传给 client.Chat 的载荷。
func (p *MessagePipeline) AssemblePayload(rawMessages []fsm.Message) APIPayload {
	// 1. 构建 system prompt（走已有的 Builder）
	systemPrompt := p.builder.Build()

	// 2. Normalize messages
	messages := p.normalizeMessages(rawMessages)

	// 3. Inject reminders（以 system role 追加到末尾）
	messages = p.injectReminders(messages)

	// 4. Inject attachments（预留）
	messages = p.injectAttachments(messages)

	return APIPayload{SystemPrompt: systemPrompt, Messages: messages}
}

// injectReminders 将所有 reminder 以 role="system" 追加到消息列表末尾。
func (p *MessagePipeline) injectReminders(msgs []fsm.Message) []fsm.Message {
	for _, r := range p.reminders {
		msgs = append(msgs, fsm.Message{
			Role:    "system",
			Content: r.Content,
		})
	}
	return msgs
}

// injectAttachments 注入附件（当前为占位实现）。
func (p *MessagePipeline) injectAttachments(msgs []fsm.Message) []fsm.Message {
	// 预留：后续可将 attachment 作为 user 消息追加
	return msgs
}

// UpdateModelName 热更新 Pipeline 使用的模型名
func (p *MessagePipeline) UpdateModelName(model string) {
	if p.builder != nil {
		p.builder.UpdateModel(model)
	}
}

// SkillsSection 返回 system prompt 中 skills 目录段文本（无 skills 时为空字符串）。
// 上下文占用统计据此把 skills 段从 system prompt 主体中拆分出来单独展示。
func (p *MessagePipeline) SkillsSection() string {
	if p.builder == nil {
		return ""
	}
	return p.builder.SkillsSection()
}

// BuildSystemPrompt 返回组装完成的完整 system prompt（含 skills 目录段），
// 供上下文占用统计等只需要 system prompt 尺寸的场景使用。
func (p *MessagePipeline) BuildSystemPrompt() string {
	if p.builder == nil {
		return ""
	}
	return p.builder.Build()
}

// SkillCount 返回当前已加载的 skill 数量。
func (p *MessagePipeline) SkillCount() int {
	if p.builder == nil {
		return 0
	}
	return p.builder.SkillCount()
}

// UpdateWorkDir 热更新 Pipeline 使用的工作目录
func (p *MessagePipeline) UpdateWorkDir(workDir string) {
	if p.builder != nil {
		p.builder.UpdateWorkDir(workDir)
	}
}
