package permission

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/charmbracelet/lipgloss"
)

// Asker 在权限决策为 ask 时，向真实用户请求确认。
type Asker interface {
	Ask(toolName string, args map[string]any, reason string) (ok bool, denyReason string)
}

// StdinAsker 通过标准输入交互式询问用户。
//
// 输出默认写到 stdout（与 CLI 前端保持一致的用户可见通道），
// 输入读 stdin。用户输入 y / yes 视为放行，其他（包括直接回车）一律拒绝。
type StdinAsker struct {
	Out io.Writer // 默认 os.Stdout
	In  io.Reader // 默认 os.Stdin
}

// NewStdinAsker 用默认 stdin / stdout 创建一个 StdinAsker。
func NewStdinAsker() *StdinAsker {
	return &StdinAsker{Out: os.Stdout, In: os.Stdin}
}

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

// Ask 实现 Asker 接口。
func (a *StdinAsker) Ask(toolName string, args map[string]any, reason string) (bool, string) {
	// Set io.writer
	out := a.Out
	if out == nil {
		out = os.Stdout
	}
	// Set io.reader
	in := a.In
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

// ─── API 模式：通过 NDJSON 协议向前端请求权限确认 ───

// ApiAsker 通过 NDJSON stdout/stdin 协议与 Tauri 前端交互完成权限确认。
//
// 工作流程：
//  1. Ask() 生成唯一 requestID，写入 NDJSON 行到 w（ch="permission"）；
//  2. Ask() 阻塞等待内部 channel；
//  3. ConcurrentReader 在后台持续读 stdin，收到 permission_reply 后调用 Resolve()；
//  4. Resolve() 向 channel 写入结果，Ask() 解除阻塞并返回。
type ApiAsker struct {
	w       io.Writer
	mu      sync.Mutex // 保护并发写入
	pending sync.Map   // requestID -> chan permissionResponse
	idSeq   atomic.Int64
}

type permissionResponse struct {
	ok     bool
	reason string
}

// NewApiAsker 创建一个 API 模式专用的 Asker，输出到指定 Writer（通常是 os.Stdout）。
func NewApiAsker(w io.Writer) *ApiAsker {
	return &ApiAsker{w: w}
}

// Ask 实现 Asker 接口。发送权限请求 NDJSON 事件并阻塞等待前端回复。
func (a *ApiAsker) Ask(toolName string, args map[string]any, reason string) (bool, string) {
	id := fmt.Sprintf("perm_%d", a.idSeq.Add(1))
	ch := make(chan permissionResponse, 1)
	a.pending.Store(id, ch)
	defer a.pending.Delete(id)

	argsJSON, _ := json.Marshal(args)
	msg := map[string]any{
		"ch": "permission",
		"data": map[string]string{
			"id":     id,
			"tool":   toolName,
			"reason": reason,
			"args":   string(argsJSON),
		},
	}
	b, _ := json.Marshal(msg)
	a.mu.Lock()
	fmt.Fprintf(a.w, "%s\n", b)
	a.mu.Unlock()

	resp, ok := <-ch
	if !ok {
		return false, "permission channel closed"
	}
	return resp.ok, resp.reason
}

// Resolve 由 ConcurrentReader 调用，将前端的权限回复路由到等待中的 Ask()。
func (a *ApiAsker) Resolve(id string, ok bool, reason string) {
	if v, loaded := a.pending.Load(id); loaded {
		v.(chan permissionResponse) <- permissionResponse{ok: ok, reason: reason}
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
