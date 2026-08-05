// ui/tui_permission.go
//
// TUI 权限确认浮层组件：以模态浮层形式显示权限确认对话框。
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PermissionOverlay 权限确认浮层。
type PermissionOverlay struct {
	visible  bool
	id       string
	toolName string
	args     string
	reason   string
	replyCh  chan bool // 用于向 TuiAsker 发送用户回复
}

// NewPermissionOverlay 创建一个新的权限确认浮层。
func NewPermissionOverlay() *PermissionOverlay {
	return &PermissionOverlay{}
}

// Show 显示权限确认浮层。
func (p *PermissionOverlay) Show(id, tool, args, reason string, replyCh chan bool) {
	p.visible = true
	p.id = id
	p.toolName = tool
	p.args = args
	p.reason = reason
	p.replyCh = replyCh
}

// Hide 隐藏浮层。
func (p *PermissionOverlay) Hide() {
	p.visible = false
	p.id = ""
	p.toolName = ""
	p.args = ""
	p.reason = ""
	p.replyCh = nil
}

// Visible 返回浮层是否可见。
func (p *PermissionOverlay) Visible() bool {
	return p.visible
}

// View 渲染权限确认浮层。
func (p *PermissionOverlay) View(width int) string {
	if !p.visible {
		return ""
	}

	// 面板宽度自适应，最大 70
	panelW := width - 4
	if panelW > 70 {
		panelW = 70
	}
	if panelW < 40 {
		panelW = 40
	}

	// 标题样式
	titleStyle := lipgloss.NewStyle().
		Foreground(cYellow).
		Bold(true)

	// 标签样式
	labelStyle := lipgloss.NewStyle().
		Foreground(cOverlay).
		Width(8)

	// 值样式
	valueStyle := lipgloss.NewStyle().
		Foreground(cText)

	// 提示样式
	hintStyle := lipgloss.NewStyle().
		Foreground(cSubtext).
		Italic(true)

	var sb strings.Builder

	// 标题
	sb.WriteString(titleStyle.Render("⚠  Permission Required"))
	sb.WriteString("\n\n")

	// 工具信息
	sb.WriteString(labelStyle.Render("tool:"))
	sb.WriteString(valueStyle.Render(p.toolName))
	sb.WriteString("\n")

	// 原因
	sb.WriteString(labelStyle.Render("reason:"))
	sb.WriteString(valueStyle.Render(p.reason))
	sb.WriteString("\n")

	// 参数（如果不太长）
	if len(p.args) > 0 && len(p.args) < 100 {
		sb.WriteString(labelStyle.Render("args:"))
		sb.WriteString(valueStyle.Render(p.args))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	// 提示
	sb.WriteString(hintStyle.Render("[y] Allow  ·  [n] Deny  ·  [Esc] Deny"))

	// 使用圆角边框渲染面板
	panelStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(cYellow).
		Padding(1, 2).
		Width(panelW)

	return panelStyle.Render(sb.String())
}

// HandleKey 处理键盘事件，返回是否拦截了按键。
func (p *PermissionOverlay) HandleKey(key string) bool {
	if !p.visible {
		return false
	}

	switch key {
	case "y", "Y":
		if p.replyCh != nil {
			p.replyCh <- true
		}
		p.Hide()
		return true
	case "n", "N", "esc":
		if p.replyCh != nil {
			p.replyCh <- false
		}
		p.Hide()
		return true
	}

	// 拦截所有其他按键，防止传递给底层组件
	return true
}

// GetID 返回当前权限请求的 ID。
func (p *PermissionOverlay) GetID() string {
	return p.id
}

// String 返回浮层的调试信息。
func (p *PermissionOverlay) String() string {
	if !p.visible {
		return "PermissionOverlay(hidden)"
	}
	return fmt.Sprintf("PermissionOverlay(tool=%s, id=%s)", p.toolName, p.id)
}
