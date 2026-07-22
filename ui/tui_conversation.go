// ui/tui_conversation.go
//
// 对话流 viewport 组件：管理消息列表的追加与滚动。
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConversationView 管理对话流的 viewport 和内容。
type ConversationView struct {
	vp           viewport.Model
	content      strings.Builder
	cards        []CollapsibleCard
	followBottom bool // 是否自动跟随底部（新消息时自动滚到底）
}

// NewConversationView 创建一个新的对话流视图。
func NewConversationView() *ConversationView {
	vp := viewport.New(80, 20)
	vp.YPosition = 0
	return &ConversationView{
		vp:           vp,
		followBottom: true,
	}
}

// View 渲染对话流到指定宽度和高度。
func (c *ConversationView) View(width, height int) string {
	c.vp.Width = width
	c.vp.Height = height

	// 构建完整内容：文本 + 卡片
	var full strings.Builder
	full.WriteString(c.content.String())

	// 渲染卡片内嵌在对话流末尾
	if len(c.cards) > 0 {
		if full.Len() > 0 {
			full.WriteString("\n")
		}
		for i := range c.cards {
			card := &c.cards[i]
			full.WriteString(renderCard(card, width))
			full.WriteString("\n")
		}
	}

	c.vp.SetContent(full.String())

	// 仅当 followBottom 时自动滚到底部
	if c.followBottom {
		c.vp.GotoBottom()
	}

	// 渲染 viewport
	vpView := c.vp.View()

	// 如果内容超出可视区，叠加滚动条
	totalLines := c.vp.TotalLineCount()
	visibleLines := c.vp.VisibleLineCount()
	if totalLines > visibleLines && visibleLines > 0 {
		return c.renderWithScrollbar(vpView, width, height, totalLines, visibleLines)
	}

	return vpView
}

// renderWithScrollbar 在 viewport 右侧叠加滚动条。
func (c *ConversationView) renderWithScrollbar(vpView string, width, height, totalLines, visibleLines int) string {
	// 计算滚动条拇指位置和大小
	offset := c.vp.YOffset
	thumbH := visibleLines * visibleLines / totalLines
	if thumbH < 1 {
		thumbH = 1
	}
	thumbTop := offset * (visibleLines - thumbH) / (totalLines - visibleLines)
	if thumbTop < 0 {
		thumbTop = 0
	}
	if thumbTop+thumbH > visibleLines {
		thumbTop = visibleLines - thumbH
	}

	// 拆分 viewport 的每一行
	lines := strings.Split(vpView, "\n")

	// 确保行数匹配 visibleLines
	for len(lines) < visibleLines {
		lines = append(lines, "")
	}

	// 滚动条字符
	trackChar := "│"
	thumbChar := "█"
	scrollStyle := StyleScrollbar
	thumbStyle := StyleScrollThumb

	var sb strings.Builder
	for i := 0; i < visibleLines; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		// 截断或补全到 width-2（为滚动条留 2 列空间）
		lineW := lipgloss.Width(line)
		contentW := width - 2
		if contentW < 10 {
			contentW = 10
		}

		// 用空格补齐到 contentW
		padding := contentW - lineW
		if padding < 0 {
			// 截断（ANSI 安全：在末尾追加截断标记）
			padding = 0
		}

		// 滚动条字符
		var barChar string
		if i >= thumbTop && i < thumbTop+thumbH {
			barChar = thumbStyle.Render(thumbChar)
		} else {
			barChar = scrollStyle.Render(trackChar)
		}

		sb.WriteString(line)
		sb.WriteString(strings.Repeat(" ", padding))
		sb.WriteString(barChar)
		sb.WriteString("\n")
	}

	return sb.String()
}
func renderCard(card *CollapsibleCard, width int) string {
	contentW := width - 4
	if contentW < 10 {
		contentW = 10
	}

	switch card.Type {
	case CardToolCall:
		return renderToolCard(card, contentW)
	case CardReasoning:
		return renderReasoningCard(card, contentW)
	}
	return ""
}

// renderToolCard 渲染工具调用卡片。
func renderToolCard(card *CollapsibleCard, width int) string {
	headerIcon := "▸"
	borderStyle := StyleCardToolCollapsed
	if card.Expanded {
		headerIcon = "▾"
		borderStyle = StyleCardToolExpanded
	}
	if card.Focused {
		borderStyle = StyleCardFocused
	}

	header := StyleToolCall.Render(headerIcon + " " + card.ToolName)
	if card.Args != "" {
		header += "  " + StyleToolArgs.Render(card.Args)
	}

	if !card.Expanded {
		return borderStyle.Width(width).Render(header)
	}

	// 展开态：参数 + 结果 + 状态
	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n")
	sb.WriteString(StyleCardDivider.Render(strings.Repeat("─", width-2)))
	sb.WriteString("\n")

	// 参数
	if card.Args != "" {
		sb.WriteString(StyleToolCall.Render("  Arguments:"))
		sb.WriteString("\n")
		sb.WriteString(StyleToolArgs.Render("  " + card.Args))
		sb.WriteString("\n\n")
	}

	// 结果
	if card.Result != "" || card.IsError {
		sb.WriteString(StyleToolCall.Render("  Result:"))
		sb.WriteString("\n")
		resultText := card.Result
		const maxResultLen = 2000
		if len(resultText) > maxResultLen {
			resultText = resultText[:maxResultLen] + "\n…(truncated)"
		}
		if card.IsError {
			sb.WriteString(StyleError.Render(resultText))
		} else {
			sb.WriteString(StyleToolResult.Render(resultText))
		}
		sb.WriteString("\n\n")

		// 状态
		sb.WriteString(StyleCardDivider.Render(strings.Repeat("─", width-2)))
		sb.WriteString("\n")
		if card.IsError {
			sb.WriteString(StyleError.Render("  ✗ Failed"))
		} else {
			sb.WriteString(StyleToolOK.Render(fmt.Sprintf("  ✓ Success (%d bytes)", len(card.Result))))
		}
	}

	return borderStyle.Width(width).Render(sb.String())
}

// renderReasoningCard 渲染 reasoning 折叠卡片。
func renderReasoningCard(card *CollapsibleCard, width int) string {
	borderStyle := StyleCardReasonCollapsed
	if card.Expanded {
		borderStyle = StyleCardReasonExpanded
	}
	if card.Focused {
		borderStyle = StyleCardFocused
	}

	charCount := len(card.Thinking)
	header := StyleReasoningLabel.Render(fmt.Sprintf("💭 Thinking · %d chars", charCount))

	if !card.Expanded {
		return borderStyle.Width(width).Render(header)
	}

	// 展开态：完整 reasoning 文本
	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n")
	sb.WriteString(StyleCardDivider.Render(strings.Repeat("─", width-2)))
	sb.WriteString("\n")
	sb.WriteString(StyleReasoning.Width(width - 2).Render(card.Thinking))

	return borderStyle.Width(width).Render(sb.String())
}

// appendLine 追加一行到内容末尾。
func (c *ConversationView) appendLine(line string) {
	c.content.WriteString(line)
	c.content.WriteString("\n")
}

// AppendAssistant 追加助手回复（Markdown 渲染）。
func (c *ConversationView) AppendAssistant(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// 使用 viewport 实际宽度，但有保底值
	w := c.vp.Width
	if w < 40 {
		w = 80
	}
	rendered := RenderMarkdown(text, w-4) // 减去 padding
	c.appendLine(StyleAssistantLabel.Render("🤖 Assistant"))
	c.appendLine(rendered)
}

// AppendUser 追加用户输入到对话流。
func (c *ConversationView) AppendUser(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	c.appendLine(StyleUserLabel.Render("👤 You"))
	c.appendLine(StyleUser.Render(text))
}

// AppendSubAgent 追加子智能体调用。
func (c *ConversationView) AppendSubAgent(promptPreview string) {
	if len(promptPreview) > 120 {
		promptPreview = promptPreview[:120] + "…"
	}
	c.appendLine(StyleSubAgent.Render("🤖 sub_task: ") + StyleToolArgs.Render(promptPreview))
}

// AppendHookBlocked 追加 hook 阻止信息。
func (c *ConversationView) AppendHookBlocked(toolName, reason string) {
	c.appendLine(StyleHookBlocked.Render(fmt.Sprintf("⚠ hook blocked: %s  (%s)", toolName, reason)))
}

// AppendTodo 追加 Todo 计划面板。
func (c *ConversationView) AppendTodo(rendered string) {
	rendered = strings.TrimSpace(rendered)
	if rendered == "" || rendered == "No plan items." {
		return
	}
	title := StyleTodoTitle.Render("📋 Plan")
	body := StyleTodoPanel.Render(title + "\n" + rendered)
	c.appendLine(body)
}

// AppendCompact 追加压缩提示。
func (c *ConversationView) AppendCompact(beforeBytes, afterBytes int) {
	c.appendLine(StyleCompact.Render(fmt.Sprintf("🗜 compacted %d → %d bytes", beforeBytes, afterBytes)))
}

// AppendError 追加错误信息。
func (c *ConversationView) AppendError(scope, msg string) {
	body := fmt.Sprintf("✗ [%s] %s", scope, msg)
	c.appendLine(StyleError.Render(body))
}

// AppendInfo 追加普通提示信息。
func (c *ConversationView) AppendInfo(msg string) {
	c.appendLine(StyleSeparator.Render("· " + msg))
}

// AppendSeparator 追加回合分隔线。
func (c *ConversationView) AppendSeparator() {
	c.appendLine("")
	c.appendLine(StyleTurnSep.Render("· · ·"))
}

// Clear 清空对话流内容。
func (c *ConversationView) Clear() {
	c.content.Reset()
	c.cards = nil
}

// Update 处理 viewport 的更新（如鼠标滚动、键盘滚动）。
func (c *ConversationView) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	c.vp, cmd = c.vp.Update(msg)

	// 检测用户是否手动滚离底部 → 取消自动跟随
	if !c.vp.AtBottom() {
		c.followBottom = false
	} else {
		c.followBottom = true
	}

	return cmd
}

// ScrollUp 向上滚动半页。
func (c *ConversationView) ScrollUp() {
	c.followBottom = false
	c.vp.LineUp(c.vp.Height / 2)
}

// ScrollDown 向下滚动半页。
func (c *ConversationView) ScrollDown() {
	c.vp.LineDown(c.vp.Height / 2)
	if c.vp.AtBottom() {
		c.followBottom = true
	}
}

// ScrollToTop 跳到顶部。
func (c *ConversationView) ScrollToTop() {
	c.followBottom = false
	c.vp.GotoTop()
}

// ScrollToBottom 跳到底部并恢复自动跟随。
func (c *ConversationView) ScrollToBottom() {
	c.followBottom = true
	c.vp.GotoBottom()
}

// markNewContent 标记有新内容，触发自动跟随。
func (c *ConversationView) markNewContent() {
	c.followBottom = true
}

// ------- 可折叠卡片系统 -------

// CardType 卡片类型。
type CardType int

const (
	CardToolCall CardType = iota
	CardReasoning
)

// CollapsibleCard 表示一个可折叠的内容块。
type CollapsibleCard struct {
	Type     CardType
	ToolName string
	Args     string
	Result   string
	IsError  bool
	Thinking string
	Expanded bool
	Focused  bool
}

// cards 存储所有可折叠卡片。
func (c *ConversationView) cardsField() *[]CollapsibleCard {
	return &c.cards
}

// AppendToolCard 追加工具调用卡片（折叠态）。
func (c *ConversationView) AppendToolCard(data ToolCallData) {
	c.cards = append(c.cards, CollapsibleCard{
		Type:     CardToolCall,
		ToolName: data.Name,
		Args:     data.Args,
		Expanded: false,
	})
}

// CompleteToolCard 填入工具结果并自动展开。
func (c *ConversationView) CompleteToolCard(data ToolResultData) {
	// 找到最后一个未完成的 ToolCall 卡片
	for i := len(c.cards) - 1; i >= 0; i-- {
		if c.cards[i].Type == CardToolCall && c.cards[i].Result == "" {
			c.cards[i].Result = data.Content
			c.cards[i].IsError = data.IsError
			c.cards[i].Expanded = true
			// 折叠之前的卡片
			for j := range c.cards {
				if j != i {
					c.cards[j].Expanded = false
				}
			}
			return
		}
	}
}

// AppendReasoningCard 追加 reasoning 折叠卡片。
func (c *ConversationView) AppendReasoningCard(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	c.cards = append(c.cards, CollapsibleCard{
		Type:     CardReasoning,
		Thinking: text,
		Expanded: false,
	})
}

// CollapseAllCards 折叠所有卡片。
func (c *ConversationView) CollapseAllCards() {
	for i := range c.cards {
		c.cards[i].Expanded = false
		c.cards[i].Focused = false
	}
}

// ToggleCard 切换当前聚焦卡片的展开/折叠。
func (c *ConversationView) ToggleCard() {
	for i := range c.cards {
		if c.cards[i].Focused {
			c.cards[i].Expanded = !c.cards[i].Expanded
			return
		}
	}
}

// HasFocusedCard 返回是否有卡片处于聚焦状态。
func (c *ConversationView) HasFocusedCard() bool {
	for i := range c.cards {
		if c.cards[i].Focused {
			return true
		}
	}
	return false
}

// FocusCard 移动卡片焦点，delta 为正向右，负向左。
func (c *ConversationView) FocusCard(delta int) {
	if len(c.cards) == 0 {
		return
	}
	// 找到当前焦点
	current := -1
	for i := range c.cards {
		if c.cards[i].Focused {
			current = i
			c.cards[i].Focused = false
			break
		}
	}
	// 计算新焦点
	newFocus := current + delta
	if current == -1 {
		if delta > 0 {
			newFocus = 0
		} else {
			newFocus = len(c.cards) - 1
		}
	}
	if newFocus < 0 {
		newFocus = len(c.cards) - 1
	}
	if newFocus >= len(c.cards) {
		newFocus = 0
	}
	c.cards[newFocus].Focused = true
}
