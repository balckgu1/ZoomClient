// web/handlers_context.go
//
// 上下文洞察端点：GET /api/context/timeline 返回轻量头部快照（当前组成、计数、
// 最新请求锚点），GET /api/context/timeline/detail 返回按需拉取的详情载荷
// （逐请求历史、事件、存活表面与归档）。折叠器未注入时返回空快照而非错误。
package web

import (
	"encoding/json"
	"net/http"

	"zoomClient/ctxinsight"
)

// handleContextTimeline 处理 GET /api/context/timeline：
// 直接读折叠器（内部读写锁保护，与 agentLoop 写入互斥安全）。
// 不走 SSE 缓存：LLM 失败等中断路径不会推送事件，缓存会滞留旧值。
func (s *Server) handleContextTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.recorder.Head())
}

// handleContextTimelineDetail 处理 GET /api/context/timeline/detail：
// 面板打开时才拉取的全量详情，读锁下拷贝，与 agentLoop 写入互斥安全。
func (s *Server) handleContextTimelineDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.recorder.Detail())
}

// pushTimelineHead 将最新头部快照经 SSE 推送。
// 供会话切换/新建/删除等 HTTP handler 在改动折叠器后主动刷新前端。
func (s *Server) pushTimelineHead() {
	s.session.EventCh <- Event{CH: "system", Data: map[string]any{
		"event": "context_timeline",
		"head":  s.recorder.Head(),
	}}
}

// restoreRecorder 会话加载时恢复折叠器状态；快照缺失或损坏时重置为空。
// 阈值以当前配置为准（快照中的旧阈值可能已过期）；模型名保持当前值——
// 后端加载会话并不切换模型客户端，洞察应反映实际在用的模型。
func (s *Server) restoreRecorder(id string) {
	if s.recorder == nil {
		return
	}
	limit := 0
	if s.config != nil {
		limit = s.config.Compact.ContextLimit
	}
	if s.sessionMgr != nil {
		if data, err := s.sessionMgr.LoadContext(id); err == nil {
			var st ctxinsight.State
			if json.Unmarshal(data, &st) == nil {
				s.recorder.Restore(st)
				s.recorder.SetLimit(limit)
				return
			}
		}
	}
	s.recorder.Reset()
	s.recorder.SetLimit(limit)
}
