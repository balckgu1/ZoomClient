// ui/tui_markdown.go
//
// Markdown 轻量渲染器：逐行状态机解析，支持标题/代码块/列表/粗斜体/引用块。
package ui

import (
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// parseState 解析状态
type parseState int

const (
	stateNormal     parseState = iota
	stateCodeBlock             // 围栏代码块内
	stateBlockquote            // 引用块内
	stateTable                 // 表格内
)

// RenderMarkdown 将 markdown 文本转为 lipgloss 样式字符串。
func RenderMarkdown(text string, width int) string {
	if width < 20 {
		width = 20
	}
	contentW := width - 4
	if contentW < 10 {
		contentW = 10
	}

	lines := strings.Split(text, "\n")
	var result strings.Builder
	state := stateNormal
	var codeLines []string
	var codeLang string
	var quoteLines []string

	flushCodeBlock := func() {
		if len(codeLines) == 0 {
			return
		}
		body := strings.Join(codeLines, "\n")
		// 截断过长代码
		const maxCodeLen = 5000
		const maxCodeLines = 50
		truncated := false
		if len(body) > maxCodeLen {
			body = body[:maxCodeLen]
			truncated = true
		}
		if len(codeLines) > maxCodeLines {
			codeLines = codeLines[:maxCodeLines]
			body = strings.Join(codeLines, "\n")
			truncated = true
		}

		// 尝试语法高亮
		highlighted := highlightCode(codeLang, body)

		// 语言标签
		var labelLine string
		if codeLang != "" {
			labelLine = StyleSeparator.Render(codeLang)
		}
		if truncated {
			if labelLine != "" {
				labelLine += " "
			}
			labelLine += StyleSeparator.Render("(truncated)")
		}

		if labelLine != "" {
			highlighted = labelLine + "\n" + highlighted
		}
		rendered := StyleCodeBlock.Width(contentW).Render(highlighted)
		result.WriteString(rendered)
		result.WriteString("\n")
		codeLines = nil
		codeLang = ""
	}

	flushBlockquote := func() {
		if len(quoteLines) == 0 {
			return
		}
		body := strings.Join(quoteLines, "\n")
		rendered := StyleBlockquote.Width(contentW).Render(body)
		result.WriteString(rendered)
		result.WriteString("\n")
		quoteLines = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 代码块切换
		if strings.HasPrefix(trimmed, "```") {
			if state == stateCodeBlock {
				flushCodeBlock()
				state = stateNormal
				continue
			}
			// 进入代码块，刷新之前的引用块
			if state == stateBlockquote {
				flushBlockquote()
			}
			state = stateCodeBlock
			codeLang = strings.TrimPrefix(trimmed, "```")
			continue
		}

		// 代码块内的行
		if state == stateCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}
		
		// 表格行（连续以 | 开头的行，在非引用/代码块内）
		if state == stateTable || isTableRow(trimmed) {
			if state != stateTable {
				if state == stateBlockquote {
					flushBlockquote()
				}
				state = stateTable
			}
			// 收集表格行，遇到非表格行时刷新
			result.WriteString(renderTableRow(trimmed))
			result.WriteString("\n")
			continue
		} else if state == stateTable {
			// 退出表格，继续处理当前行
			state = stateNormal
		}

		// 引用块
		if strings.HasPrefix(trimmed, ">") {
			if state != stateBlockquote {
				state = stateBlockquote
			}
			content := strings.TrimPrefix(trimmed, ">")
			content = strings.TrimSpace(content)
			quoteLines = append(quoteLines, content)
			continue
		}

		// 退出引用块
		if state == stateBlockquote {
			flushBlockquote()
			state = stateNormal
		}

		// 空行：刷新引用块
		if trimmed == "" {
			if state == stateBlockquote {
				flushBlockquote()
				state = stateNormal
			}
			result.WriteString("\n")
			continue
		}

		// 分割线
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			result.WriteString(StyleHr.Render(strings.Repeat("─", contentW)))
			result.WriteString("\n")
			continue
		}

		// 标题
		if strings.HasPrefix(trimmed, "# ") {
			rendered := renderInline(StyleH1.Render(trimmed[2:]))
			result.WriteString(rendered)
			result.WriteString("\n")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			rendered := renderInline(StyleH2.Render(trimmed[3:]))
			result.WriteString(rendered)
			result.WriteString("\n")
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			rendered := renderInline(StyleH3.Render(trimmed[4:]))
			result.WriteString(rendered)
			result.WriteString("\n")
			continue
		}
		if strings.HasPrefix(trimmed, "#### ") || strings.HasPrefix(trimmed, "##### ") || strings.HasPrefix(trimmed, "###### ") {
			// H4-H6: 使用 H3 样式
			i := 5
			if trimmed[4] == '#' {
				i = 6
				if len(trimmed) > 6 && trimmed[5] == '#' {
					i = 7
				}
			}
			if i < len(trimmed) {
				rendered := renderInline(StyleH3.Render(trimmed[i:]))
				result.WriteString(rendered)
				result.WriteString("\n")
			}
			continue
		}

		// 无序列表
		if isUnorderedListItem(trimmed) {
			content := trimmed[2:]
			bullet := StyleListBullet.Render("  •")
			rendered := renderInline(content)
			result.WriteString(bullet)
			result.WriteString(" ")
			result.WriteString(rendered)
			result.WriteString("\n")
			continue
		}

		// 有序列表
		if isOrderedListItem(trimmed) {
			// 找到 ". " 之后的内容
			idx := strings.Index(trimmed, ". ")
			if idx > 0 {
				content := trimmed[idx+2:]
				num := StyleListBullet.Render("  " + trimmed[:idx] + ".")
				rendered := renderInline(content)
				result.WriteString(num)
				result.WriteString(" ")
				result.WriteString(rendered)
				result.WriteString("\n")
			}
			continue
		}

		// 普通文本：内联渲染
		rendered := renderInline(trimmed)
		result.WriteString(rendered)
		result.WriteString("\n")
	}

	// 刷新残留状态
	if state == stateCodeBlock {
		flushCodeBlock()
	}
	if state == stateBlockquote {
		flushBlockquote()
	}

	return result.String()
}

// isUnorderedListItem 检查是否为无序列表项（- 或 * 开头）。
func isUnorderedListItem(line string) bool {
	return (strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ")) && len(line) > 2
}

// isOrderedListItem 检查是否为有序列表项（数字. 开头）。
func isOrderedListItem(line string) bool {
	if len(line) < 3 {
		return false
	}
	// 简单检查：第一个字符是数字，后面跟 ". "
	for i, ch := range line {
		if ch == '.' && i > 0 && i < len(line)-1 && line[i+1] == ' ' {
			return true
		}
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return false
}

var (
	boldRe       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	italicRe     = regexp.MustCompile(`\*(.+?)\*`)
	inlineCodeRe = regexp.MustCompile("`(.+?)`")
)

// renderInline 处理内联样式：粗体、斜体、行内代码。
func renderInline(text string) string {
	// 行内代码（必须在粗体/斜体之前处理，避免冲突）
	result := inlineCodeRe.ReplaceAllStringFunc(text, func(m string) string {
		inner := m[1 : len(m)-1]
		return StyleInlineCode.Render(inner)
	})

	// 粗体
	boldStyle := lipgloss.NewStyle().Bold(true).Foreground(cText)
	result = boldRe.ReplaceAllStringFunc(result, func(m string) string {
		inner := m[2 : len(m)-2]
		return boldStyle.Render(inner)
	})

	// 斜体
	italicStyle := lipgloss.NewStyle().Italic(true).Foreground(cSubtext)
	result = italicRe.ReplaceAllStringFunc(result, func(m string) string {
		inner := m[1 : len(m)-1]
		return italicStyle.Render(inner)
	})

	return result
}

// highlightCode 使用 chroma 对代码进行语法高亮。
// 若语言无法识别则降级为纯文本输出。
func highlightCode(lang, code string) string {
	if lang == "" || code == "" {
		return code
	}

	// 规范化语言名（chroma 支持别名）
	lexer := lexers.Get(lang)
	if lexer == nil {
		// 尝试分析内容自动检测语言
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		// 无法识别，降级为纯文本
		return code
	}

	// 使用终端友好的 formatter（256色）
	formatter := formatters.Get("terminal256")
	if formatter == nil {
		return code
	}

	// 使用 Catppuccin Mocha 风格（与 TUI 配色一致）
	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}

	// 词法分析
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}

	var sb strings.Builder
	err = formatter.Format(&sb, style, iterator)
	if err != nil {
		return code
	}

	return sb.String()
}

// isTableRow 判断是否为 Markdown 表格行（包含 | 且不是引用块）。
func isTableRow(line string) bool {
	return strings.Contains(line, "|") && !strings.HasPrefix(strings.TrimSpace(line), ">")
}

// renderTableRow 渲染单行表格数据，按列对齐。
func renderTableRow(line string) string {
	// 去除首尾的 |
	trimmed := strings.Trim(line, "| ")
	if trimmed == "" {
		return ""
	}

	cells := strings.Split(trimmed, "|")
	var rendered strings.Builder
	sepStyle := lipgloss.NewStyle().Foreground(cOverlay)
	rendered.WriteString(sepStyle.Render("│"))

	for i, cell := range cells {
		cell = strings.TrimSpace(cell)
		// 如果是分隔行 (---, :---, ---:, :---:)，渲染为分隔线
		if isTableSeparator(cell) {
			rendered.WriteString(sepStyle.Render(strings.Repeat("─", max(6, len([]rune(cell))))))
		} else {
			rendered.WriteString(renderInline(cell))
		}
		if i < len(cells)-1 {
			rendered.WriteString(sepStyle.Render("│"))
		}
	}
	rendered.WriteString(sepStyle.Render("│"))
	return rendered.String()
}

// isTableSeparator 判断是否为表格分隔行（如 ---, :---, ---:, :---:）。
func isTableSeparator(cell string) bool {
	trimmed := strings.TrimSpace(cell)
	if trimmed == "" {
		return false
	}
	// 去除首尾的冒号
	trimmed = strings.Trim(trimmed, ":")
	// 检查是否全部由 - 组成
	for _, ch := range trimmed {
		if ch != '-' {
			return false
		}
	}
	return len(trimmed) > 0
}
