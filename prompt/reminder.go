package prompt

// Reminder 系统提醒，只活一轮或几轮。
// 与长期 prompt block 的区别：reminder 是临时注入，可一轮后自动消失。
type Reminder struct {
	Content string // 提醒文本
	Source  string // 来源标识：hook / todo / post_hook
	OneShot bool   // true = 用完即删
}

// AddReminder 添加一条临时提醒到管道。
func (p *MessagePipeline) AddReminder(r Reminder) {
	p.reminders = append(p.reminders, r)
}

// ClearOneShotReminders 清除所有 OneShot=true 的 reminder。每轮调 LLM 后调用。
func (p *MessagePipeline) ClearOneShotReminders() {
	kept := p.reminders[:0]
	for _, r := range p.reminders {
		if !r.OneShot {
			kept = append(kept, r)
		}
	}
	p.reminders = kept
}
