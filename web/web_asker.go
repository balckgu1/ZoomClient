package web

import (
	"encoding/json"
	"time"
)

// WebAsker 是 Web 模式的权限询问器
// 通过 Session 的 EventCh 推送 permission_ask 事件，
// 通过 Session 的 permPending channel 接收前端回复。
type WebAsker struct {
	session *Session
	timeout time.Duration // 等待超时，超时后自动拒绝
}

// NewWebAsker 创建 WebAsker，默认 60 秒超时
func NewWebAsker(s *Session) *WebAsker {
	return &WebAsker{
		session: s,
		timeout: 60 * time.Second,
	}
}

// Ask 实现 permission.Asker 接口，推送 permission_ask 事件到前端，阻塞等待回复或超时
func (a *WebAsker) Ask(toolName string, args map[string]any, reason string) (bool, string) {
	argsJSON, _ := json.Marshal(args)

	// 推送权限询问事件
	reqID := a.session.RequestPermission(toolName, string(argsJSON), reason)

	// 带超时的阻塞等待
	ch := make(chan permResult, 1)
	go func() {
		ok, denyReason := a.session.WaitForPermission(reqID)
		ch <- permResult{ok: ok, reason: denyReason}
	}()

	select {
	case result := <-ch:
		return result.ok, result.reason
	case <-time.After(a.timeout):
		return false, "permission request timed out (60s)"
	}
}

type permResult struct {
	ok     bool
	reason string
}
