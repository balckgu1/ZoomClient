// ui/tui_asker.go
//
// TUI 模式的权限确认 Asker 实现：通过事件机制与 bubbletea 集成。
package ui

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// TuiAsker 实现 permission.Asker 接口，通过 TUI 事件机制进行权限确认。
type TuiAsker struct {
	eventCh chan UIEvent
	pending sync.Map   // requestID -> chan bool
	idSeq   atomic.Int64
}

// NewTuiAsker 创建一个 TUI 模式专用的 Asker。
func NewTuiAsker(eventCh chan UIEvent) *TuiAsker {
	return &TuiAsker{
		eventCh: eventCh,
	}
}

// Ask 实现 permission.Asker 接口。发送权限请求事件到 TUI 并阻塞等待用户回复。
func (a *TuiAsker) Ask(toolName string, args map[string]any, reason string) (bool, string) {
	// 生成唯一请求 ID
	id := fmt.Sprintf("perm_%d", a.idSeq.Add(1))
	
	// 创建回复通道
	replyCh := make(chan bool, 1)
	a.pending.Store(id, replyCh)
	defer a.pending.Delete(id)

	// 序列化参数
	argsJSON, _ := json.Marshal(args)

	// 发送权限请求事件到 TUI
	a.eventCh <- UIEvent{
		Type: EventPermissionAsk,
		Data: PermissionAskData{
			ID:     id,
			Tool:   toolName,
			Args:   string(argsJSON),
			Reason: reason,
		},
	}

	// 阻塞等待用户回复
	ok := <-replyCh
	
	if ok {
		return true, ""
	}
	return false, "denied by user"
}

// Resolve 由 TUI 调用，将用户的权限回复路由到等待中的 Ask()。
func (a *TuiAsker) Resolve(id string, ok bool) {
	if v, loaded := a.pending.Load(id); loaded {
		v.(chan bool) <- ok
	}
}

// GetReplyChannel 获取指定请求 ID 的回复通道（用于 PermissionOverlay）。
func (a *TuiAsker) GetReplyChannel(id string) chan bool {
	if v, loaded := a.pending.Load(id); loaded {
		return v.(chan bool)
	}
	return nil
}
