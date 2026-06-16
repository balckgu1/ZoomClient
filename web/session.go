// web/session.go
//
// Session 封装 Web 模式下的 agent 会话状态和通信 channel。
// agentLoop 由 main 包提供（通过回调注入），Session 本身不包含 LLM 逻辑。
package web

import (
	"sync"
	"sync/atomic"
	"zoomClient/fsm"
)

// Command 表示一条来自前端 HTTP 请求的上行命令。
type Command struct {
	Action  string // "chat" | "clear" | "compact" | "exit"
	Message string // chat 命令的消息内容
}

// Event 表示一条要推送给前端浏览器的 SSE 事件
type Event struct {
	CH   string `json:"ch"`   // "agent" | "system" | "emotion"
	Data any    `json:"data"` // 事件负载
}

// Session 封装 Web 模式的一次 agent 会话
type Session struct {
	ID       string
	RecordID string // 关联到 session.SessionRecord 的 ID
	State    *fsm.State
	Model    string

	// CmdCh 接收前端 HTTP POST 发来的命令，由 main 包的主循环消费
	CmdCh chan Command
	// EventCh 由 SseEmitter 写入事件，由 SSE HTTP handler 消费
	EventCh chan Event

	// Busy 标记 agentLoop 是否正在运行（供 /api/status 查询）
	Busy atomic.Bool

	// 权限交互（模仿 ApiAsker 的 pending map 模式）
	permPending sync.Map // id -> chan permResponse
	permIDSeq   atomic.Int64

	// TurnCount 跟踪轮次
	TurnCount int
}

type permResponse struct {
	ok     bool
	reason string
}

// NewSession 创建一个 Web 模式的 Session
func NewSession(id, model string) *Session {
	return &Session{
		ID:      id,
		Model:   model,
		State:   &fsm.State{Messages: []fsm.Message{}, TurnCount: 0},
		CmdCh:   make(chan Command, 16),
		EventCh: make(chan Event, 256),
	}
}

// ─── 权限交互（供 WebAsker 使用） ───

// RequestPermission 推送权限询问事件并返回 requestID。
// 调用方随后通过 WaitForPermission 阻塞等待回复。
func (s *Session) RequestPermission(toolName string, argsJSON string, reason string) string {
	id := s.permIDSeq.Add(1)
	reqID := "web_perm_" + itoa(id)
	ch := make(chan permResponse, 1)
	s.permPending.Store(reqID, ch)

	s.EventCh <- Event{
		CH: "system",
		Data: map[string]string{
			"event":  "permission_ask",
			"id":     reqID,
			"tool":   toolName,
			"args":   argsJSON,
			"reason": reason,
		},
	}
	return reqID
}

// WaitForPermission 阻塞等待权限回复，返回 (allow, denyReason)。
func (s *Session) WaitForPermission(reqID string) (bool, string) {
	v, ok := s.permPending.Load(reqID)
	if !ok {
		return false, "permission request not found"
	}
	ch := v.(chan permResponse)
	defer s.permPending.Delete(reqID)

	resp, ok := <-ch
	if !ok {
		return false, "permission channel closed"
	}
	return resp.ok, resp.reason
}

// ResolvePermission 由 HTTP handler 调用，写入权限回复。
func (s *Session) ResolvePermission(id string, ok bool, reason string) {
	if v, loaded := s.permPending.Load(id); loaded {
		v.(chan permResponse) <- permResponse{ok: ok, reason: reason}
	}
}

// itoa 简单的 int64 → string，避免 import strconv。
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
