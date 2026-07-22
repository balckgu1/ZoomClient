// ui/tui_logo.go
//
// 启动 LOGO 画面组件：cc-learn ASCII art + 版本信息。
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
	return fullLogo(width)
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
	sb.WriteString(titleStyle.Render("cc-learn"))
	sb.WriteString("\n\n")
	sb.WriteString(subStyle.Render("AI-Powered Agent CLI"))
	sb.WriteString("\n")
	sb.WriteString(subStyle.Render(fmt.Sprintf("%s  ·  github.com/cc-learn", appVersion)))
	sb.WriteString("\n\n")
	return sb.String()
}

// fullLogo 完整版彩色 LOGO。
func fullLogo(width int) string {
	// 使用 Unicode box-drawing 风格的简洁 logo
	logoLines := []string{
		`   ▄████▄   ▄████▄      ██▓    ▓█████ ▄▄▄       ██▀███   ███▄    █ `,
		`  ▒██▀ ▀█  ▒██▀ ▀█     ▓██▒    ▓█   ▀▒████▄    ▓██ ▒ ██▒ ██ ▀█   █ `,
		`  ▒▓█    ▄ ▒▓█    ▄    ▒██░    ▒███  ▒██  ▀█▄  ▓██ ░▄█ ▒▓██  ▀█ ██▒`,
		`  ▒▓▓▄ ▄██▒▒▓▓▄ ▄██▒   ▒██░    ▒▓█  ▄░██▄▄▄▄██ ▒██▀▀█▄  ▓██▒  ▐▌██▒`,
		`  ▒ ▓███▀ ░▒ ▓███▀ ░   ░██████▒░▒████▒▓█   ▓██▒░██▓ ▒██▒▒██░   ▓██░`,
		`  ░ ░▒ ▒  ░░ ░▒ ▒  ░   ░ ▒░▓  ░░░ ▒░ ░▒▒   ▓▒█░░ ▒▓ ░▒▓░░ ▒░   ▒ ▒ `,
		`    ░  ▒     ░  ▒      ░ ░ ▒  ░ ░ ░  ░ ▒   ▒▒ ░  ░▒ ░ ▒░░ ░░   ░ ▒░`,
		`  ░         ░             ░ ░      ░    ░   ▒     ░░   ░    ░   ░ ░ `,
		`  ░ ░       ░ ░             ░  ░   ░  ░     ░  ░   ░              ░ `,
		`  ░         ░                                                       `,
	}

	// 渐变色映射
	colors := []lipgloss.Color{
		cBlue, cBlue, cSky, cSky, cTeal, cTeal, cMauve, cMauve, cPink, cPink,
	}

	infoStyle := lipgloss.NewStyle().
		Foreground(cSubtext).
		Width(width).
		Align(lipgloss.Center)

	var sb strings.Builder
	sb.WriteString("\n")

	for i, line := range logoLines {
		lineStyle := lipgloss.NewStyle().
			Foreground(colors[i]).
			Bold(true).
			Width(width).
			Align(lipgloss.Center)
		sb.WriteString(lineStyle.Render(line))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(infoStyle.Render("AI-Powered Agent CLI"))
	sb.WriteString("\n")
	sb.WriteString(infoStyle.Render(fmt.Sprintf("%s  ·  github.com/cc-learn", appVersion)))
	sb.WriteString("\n\n")

	return sb.String()
}
