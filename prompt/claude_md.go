package prompt

// buildClaudeMD 生成 CLAUDE.md 指令链段
// 实现分层指令链，按以下优先级叠加：
//  1. 用户全局级（~/.claude/CLAUDE.md）
//  2. 项目根目录级（./CLAUDE.md）
//  3. 当前子目录级（./subdir/CLAUDE.md）
func (b *SystemPromptBuilder) buildClaudeMD() string {
	return ""
}
