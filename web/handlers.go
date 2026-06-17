// web/handlers.go
//
// HTTP 请求处理器：上行命令（chat/clear/compact/exit）、权限回复、状态查询、SSE 事件流。
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"zoomClient/model"
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

// ─── 会话管理 ───

// handleSessions 处理 /api/sessions 路由（GET 列表 / POST 新建）。
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		metas, err := s.sessionMgr.List()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, metas)

	case http.MethodPost:
		record := s.sessionMgr.CreateSession()
		s.session.RecordID = record.ID
		// Reset state for new session
		s.session.State.Messages = nil
		s.session.State.TurnCount = 0
		writeJSON(w, http.StatusCreated, record.ToMeta())

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSessionByID 处理 /api/sessions/{id} 路由（GET/DELETE/PATCH）。
func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	// 从路径中提取 id
	id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id required"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		record, err := s.sessionMgr.Load(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		// Switch backend state to loaded session
		s.session.RecordID = record.ID
		s.session.State.Messages = record.Messages
		s.session.State.TurnCount = record.TurnCount
		writeJSON(w, http.StatusOK, record)

	case http.MethodDelete:
		if err := s.sessionMgr.Delete(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		// 如果删除的是当前会话，更新 session.RecordID
		s.session.RecordID = s.sessionMgr.Current()
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	case http.MethodPatch:
		var req struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		if req.Title == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
			return
		}
		if err := s.sessionMgr.Rename(id, req.Title); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": id, "title": req.Title})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ─── 模型管理 ───

// handleModels 处理 /api/models 路由（GET 列表 / POST 新增）。
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		presets := s.modelRegistry.List()
		writeJSON(w, http.StatusOK, map[string]any{
			"models": presets,
			"active": s.modelRegistry.Active(),
		})
	case http.MethodPost:
		var req struct {
			Name      string `json:"name"`
			Type      string `json:"type"`
			BaseURL   string `json:"base_url"`
			APIKey    string `json:"api_key"`
			ModelName string `json:"model_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		if req.ModelName == "" {
			req.ModelName = req.Name
		}
		if req.Type == "" {
			req.Type = "openai"
		}
		preset := &model.Preset{
			Name:      req.Name,
			Type:      req.Type,
			BaseURL:   req.BaseURL,
			APIKey:    req.APIKey,
			ModelName: req.ModelName,
		}
		s.modelRegistry.Add(preset)
		writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSelectModel 处理 POST /api/model/select
func (s *Server) handleSelectModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	s.session.CmdCh <- Command{Action: "select_model", ModelName: req.Name}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// handleModelByID 处理 DELETE /api/models/{name}
func (s *Server) handleModelByID(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/models/")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model name required"})
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.modelRegistry.Remove(name); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─── 辅助函数 ───

// writeJSON 将数据序列化为 JSON 写入响应。
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
