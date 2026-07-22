// ui/tui_statusbar.go
//
// 状态栏组件：显示模型名、轮次、工作目录、日志路径。
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// StatusBar 底部状态栏组件。
type StatusBar struct{}

// NewStatusBar 创建一个新的状态栏。
func NewStatusBar() *StatusBar {
	return &StatusBar{}
}

// View 渲染状态栏。
func (s *StatusBar) View(width int, model string, turn int, workDir, logPath string) string {
	// 截断过长的工作目录
	if len(workDir) > 30 {
		workDir = "…" + workDir[len(workDir)-29:]
	}

	left := fmt.Sprintf("%s %s",
		StyleStatusLabel.Render("model:"),
		StyleStatusModel.Render(model))

	mid := fmt.Sprintf("%s %s  %s %s",
		StyleStatusLabel.Render("turn:"),
		StyleStatusValue.Render(fmt.Sprintf("%d", turn)),
		StyleStatusLabel.Render("ws:"),
		StyleStatusValue.Render(workDir))

	right := fmt.Sprintf("%s%s",
		StyleStatusLabel.Render("logs:"),
		StyleStatusValue.Render(logPath))

	// 三段式布局：左 | 中 | 右
	midW := lipgloss.Width(mid)
	rightW := lipgloss.Width(right)
	leftW := lipgloss.Width(left)

	// 计算间距
	gap1 := (width - leftW - midW - rightW) / 2
	if gap1 < 1 {
		gap1 = 1
	}
	gap2 := width - leftW - midW - rightW - gap1
	if gap2 < 1 {
		gap2 = 1
	}

	text := left + strings.Repeat(" ", gap1) + mid + strings.Repeat(" ", gap2) + right

	// 截断到宽度
	if lipgloss.Width(text) > width {
		text = text[:width]
	}

	return StyleStatusBar.Width(width).Render(text)
}
