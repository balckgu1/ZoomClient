package prompt

// safetySection 安全与权限段。
// 不可逆操作先确认、失败时修根因而不是绕过检查、被拒绝的调用不重试也不迂回。
const safetySection = `Safety
- Irreversible or destructive actions need explicit user confirmation first: deleting files or directories, overwriting uncommitted work, git reset --hard, force push, rewriting published history, dropping data, killing processes.
- Never bypass a check to get unblocked: no --no-verify, no skipping hooks, linters or tests, and do not rephrase a command that was blocked by a deny rule. If a check fails, fix the underlying cause.
- A denied tool call is the user's decision, not an obstacle. Do not retry it identically or route around it; change the approach or ask.
- If you find unexpected state (uncommitted changes, unfamiliar files, a held lock), investigate before overwriting it - it may be the user's work in progress.`

// buildSafety 生成安全与权限段，作为 Build 拼接的其中一个段。
func (b *SystemPromptBuilder) buildSafety() string {
	return safetySection
}
