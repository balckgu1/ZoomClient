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
func (s *StatusBar) View(width int, model string, turn int, sessionTitle string, tokenEst int, workDir, logPath string) string {
	// 窄终端(≤50)切换为缩略模式，仅显示 model + turn
	if width <= 50 {
		text := fmt.Sprintf("%s %s  %s %d",
			StyleStatusLabel.Render("model:"),
			StyleStatusModel.Render(model),
			StyleStatusLabel.Render("turn:"),
			turn)
		return StyleStatusBar.Width(width).Render(text)
	}

	// 截断过长的工作目录和会话标题
	if len(workDir) > 25 {
		workDir = "…" + workDir[len(workDir)-24:]
	}
	if len(sessionTitle) > 20 {
		sessionTitle = sessionTitle[:17] + "…"
	}
	if sessionTitle == "" {
		sessionTitle = "-"
	}

	// token 格式化
	tokenStr := fmt.Sprintf("%dk", tokenEst/1000)
	if tokenEst < 1000 {
		tokenStr = fmt.Sprintf("%d", tokenEst)
	}

	left := fmt.Sprintf("%s %s",
		StyleStatusLabel.Render("model:"),
		StyleStatusModel.Render(model))

	mid := fmt.Sprintf("%s %s  %s %d  %s %s",
		StyleStatusLabel.Render("session:"),
		StyleStatusValue.Render(sessionTitle),
		StyleStatusLabel.Render("turn:"),
		turn,
		StyleStatusLabel.Render("tokens:"),
		StyleStatusValue.Render(tokenStr))

	right := fmt.Sprintf("%s %s  %s %s",
		StyleStatusLabel.Render("ws:"),
		StyleStatusValue.Render(workDir),
		StyleStatusLabel.Render("logs:"),
		StyleStatusValue.Render(logPath))

	// 三段式布局：左 | 中 | 右
	midW := lipgloss.Width(mid)
	rightW := lipgloss.Width(right)
	leftW := lipgloss.Width(left)

	// 计算间距
	available := width - leftW - midW - rightW
	if available < 2 {
		// 空间不足，简化中间区域
		mid = fmt.Sprintf("%s %d  %s %s",
			StyleStatusLabel.Render("turn:"),
			turn,
			StyleStatusLabel.Render("tokens:"),
			StyleStatusValue.Render(tokenStr))
		midW = lipgloss.Width(mid)
		available = width - leftW - midW - rightW
	}
	if available < 0 {
		// 仍不够，仅显示核心信息
		text := fmt.Sprintf("%s %s  %s %d",
			StyleStatusLabel.Render("model:"),
			StyleStatusModel.Render(model),
			StyleStatusLabel.Render("turn:"),
			turn)
		return StyleStatusBar.Width(width).Render(text)
	}

	gap1 := available / 2
	if gap1 < 1 {
		gap1 = 1
	}
	gap2 := available - gap1
	if gap2 < 1 {
		gap2 = 1
	}

	text := left + strings.Repeat(" ", gap1) + mid + strings.Repeat(" ", gap2) + right

	return StyleStatusBar.Width(width).Render(text)
}
