// ui/tui_logo.go
//
// 启动 LOGO 画面组件：简洁欢迎面板。
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// appVersion 当前版本号。
const appVersion = "v1.0.0"

// LogoScreen 生成启动 LOGO 的 lipgloss 渲染字符串。
func LogoScreen(width int) string {
	if width < 50 {
		return simpleLogo(width)
	}
	return welcomePanel(width)
}

// simpleLogo 简化版 LOGO（窄终端）。
func simpleLogo(width int) string {
	titleStyle := lipgloss.NewStyle().
		Foreground(cBlue).
		Bold(true).
		Width(width).
		Align(lipgloss.Center)

	subStyle := lipgloss.NewStyle().
		Foreground(cSubtext).
		Width(width).
		Align(lipgloss.Center)

	var sb strings.Builder
	sb.WriteString("\n\n")
	sb.WriteString(titleStyle.Render("ZoomClient"))
	sb.WriteString("\n\n")
	sb.WriteString(subStyle.Render("AI-Powered Agent CLI"))
	sb.WriteString("\n")
	sb.WriteString(subStyle.Render(appVersion))
	sb.WriteString("\n\n")
	return sb.String()
}

// welcomePanel 简洁欢迎面板。
func welcomePanel(width int) string {
	// 面板宽度自适应，最大 60
	panelW := width - 4
	if panelW > 60 {
		panelW = 60
	}
	if panelW < 40 {
		panelW = 40
	}

	// 标题样式
	titleStyle := lipgloss.NewStyle().
		Foreground(cBlue).
		Bold(true).
		Align(lipgloss.Center)

	// 副标题样式
	subtitleStyle := lipgloss.NewStyle().
		Foreground(cSubtext).
		Align(lipgloss.Center)

	// 信息标签样式
	labelStyle := lipgloss.NewStyle().
		Foreground(cOverlay)

	// 信息值样式
	valueStyle := lipgloss.NewStyle().
		Foreground(cText)

	var sb strings.Builder
	sb.WriteString("\n")

	// 标题
	sb.WriteString(titleStyle.Render("Welcome to ZoomClient"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("AI-Powered Agent CLI"))
	sb.WriteString("\n\n")

	// 信息行
	infoLines := []struct {
		label string
		value string
	}{
		{"Directory:", "~/workdir"}, // TODO: 从实际配置获取
		{"Model:", "gpt-4"},          // TODO: 从实际配置获取
		{"Version:", appVersion},
	}

	for _, info := range infoLines {
		line := fmt.Sprintf("%s %s",
			labelStyle.Render(info.label),
			valueStyle.Render(info.value))
		// 居中对齐
		padding := (panelW - lipgloss.Width(line)) / 2
		if padding < 0 {
			padding = 0
		}
		sb.WriteString(strings.Repeat(" ", padding))
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	// 使用圆角边框渲染面板
	panelStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(cBlue).
		Padding(1, 2).
		Width(panelW)

	return panelStyle.Render(sb.String())
}
