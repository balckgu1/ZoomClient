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

// 对话内容的字符数上限，防止无限增长导致 viewport 渲染异常
const maxContentChars = 100000

// ConversationView 管理对话流的 viewport 和内容。
type ConversationView struct {
	vp           viewport.Model
	content      strings.Builder
	cards        []CollapsibleCard
	followBottom bool // 是否自动跟随底部（新消息时自动滚到底）

	// 渲染缓存：避免每帧完整重建内容
	contentCache string
	dirty        bool

	// 内容截断标记
	truncated    bool
	truncatedAt  int // 截断位置（字节偏移），用于增量修剪
}

// NewConversationView 创建一个新的对话流视图。
func NewConversationView() *ConversationView {
	vp := viewport.New(80, 20)
	vp.YPosition = 0
	return &ConversationView{
		vp:           vp,
		followBottom: true,
		dirty:        true, // 初始需要构建内容
	}
}

// View 渲染对话流到指定宽度和高度。
func (c *ConversationView) View(width, height int) string {
	c.vp.Width = width
	c.vp.Height = height

	// 仅在内容变化时重建缓存（脏标记机制）
	if c.dirty {
		var full strings.Builder

		// 如果内容曾被裁剪，在顶部添加提示
		if c.truncated {
			full.WriteString(StyleSeparator.Render("… (earlier content trimmed) …"))
			full.WriteString("\n\n")
		}

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

		c.contentCache = full.String()
		c.dirty = false
	}

	// 先以完整宽度设置内容，计算行数判断是否需要滚动条
	c.vp.Width = width
	c.vp.SetContent(c.contentCache)
	c.vp.Height = height

	totalLines := c.vp.TotalLineCount()
	visibleLines := c.vp.VisibleLineCount()
	hasScrollbar := totalLines > visibleLines && visibleLines > 0

	// 如果有滚动条，需要以更窄的宽度重新渲染，为滚动条留空间
	if hasScrollbar {
		scrollbarWidth := 1 // 从 2 改为 1，更精致
		c.vp.Width = width - scrollbarWidth
		if c.vp.Width < 10 {
			c.vp.Width = 10
		}
		c.vp.SetContent(c.contentCache)
		totalLines = c.vp.TotalLineCount()
		visibleLines = c.vp.VisibleLineCount()
	}

	// 确保 YOffset 在有效范围内
	if c.vp.YOffset < 0 {
		c.vp.YOffset = 0
	}
	maxOffset := totalLines - visibleLines
	if maxOffset < 0 {
		maxOffset = 0
	}
	if c.vp.YOffset > maxOffset {
		c.vp.YOffset = maxOffset
	}

	// 仅当 followBottom 时自动滚到底部
	if c.followBottom {
		c.vp.GotoBottom()
	}

	// 渲染 viewport（viewport.View() 会根据 Height 自动截断）
	vpView := c.vp.View()

	// 如果内容超出可视区，叠加滚动条
	if hasScrollbar {
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
	thumbTop := 0
	if totalLines > visibleLines {
		thumbTop = offset * (visibleLines - thumbH) / (totalLines - visibleLines)
	}
	if thumbTop < 0 {
		thumbTop = 0
	}
	if thumbTop+thumbH > visibleLines {
		thumbTop = visibleLines - thumbH
	}

	// 拆分 viewport 的每一行
	lines := strings.Split(vpView, "\n")

	// 确保行数完全匹配 visibleLines（截断或补齐）
	if len(lines) > visibleLines {
		lines = lines[:visibleLines]
	}
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
		line := lines[i]
		
		// 内容宽度 = 总宽度 - 滚动条宽度(1)
		contentW := width - 1
		if contentW < 10 {
			contentW = 10
		}

		// 截断或补全到 contentW
		lineW := lipgloss.Width(line)
		if lineW > contentW {
			// 行内容超出宽度，需要截断（ANSI 安全截断）
			line = truncateLineANSI(line, contentW)
			lineW = contentW
		}
		padding := contentW - lineW

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
		if i < visibleLines-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// truncateLineANSI 安全截断包含 ANSI 转义序列的行到指定显示宽度。
func truncateLineANSI(line string, maxWidth int) string {
	runes := []rune(line)
	var result []rune
	visible := 0
	inEscape := false
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if ch == '\x1b' {
			inEscape = true
			result = append(result, ch)
			continue
		}
		if inEscape {
			result = append(result, ch)
			if ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' {
				inEscape = false
			}
			continue
		}
		if visible >= maxWidth {
			break
		}
		result = append(result, ch)
		visible++
	}
	// 如果确实发生了截断，追加截断标记
	if visible >= maxWidth && len(runes) > len(result) {
		result = append(result, []rune("…")...)
	}
	return string(result)
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

// renderToolCard 渲染工具调用卡片（简化版，无边框）。
func renderToolCard(card *CollapsibleCard, width int) string {
	headerIcon := "▸"
	if card.Expanded {
		headerIcon = "▾"
	}

	// 根据状态选择样式
	var headerStyle lipgloss.Style
	if card.IsError {
		headerStyle = StyleError
	} else if card.Focused {
		headerStyle = StyleCardFocusedSimple
	} else {
		headerStyle = StyleToolCall
	}

	header := headerStyle.Render(headerIcon + " " + card.ToolName)

	if !card.Expanded {
		return header
	}

	// 展开态：参数 + 结果 + 状态（使用缩进区分层次）
	var sb strings.Builder
	sb.WriteString(header)
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
			sb.WriteString(StyleError.Render("  " + resultText))
		} else {
			sb.WriteString(StyleToolResult.Render("  " + resultText))
		}
		sb.WriteString("\n\n")

		// 状态
		if card.IsError {
			sb.WriteString(StyleError.Render("  ✗ Failed"))
		} else {
			sb.WriteString(StyleToolOK.Render(fmt.Sprintf("  ✓ Success (%d bytes)", len(card.Result))))
		}
	}

	return sb.String()
}

// renderReasoningCard 渲染 reasoning 折叠卡片（简化版，无边框）。
func renderReasoningCard(card *CollapsibleCard, width int) string {
	charCount := len(card.Thinking)
	
	var headerStyle lipgloss.Style
	if card.Focused {
		headerStyle = StyleCardFocusedSimple
	} else {
		headerStyle = StyleReasoningLabel
	}
	
	header := headerStyle.Render(fmt.Sprintf("💭 Thinking · %d chars", charCount))

	if !card.Expanded {
		return header
	}

	// 展开态：完整 reasoning 文本（使用缩进）
	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n")
	sb.WriteString(StyleReasoning.Render("  " + card.Thinking))

	return sb.String()
}

// appendLine 追加一行到内容末尾，超出上限时从头部裁剪。
func (c *ConversationView) appendLine(line string) {
	c.content.WriteString(line)
	c.content.WriteString("\n")

	// 内容超出上限时，从头部裁剪一半，保留尾部最新对话
	if c.content.Len() > maxContentChars {
		full := c.content.String()
		// 找到中段附近的行边界，避免截断在行中间
		trimPos := len(full) - maxContentChars/2
		// 向后寻找最近的换行符
		newlineIdx := strings.Index(full[trimPos:], "\n")
		if newlineIdx >= 0 {
			trimPos += newlineIdx + 1
		}
		// 跳过开头可能残留的不完整行
		if trimPos < len(full) {
			trimmed := full[trimPos:]
			c.content.Reset()
			c.content.WriteString(trimmed)
			c.truncated = true
			c.truncatedAt = trimPos
		}
	}
	c.dirty = true // 内容变化，标记缓存失效
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
	c.appendLine("") // 在助手回复前添加空行
	c.appendLine(StyleAssistantLabel.Render("🤖 Assistant"))
	c.appendLine(rendered)
}

// AppendUser 追加用户输入到对话流。
func (c *ConversationView) AppendUser(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	c.appendLine("") // 在用户消息前添加空行
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
	c.truncated = false
	c.truncatedAt = 0
	c.dirty = true // 内容变化，标记缓存失效
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
	c.dirty = true // 卡片变化，标记缓存失效
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
			c.dirty = true // 卡片变化，标记缓存失效
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
	c.dirty = true // 卡片变化，标记缓存失效
}

// CollapseAllCards 折叠所有卡片。
func (c *ConversationView) CollapseAllCards() {
	for i := range c.cards {
		c.cards[i].Expanded = false
		c.cards[i].Focused = false
	}
	c.dirty = true // 卡片状态变化，标记缓存失效
}

// ToggleCard 切换当前聚焦卡片的展开/折叠。
func (c *ConversationView) ToggleCard() {
	for i := range c.cards {
		if c.cards[i].Focused {
			c.cards[i].Expanded = !c.cards[i].Expanded
			c.dirty = true // 卡片状态变化，标记缓存失效
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
	c.dirty = true // 卡片焦点变化，标记缓存失效
}
