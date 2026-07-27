// ui/tui_styles.go
//
// 基于 lipgloss 的全局样式常量，供 TUI 各组件使用。
// 配色: Catppuccin Mocha — 温暖、精致的终端美学
package ui

import "github.com/charmbracelet/lipgloss"

// ═══════════════════════════════════════════
// 颜色常量 — Catppuccin Mocha Palette
// ═══════════════════════════════════════════
var (
	cText    = lipgloss.Color("#CDD6F4") // 主文字：柔白
	cSubtext = lipgloss.Color("#A6ADC8") // 次文字：暖灰
	cOverlay = lipgloss.Color("#6C7086") // 覆盖层：深灰
	cSurface = lipgloss.Color("#313244") // 表面：暗紫灰
	cBase    = lipgloss.Color("#1E1E2E") // 基础：深紫黑

	cBlue     = lipgloss.Color("#89B4FA") // 天蓝：助手
	cSky      = lipgloss.Color("#89DCEB") // 晴空：高亮
	cTeal     = lipgloss.Color("#94E2D5") // 青绿：装饰
	cGreen    = lipgloss.Color("#A6E3A1") // 薄荷绿：成功
	cYellow   = lipgloss.Color("#F9E2AF") // 暖黄：工具
	cPeach    = lipgloss.Color("#FAB387") // 蜜桃：用户
	cPink     = lipgloss.Color("#F5C2E7") // 柔粉：强调
	cRed      = lipgloss.Color("#F38BA8") // 珊瑚红：错误
	cMauve    = lipgloss.Color("#CBA6F7") // 淡紫：reasoning
	cLavender = lipgloss.Color("#B4BEFE") // 薰衣草：代码

	cBorder = lipgloss.Color("#45475A") // 边框色
)

// ═══════════════════════════════════════════
// 基础组件样式
// ═══════════════════════════════════════════
var (
	// 助手回复：天蓝色左边框 + 标签
	StyleAssistant = lipgloss.NewStyle().
			Foreground(cBlue).
			BorderStyle(lipgloss.ThickBorder()).
			BorderLeft(true).
			BorderForeground(cBlue).
			PaddingLeft(1)

	// 助手标签
	StyleAssistantLabel = lipgloss.NewStyle().
				Foreground(cBlue).
				Bold(true).
				PaddingLeft(1)

	// 用户输入：蜜桃色
	StyleUser = lipgloss.NewStyle().
			Foreground(cPeach).
			Bold(true)

	// 用户标签
	StyleUserLabel = lipgloss.NewStyle().
			Foreground(cPeach).
			Bold(true).
			PaddingLeft(1)

	// reasoning：薰衣草斜体
	StyleReasoning = lipgloss.NewStyle().
			Foreground(cMauve).
			Italic(true).
			PaddingLeft(2)

	// reasoning 标签
	StyleReasoningLabel = lipgloss.NewStyle().
				Foreground(cMauve).
				Bold(true)

	// 工具调用：暖黄色
	StyleToolCall = lipgloss.NewStyle().
			Foreground(cYellow).
			Bold(true)

	StyleToolArgs = lipgloss.NewStyle().
			Foreground(cSubtext)

	// 工具结果摘要
	StyleToolResult = lipgloss.NewStyle().
			Foreground(cSubtext).
			PaddingLeft(2)

	// 工具成功状态
	StyleToolOK = lipgloss.NewStyle().
			Foreground(cGreen).
			Bold(true)

	// 错误面板
	StyleError = lipgloss.NewStyle().
			Foreground(cRed).
			Bold(true).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(cRed).
			Padding(0, 1)

	// hook 阻止
	StyleHookBlocked = lipgloss.NewStyle().
				Foreground(cRed).
				Bold(true)

	// Todo 面板
	StyleTodoPanel = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(cGreen).
			Padding(0, 1)

	StyleTodoTitle = lipgloss.NewStyle().
			Foreground(cGreen).
			Bold(true)

	// 压缩 / subagent / 分隔线
	StyleCompact   = lipgloss.NewStyle().Foreground(cLavender)
	StyleSubAgent  = lipgloss.NewStyle().Foreground(cSky).Bold(true)
	StyleSeparator = lipgloss.NewStyle().Foreground(cOverlay)

	// 状态栏
	StyleStatusBar = lipgloss.NewStyle().
			Background(cSurface).
			Foreground(cSubtext).
			Padding(0, 1)

	// 状态栏各节颜色
	StyleStatusModel = lipgloss.NewStyle().
				Foreground(cBlue).
				Bold(true)
	StyleStatusLabel = lipgloss.NewStyle().
				Foreground(cOverlay)
	StyleStatusValue = lipgloss.NewStyle().
				Foreground(cSubtext)

	// 输入区边框
	StyleInputBorder = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(cBorder)

	// 输入区聚焦边框
	StyleInputBorderFocused = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(cBlue)

	// 面板边框（对话流外框）
	StylePanelBorder = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(cBorder)

	// ═══════════════════════════════════════════
	// 可折叠卡片
	// ═══════════════════════════════════════════

	// 工具卡片 — 折叠态（暖黄边框）
	StyleCardToolCollapsed = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(cYellow).
				Padding(0, 1)

	// 工具卡片 — 展开态（暖黄边框加粗）
	StyleCardToolExpanded = lipgloss.NewStyle().
				BorderStyle(lipgloss.ThickBorder()).
				BorderForeground(cYellow).
				Padding(0, 1)

	// 工具卡片 — 错误态（红色边框）
	StyleCardToolError = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(cRed).
				Padding(0, 1)

	// reasoning 卡片 — 折叠态（淡紫边框）
	StyleCardReasonCollapsed = lipgloss.NewStyle().
					BorderStyle(lipgloss.RoundedBorder()).
					BorderForeground(cMauve).
					Padding(0, 1)

	// reasoning 卡片 — 展开态（淡紫边框加粗）
	StyleCardReasonExpanded = lipgloss.NewStyle().
				BorderStyle(lipgloss.ThickBorder()).
				BorderForeground(cMauve).
				Padding(0, 1)

	// 卡片焦点态（天蓝高亮）
	StyleCardFocused = lipgloss.NewStyle().
				BorderStyle(lipgloss.DoubleBorder()).
				BorderForeground(cSky).
				Padding(0, 1)

	// 卡片分隔线
	StyleCardDivider = lipgloss.NewStyle().
				Foreground(cOverlay)

	// ═══════════════════════════════════════════
	// Markdown 渲染样式
	// ═══════════════════════════════════════════

	StyleH1 = lipgloss.NewStyle().
		Foreground(cText).
		Bold(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(cBlue).
		PaddingBottom(0)

	StyleH2 = lipgloss.NewStyle().
		Foreground(cBlue).
		Bold(true)

	StyleH3 = lipgloss.NewStyle().
		Foreground(cSky).
		Bold(true)

	StyleCodeBlock = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(cBorder).
			Background(cBase).
			Padding(0, 1)

	StyleInlineCode = lipgloss.NewStyle().
			Background(cSurface).
			Foreground(cLavender)

	StyleBlockquote = lipgloss.NewStyle().
			Foreground(cSubtext).
			Italic(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderLeft(true).
			BorderForeground(cTeal).
			PaddingLeft(1)

	StyleListBullet = lipgloss.NewStyle().
			Foreground(cTeal)

	StyleHr = lipgloss.NewStyle().
		Foreground(cOverlay)

	// 回合分隔线
	StyleTurnSep = lipgloss.NewStyle().
			Foreground(cOverlay)

	// ═══════════════════════════════════════════
	// 滚动条
	// ═══════════════════════════════════════════
	StyleScrollbar = lipgloss.NewStyle().
			Foreground(cBorder)

	StyleScrollThumb = lipgloss.NewStyle().
				Foreground(cOverlay).
				Bold(true)
)
