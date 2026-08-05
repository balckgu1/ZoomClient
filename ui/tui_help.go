// ui/tui_help.go
//
// 快捷键帮助浮层组件：以分类表格展示所有可用快捷键。
package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpSection 帮助面板中的一个分类。
type helpSection struct {
	Title string
	Items []helpItem
}

// helpItem 单个快捷键条目。
type helpItem struct {
	Key  string
	Desc string
}

// HelpOverlay 管理快捷键帮助浮层的显示状态。
type HelpOverlay struct {
	visible  bool
	sections []helpSection
}

// NewHelpOverlay 创建一个新的帮助浮层。
func NewHelpOverlay() *HelpOverlay {
	return &HelpOverlay{
		sections: []helpSection{
			{
				Title: "发送",
				Items: []helpItem{
					{"Enter", "发送消息"},
					{"Shift+Enter", "换行"},
				},
			},
			{
				Title: "导航",
				Items: []helpItem{
					{"PgUp / Ctrl+Up", "向上滚动"},
					{"PgDn / Ctrl+Down", "向下滚动"},
					{"Home", "跳到顶部"},
					{"End", "跳到底部"},
				},
			},
			{
				Title: "卡片操作",
				Items: []helpItem{
					{"← →", "切换焦点卡片"},
					{"Enter", "展开/折叠卡片"},
				},
			},
			{
				Title: "输入历史",
				Items: []helpItem{
					{"↑", "上一条历史"},
					{"↓", "下一条历史"},
				},
			},
			{
				Title: "Slash 命令",
				Items: []helpItem{
					{"/", "输入 / 查看命令列表"},
					{"Tab", "补全选中命令"},
				},
			},
			{
				Title: "系统",
				Items: []helpItem{
					{"Ctrl+C", "停止/退出"},
					{"Ctrl+Q", "退出程序"},
					{"Ctrl+L", "清空对话流"},
					{"? / Ctrl+H", "显示/隐藏帮助"},
				},
			},
		},
	}
}

// Visible 返回浮层是否可见。
func (h *HelpOverlay) Visible() bool { return h.visible }

// Toggle 切换显示/隐藏。
func (h *HelpOverlay) Toggle() { h.visible = !h.visible }

// Hide 隐藏浮层。
func (h *HelpOverlay) Hide() { h.visible = false }

// View 渲染帮助浮层（简化版，无边框）。
func (h *HelpOverlay) View(width int) string {
	if !h.visible {
		return ""
	}

	// 浮层宽度自适应
	overlayW := width - 4
	if overlayW > 70 {
		overlayW = 70
	}
	if overlayW < 36 {
		overlayW = 36
	}

	// 标题样式
	titleStyle := lipgloss.NewStyle().
		Foreground(cBlue).
		Bold(true)

	// 分类标题样式
	sectionStyle := lipgloss.NewStyle().
		Foreground(cSky).
		Bold(true)

	// 按键样式
	keyStyle := lipgloss.NewStyle().
		Foreground(cYellow).
		Bold(true).
		Width(20)

	// 描述样式
	descStyle := lipgloss.NewStyle().
		Foreground(cSubtext)

	var sb strings.Builder

	// 标题行
	sb.WriteString(titleStyle.Render(" Keyboard Shortcuts"))
	sb.WriteString("\n")
	sb.WriteString(StyleCardDivider.Render(strings.Repeat("─", overlayW-2)))
	sb.WriteString("\n\n")

	for _, sec := range h.sections {
		sb.WriteString(sectionStyle.Render(" " + sec.Title))
		sb.WriteString("\n")
		for _, item := range sec.Items {
			sb.WriteString(keyStyle.Render("  " + item.Key))
			sb.WriteString(descStyle.Render(item.Desc))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// 底部提示
	sb.WriteString(StyleCardDivider.Render(strings.Repeat("─", overlayW-2)))
	sb.WriteString("\n")
	sb.WriteString(StyleSeparator.Render("  Esc close"))

	return sb.String()
}
