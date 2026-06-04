// ui/renderer.go
//
// Renderer 提供 cc-learn CLI 前端所有"用户可见事件"的渲染入口。
// 所有方法都是同步直接写 stdout，不与 zap 日志耦合。
package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// Renderer 是前端渲染器。stdout 默认指向 os.Stdout，stdin 默认指向 os.Stdin，
// 单元测试或重定向场景可替换这两个字段。
type Renderer struct {
	Out    io.Writer
	In     io.Reader
	reader *bufio.Reader
}

// New 创建一个使用 os.Stdin / os.Stdout 的 Renderer。
func New() *Renderer {
	r := &Renderer{Out: os.Stdout, In: os.Stdin}
	r.reader = bufio.NewReader(r.In)
	return r
}

// 内部辅助：直接 Println 到 Out。
func (r *Renderer) println(s string) {
	fmt.Fprintln(r.Out, s)
}

// ===================== 会话级 =====================

// PrintSessionStart 显示欢迎横幅与日志文件位置。
func (r *Renderer) PrintSessionStart(model, logPath string) {
	author := "balckgu1"
	banner := styleBanner.Render("ZoomClient  ·  Agent CLI")
	info := styleSeparator.Render(fmt.Sprintf("model: %s   |   logs: %s", model, logPath))
	auth := styleSeparator.Render(fmt.Sprintf("author: %s", author))
	hint := styleSeparator.Render("commands: /exit /clear /compact /help")
	r.println(banner)
	r.println(info)
	r.println(auth)
	r.println(hint)
	r.println("")
}

// PrintSessionEnd 显示会话结束信息。
func (r *Renderer) PrintSessionEnd(totalTurns int) {
	r.println("")
	r.println(styleSeparator.Render(fmt.Sprintf("── session ended · total turns: %d ──", totalTurns)))
}

// PrintTurnSeparator 在每一次 agentLoop 调用结束后画一条分隔。
func (r *Renderer) PrintTurnSeparator() {
	r.println(styleSeparator.Render(strings.Repeat("─", 60)))
}

// ===================== 用户输入 =====================

// PromptUser 在 stdout 渲染输入提示并从 stdin 读取一行。
//
// 返回去掉首尾空白的字符串；若读到 EOF（Ctrl+Z 或管道关闭），第二个返回值为 false。
func (r *Renderer) PromptUser() (string, bool) {
	fmt.Fprint(r.Out, styleUserPrompt.Render("👤 You> "))
	line, err := r.reader.ReadString('\n')
	if err != nil {
		// 把已读到的部分也返回给调用方（通常 EOF 时 line 为空）
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return "", false
		}
		return trimmed, true
	}
	return strings.TrimSpace(line), true
}

// ===================== 助手内容 =====================

// PrintAssistant 渲染助手最终文本
func (r *Renderer) PrintAssistant(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	r.println("")
	r.println(styleAssistant.Render("🤖 " + text))
}

// PrintReasoning 渲染 thinking 模式下的 reasoning_content
func (r *Renderer) PrintReasoning(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	const maxLen = 1500
	suffix := ""
	if len(text) > maxLen {
		text = text[:maxLen]
		suffix = "  …(reasoning truncated)"
	}
	r.println(styleReasoning.Render("💭 " + text + suffix))
}

// ===================== 工具调用 =====================

// PrintToolCall 渲染"模型即将调用某个工具"。
//
// argsPreview 由调用方裁剪到合理长度即可（一般 80~120 字符内）。
func (r *Renderer) PrintToolCall(name, argsPreview string) {
	head := styleToolCall.Render("▶ " + name)
	if argsPreview != "" {
		r.println(head + "  " + styleToolArgs.Render(argsPreview))
		return
	}
	r.println(head)
}

// PrintToolResult 渲染工具执行的摘要结果。
//
//   - 成功时显示前 3 行预览 + 行数/字节数；
//   - 出错时整体红色，但仍然走"摘要"形态而不是完整堆栈。
func (r *Renderer) PrintToolResult(name, content string, isError bool) {
	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	preview := lines
	if totalLines > 3 {
		preview = lines[:3]
	}

	header := fmt.Sprintf("└─ %s · %d lines / %d bytes", name, totalLines, len(content))
	if isError {
		header = "└─ " + name + " · error"
		r.println(styleError.Render(header))
		r.println(styleToolResult.Render(strings.Join(preview, "\n")))
		if totalLines > 3 {
			r.println(styleToolResult.Render(fmt.Sprintf("   …(%d more lines)", totalLines-3)))
		}
		return
	}

	r.println(styleToolResult.Render(header))
	if strings.TrimSpace(content) != "" {
		r.println(styleToolResult.Render(strings.Join(preview, "\n")))
		if totalLines > 3 {
			r.println(styleToolResult.Render(fmt.Sprintf("   …(%d more lines)", totalLines-3)))
		}
	}
}

// PrintHookBlocked 渲染 hook 拒绝某个工具调用的事件。
func (r *Renderer) PrintHookBlocked(toolName, reason string) {
	r.println(styleHookBlocked.Render(fmt.Sprintf("⚠ hook blocked: %s  (%s)", toolName, reason)))
}

// PrintSubAgent 渲染 sub_task 工具调用，单独样式以突出"子智能体"语义。
func (r *Renderer) PrintSubAgent(promptPreview string) {
	if len(promptPreview) > 120 {
		promptPreview = promptPreview[:120] + "…"
	}
	r.println(styleSubAgent.Render("🤖 sub_task: ") + styleToolArgs.Render(promptPreview))
}

// ===================== 计划 / 压缩 / 错误 =====================

// PrintTodoPanel 渲染当前会话计划（接受已渲染好的纯文本，由 TodoManager.Render() 提供）。
func (r *Renderer) PrintTodoPanel(rendered string) {
	rendered = strings.TrimSpace(rendered)
	if rendered == "" || rendered == "No plan items." {
		return
	}
	title := styleTodoTitle.Render("📋 Plan")
	body := styleTodoPanel.Render(title + "\n" + rendered)
	r.println("")
	r.println(body)
}

// PrintCompact 渲染整体压缩触发后的体积变化。
func (r *Renderer) PrintCompact(beforeBytes, afterBytes int) {
	r.println(styleCompact.Render(fmt.Sprintf("🗜 compacted %d → %d bytes", beforeBytes, afterBytes)))
}

// PrintError 渲染一条用户可见的错误（如 LLM 调用失败、工具执行致命错误）。
func (r *Renderer) PrintError(scope, msg string) {
	body := fmt.Sprintf("✗ [%s] %s", scope, msg)
	r.println(styleError.Render(body))
}

// PrintInfo 渲染一条普通的提示信息（用于斜杠命令反馈等）。
func (r *Renderer) PrintInfo(msg string) {
	r.println(styleSeparator.Render("· " + msg))
}

// ===================== Emitter 接口实现 =====================
// 以下方法使 Renderer 满足 emitter.Emitter 接口，
// 内部委托给现有的 PrintXxx 方法，保持 CLI 行为不变。

func (r *Renderer) EmitSessionStart(model, logPath string) { r.PrintSessionStart(model, logPath) }
func (r *Renderer) EmitSessionEnd(totalTurns int)          { r.PrintSessionEnd(totalTurns) }
func (r *Renderer) EmitTurnSeparator()                     { r.PrintTurnSeparator() }
func (r *Renderer) EmitAssistant(text string)              { r.PrintAssistant(text) }
func (r *Renderer) EmitReasoning(text string)              { r.PrintReasoning(text) }
func (r *Renderer) EmitDone()                              {} // CLI 模式不需要显式 done 信号
func (r *Renderer) EmitToolCall(name, argsPreview string)  { r.PrintToolCall(name, argsPreview) }
func (r *Renderer) EmitToolResult(name, content string, isError bool) {
	r.PrintToolResult(name, content, isError)
}
func (r *Renderer) EmitSubAgent(promptPreview string)                { r.PrintSubAgent(promptPreview) }
func (r *Renderer) EmitHookBlocked(toolName, reason string)          { r.PrintHookBlocked(toolName, reason) }
func (r *Renderer) EmitTodoPanel(rendered string)                    { r.PrintTodoPanel(rendered) }
func (r *Renderer) EmitCompact(beforeBytes, afterBytes int)          { r.PrintCompact(beforeBytes, afterBytes) }
func (r *Renderer) EmitError(scope, msg string)                      { r.PrintError(scope, msg) }
func (r *Renderer) EmitInfo(msg string)                              { r.PrintInfo(msg) }
func (r *Renderer) EmitEmotion(state string, meta map[string]string) {} // CLI 模式无宠物情绪
func (r *Renderer) EmitSystem(event string, data map[string]string)  {} // CLI 模式无系统事件流
