// ui/tui_slash.go
//
// Slash 命令自动补全浮层组件。
package ui

import (
	"fmt"
	"strings"
)

// SlashCommand 表示一个斜杠命令。
type SlashCommand struct {
	Name        string
	Description string
}

// SlashOverlay 管理斜杠命令的下拉提示浮层。
type SlashOverlay struct {
	visible  bool
	commands []SlashCommand
	filtered []SlashCommand
	selected int
}

// NewSlashOverlay 创建一个新的斜杠命令浮层。
func NewSlashOverlay() *SlashOverlay {
	return &SlashOverlay{
		commands: []SlashCommand{
			{Name: "/exit", Description: "退出程序"},
			{Name: "/clear", Description: "清空对话历史"},
			{Name: "/compact", Description: "手动压缩对话历史"},
			{Name: "/setmode", Description: "添加/更新模型预设"},
			{Name: "/selectmode", Description: "切换模型"},
			{Name: "/models", Description: "列出所有模型预设"},
			{Name: "/workspace", Description: "显示/切换工作目录"},
			{Name: "/session", Description: "管理会话"},
			{Name: "/help", Description: "显示帮助"},
		},
	}
}

// Visible 返回浮层是否可见。
func (o *SlashOverlay) Visible() bool { return o.visible }

// Show 显示浮层。
func (o *SlashOverlay) Show() { o.visible = true }

// Hide 隐藏浮层并重置状态。
func (o *SlashOverlay) Hide() {
	o.visible = false
	o.filtered = nil
	o.selected = 0
}

// UpdateFilter 根据输入过滤命令列表。
func (o *SlashOverlay) UpdateFilter(input string) {
	o.filtered = nil
	prefix := input
	for _, cmd := range o.commands {
		if len(cmd.Name) >= len(prefix) && cmd.Name[:len(prefix)] == prefix {
			o.filtered = append(o.filtered, cmd)
		}
	}
	if o.selected >= len(o.filtered) {
		o.selected = 0
	}
}

// Select 移动选中项，delta 为正向下，负向上。
func (o *SlashOverlay) Select(delta int) {
	if len(o.filtered) == 0 {
		return
	}
	o.selected += delta
	if o.selected < 0 {
		o.selected = len(o.filtered) - 1
	}
	if o.selected >= len(o.filtered) {
		o.selected = 0
	}
}

// GetSelected 返回当前选中命令的名称。
func (o *SlashOverlay) GetSelected() string {
	if o.selected < len(o.filtered) {
		return o.filtered[o.selected].Name
	}
	return ""
}

// View 渲染斜杠命令浮层。
func (o *SlashOverlay) View(width int) string {
	if !o.visible || len(o.filtered) == 0 {
		return ""
	}

	maxW := width - 4
	if maxW > 60 {
		maxW = 60
	}
	if maxW < 20 {
		maxW = 20
	}

	// 标题行
	title := " Commands"
	if len(o.filtered) < len(o.commands) {
		title = fmt.Sprintf(" Commands (%d/%d)", len(o.filtered), len(o.commands))
	}

	var sb strings.Builder
	sb.WriteString(StyleToolCall.Render("▸" + title))
	sb.WriteString("\n")
	sb.WriteString(StyleSeparator.Render(strings.Repeat("─", maxW)))
	sb.WriteString("\n")

	for i, cmd := range o.filtered {
		marker := "  "
		style := StyleSeparator
		if i == o.selected {
			marker = "▸ "
			style = StyleToolCall
		}
		line := fmt.Sprintf("%-16s %s", marker+cmd.Name, cmd.Description)
		sb.WriteString(style.Render(line))
		sb.WriteString("\n")
	}

	// 底部提示
	sb.WriteString(StyleSeparator.Render(strings.Repeat("─", maxW)))
	sb.WriteString("\n")
	sb.WriteString(StyleSeparator.Render("  ↑↓ select  ·  Tab complete  ·  Esc close"))

	return StyleCardToolCollapsed.Width(maxW + 2).Render(sb.String())
}
