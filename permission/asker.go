package permission

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// askPromptStyle 权限询问面板样式：黄色圆角边框
var askPromptStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#FBBF24")).
	Foreground(lipgloss.Color("#FBBF24")).
	Bold(true).
	Padding(0, 1)

var askHintStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FBBF24")).
	Bold(true)

// Asker 接口
type Asker interface {
	Ask(toolName string, args map[string]any, reason string) (ok bool, denyReason string)
}

// StdinAsker 通过标准输入交互式询问用户
type StdinAsker struct {
	Out io.Writer // 默认 os.Stdout
	In  io.Reader // 默认 os.Stdin
}

// NewStdinAsker 用默认 stdin / stdout 创建一个 StdinAsker。
func NewStdinAsker() *StdinAsker {
	return &StdinAsker{
		Out: os.Stdout,
		In:  os.Stdin,
	}
}

// Ask 实现 Asker 接口
func (asker *StdinAsker) Ask(toolName string, args map[string]any, reason string) (bool, string) {
	// Set io.writer
	out := asker.Out
	if out == nil {
		out = os.Stdout
	}
	// Set io.reader
	in := asker.In
	if in == nil {
		in = os.Stdin
	}

	body := fmt.Sprintf("⚠  Permission required\ntool   : %s\nreason : %s\nargs   : %v", toolName, reason, args)
	fmt.Fprintln(out, askPromptStyle.Render(body))
	fmt.Fprint(out, askHintStyle.Render("approve? [y/N]: "))

	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return false, "no input, denied by default"
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	switch answer {
	case "y", "yes":
		return true, ""
	default:
		return false, "denied by user"
	}
}

// DenyAsker 永远拒绝。适合非交互式 / CI / 测试场景的安全默认值。
type DenyAsker struct{}

// Ask 实现 Asker 接口。
func (DenyAsker) Ask(toolName string, args map[string]any, reason string) (bool, string) {
	return false, "ask fallback denied (non-interactive)"
}

// AllowAsker 永远放行。仅用于本地调试或单元测试，禁止在生产环境使用。
type AllowAsker struct{}

// Ask 实现 Asker 接口。
func (AllowAsker) Ask(toolName string, args map[string]any, reason string) (bool, string) {
	return true, ""
}
