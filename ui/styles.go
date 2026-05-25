// Package ui 集中负责 cc-learn CLI 前端的"用户可见"渲染。
//
// 设计原则：
//   - 日志（zap）→ ./logs/backend.log，承载完整诊断流；
//   - 前端（本包）→ stdout，只渲染对人有意义的事件；
//   - 双轨并存，互不耦合，便于未来切换到 bubbletea 全 TUI。
//
// 样式集中放在本文件，便于一处调整全局观感。
package ui

import "github.com/charmbracelet/lipgloss"

// 颜色常量：使用 lipgloss 的 AdaptiveColor 让暗/亮终端都好看；
// 这里为简化教学，直接用 ANSI 256-color 编号即可。
var (
	colorAssistant = lipgloss.Color("#7DD3FC") // 浅青：助手回复
	colorUser      = lipgloss.Color("#F472B6") // 粉色：用户输入
	colorReason    = lipgloss.Color("#94A3B8") // 灰色：reasoning / 弱信息
	colorTool      = lipgloss.Color("#FBBF24") // 黄色：工具调用
	colorOK        = lipgloss.Color("#34D399") // 绿色：成功 / completed
	colorError     = lipgloss.Color("#F87171") // 红色：错误
	colorCompact   = lipgloss.Color("#C084FC") // 洋红：压缩
	colorSubAgent  = lipgloss.Color("#60A5FA") // 蓝色：子智能体
	colorMuted     = lipgloss.Color("#64748B") // 暗灰：分隔线、附注
)

// 各类样式：编译期常量，便于其它文件直接引用。
var (
	// 助手回复：浅青色加粗，左边一道竖线增强阅读层次
	styleAssistant = lipgloss.NewStyle().
			Foreground(colorAssistant).
			Bold(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderLeft(true).
			BorderForeground(colorAssistant).
			PaddingLeft(1)

	// reasoning：灰色斜体，缩进一格
	styleReasoning = lipgloss.NewStyle().
			Foreground(colorReason).
			Italic(true).
			PaddingLeft(2)

	// 用户输入提示符
	styleUserPrompt = lipgloss.NewStyle().
			Foreground(colorUser).
			Bold(true)

	// 工具调用：黄色 ▶ 工具名加粗
	styleToolCall = lipgloss.NewStyle().
			Foreground(colorTool).
			Bold(true)

	styleToolArgs = lipgloss.NewStyle().
			Foreground(colorMuted)

	// 工具结果摘要
	styleToolResult = lipgloss.NewStyle().
			Foreground(colorMuted).
			PaddingLeft(2)

	// 错误面板（红色边框）
	styleError = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorError).
			Padding(0, 1)

	// hook 阻止
	styleHookBlocked = lipgloss.NewStyle().
				Foreground(colorError).
				Bold(true)

	// Todo 面板（圆角边框）
	styleTodoPanel = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorOK).
			Padding(0, 1)

	styleTodoTitle = lipgloss.NewStyle().
			Foreground(colorOK).
			Bold(true)

	// 压缩、subagent、分隔线
	styleCompact   = lipgloss.NewStyle().Foreground(colorCompact)
	styleSubAgent  = lipgloss.NewStyle().Foreground(colorSubAgent).Bold(true)
	styleSeparator = lipgloss.NewStyle().Foreground(colorMuted)

	// 会话开始横幅
	styleBanner = lipgloss.NewStyle().
			BorderStyle(lipgloss.DoubleBorder()).
			BorderForeground(colorAssistant).
			Padding(0, 2).
			Bold(true).
			Foreground(colorAssistant)
)
