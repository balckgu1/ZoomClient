// web/handlers.go
//
// HTTP 请求处理器：上行命令（chat/clear/compact/exit）、权限回复、状态查询、SSE 事件流。
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// handleSSE 建立 SSE 长连接，将 Session.EventCh 中的事件以 text/event-stream 格式推送。
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // nginx 兼容

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-s.session.EventCh:
			b, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

// ─── 上行命令 ───

type chatRequest struct {
	Message string `json:"message"`
}

// handleChat 处理 POST /api/chat
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is empty"})
		return
	}
	if s.session.Busy.Load() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "agent is busy"})
		return
	}
	s.session.CmdCh <- Command{Action: "chat", Message: req.Message}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// handleClear 处理 POST /api/clear
func (s *Server) handleClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.session.CmdCh <- Command{Action: "clear"}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// handleCompact 处理 POST /api/compact
func (s *Server) handleCompact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.session.CmdCh <- Command{Action: "compact"}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// handleExit 处理 POST /api/exit
func (s *Server) handleExit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.session.CmdCh <- Command{Action: "exit"}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// ─── 权限回复 ───

type permissionRequest struct {
	ID     string `json:"id"`
	Allow  bool   `json:"allow"`
	Reason string `json:"reason"`
}

// handlePermission 处理 POST /api/permission
func (s *Server) handlePermission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req permissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	s.session.ResolvePermission(req.ID, req.Allow, req.Reason)
	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

// ─── 状态查询 ───

// handleStatus 处理 GET /api/status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model":      s.session.Model,
		"turn_count": s.session.State.TurnCount,
		"busy":       s.session.Busy.Load(),
		"session_id": s.session.ID,
	})
}

// ─── 辅助函数 ───

// writeJSON 将数据序列化为 JSON 写入响应。
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
