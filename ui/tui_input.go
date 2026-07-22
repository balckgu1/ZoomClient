// ui/tui_input.go
//
// 多行输入组件：基于 bubbles/textarea，Alt+Enter 发送。
package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// InputArea 包装 textarea，提供多行输入能力。
type InputArea struct {
	ta       textarea.Model
	thinking bool
}

// NewInputArea 创建一个新的输入区域。
func NewInputArea() *InputArea {
	ta := textarea.New()
	ta.Placeholder = "Message cc-learn..."
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetHeight(2)
	ta.Focus()

	// 将 textarea 的换行键从 Enter 改为 Shift+Enter
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter"))

	return &InputArea{ta: ta}
}

// Init 返回 textarea 的初始化命令（光标闪烁）。
func (i *InputArea) Init() tea.Cmd {
	return i.ta.Focus()
}

// Update 将消息转发给 textarea 并返回其 Cmd。
func (i *InputArea) Update(msg tea.Msg) tea.Cmd {
	if i.thinking {
		return nil
	}
	var cmd tea.Cmd
	i.ta, cmd = i.ta.Update(msg)
	return cmd
}

// View 渲染输入区域。
func (i *InputArea) View(width, height int) string {
	i.ta.SetWidth(width - 4)
	i.ta.SetHeight(2)

	borderStyle := StyleInputBorder
	if !i.thinking && i.ta.Focused() {
		borderStyle = StyleInputBorderFocused
	}

	hintText := " Enter 发送  ·  Shift+Enter 换行  ·  Ctrl+C 退出"
	if i.thinking {
		hintText = " ●  Thinking..."
	}

	hint := StyleSeparator.Render(hintText)
	return borderStyle.Render(i.ta.View() + "\n" + hint)
}

// Value 获取当前输入内容（trim 后）。
func (i *InputArea) Value() string {
	return strings.TrimSpace(i.ta.Value())
}

// RawValue 获取原始输入内容（不 trim，用于 slash 检测）。
func (i *InputArea) RawValue() string {
	return i.ta.Value()
}

// SetValue 设置输入内容。
func (i *InputArea) SetValue(v string) {
	i.ta.SetValue(v)
	i.ta.CursorEnd()
}

// Reset 清空输入内容。
func (i *InputArea) Reset() {
	i.ta.Reset()
}

// SetThinking 设置"思考中"状态。
func (i *InputArea) SetThinking(v bool) {
	i.thinking = v
	if v {
		i.ta.Placeholder = "Agent is thinking..."
		i.ta.Blur()
	} else {
		i.ta.Placeholder = "Message cc-learn..."
		i.ta.Focus()
	}
}
