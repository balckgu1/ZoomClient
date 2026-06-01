package prompt

// Attachment 大块可选补充信息（预留）。
// 后续可用于文件附件、上下文窗口等场景。
type Attachment struct {
	ID      string
	Content string
	Source  string
}
