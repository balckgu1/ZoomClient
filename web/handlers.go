// web/handlers.go
//
// HTTP 请求处理器：上行命令（chat/clear/compact/exit）、权限回复、状态查询、SSE 事件流。
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"zoomClient/fsm"
	"zoomClient/model"
	"zoomClient/permission"
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

// handleStop 处理 POST /api/stop —— 中断正在运行的 agentLoop
func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.session.CmdCh <- Command{Action: "stop"}
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
		"model":           s.session.Model,
		"turn_count":      s.session.State.TurnCount,
		"busy":            s.session.Busy.Load(),
		"session_id":      s.session.ID,
		"workdir":         s.session.WorkDir(),
		"permission_mode": s.permissionMgr.GetMode(),
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
		s.session.IsNew = true
		s.session.ExistingTitle = ""
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
		s.session.IsNew = false
		s.session.ExistingTitle = record.Title
		s.session.ExistingCreatedAt = record.CreatedAt
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

// handleModelByID 处理 DELETE / PUT /api/models/{name}
func (s *Server) handleModelByID(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/models/")
	// 如果路径正好是 /api/models/test，交给 handleModelTest 处理
	if name == "test" {
		s.handleModelTest(w, r)
		return
	}
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model name required"})
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := s.modelRegistry.Remove(name); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	case http.MethodPut:
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
		if req.ModelName == "" {
			req.ModelName = req.Name
		}
		if req.Type == "" {
			req.Type = "openai"
		}
		// Remove old preset and add updated one
		_ = s.modelRegistry.Remove(name)
		preset := &model.Preset{
			Name:      name,
			Type:      req.Type,
			BaseURL:   req.BaseURL,
			APIKey:    req.APIKey,
			ModelName: req.ModelName,
		}
		s.modelRegistry.Add(preset)
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleModelTest 处理 POST /api/models/test —— 测试模型连通性
func (s *Server) handleModelTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Type      string `json:"type"`
		BaseURL   string `json:"base_url"`
		APIKey    string `json:"api_key"`
		ModelName string `json:"model_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	preset := &model.Preset{
		Name:      "_test_",
		Type:      req.Type,
		BaseURL:   req.BaseURL,
		APIKey:    req.APIKey,
		ModelName: req.ModelName,
	}
	client, modelName := model.BuildClient(preset)

	// 发送一条最小消息测试连通性
	_, err := client.Chat(modelName, []fsm.Message{
		{Role: "user", Content: "hi"},
	}, nil, nil)
	if err != nil {
		msg := fmt.Sprintf("Connection failed: %v", err)
		// 对常见网络错误给出更友好的提示
		errStr := err.Error()
		if strings.Contains(errStr, "no such host") || strings.Contains(errStr, "dial tcp") && strings.Contains(errStr, "lookup") {
			msg += " | Hint: DNS resolution failed — the API endpoint may not be accessible from your network. Try using a VPN, proxy, or an alternative endpoint."
		} else if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
			msg += " | Hint: Request timed out — check your network connection or increase the timeout."
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "error",
			"message": msg,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Connection successful",
	})
}

// ─── 工作目录管理 ───

// handleWorkDir 处理 /api/workdir：GET 查询当前工作目录，POST 请求切换工作目录。
//
// 切换动作通过 CmdCh 投递给 REPL 循环串行执行：REPL 会校验目录有效性，
// 并同步更新 toolCtx.WorkPath 与系统提示词管道，最后经 SSE 反馈结果。
// 这样做可保证与 agentLoop 对工作目录的读取互斥，避免数据竞争。
func (s *Server) handleWorkDir(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"path": s.session.WorkDir()})

	case http.MethodPost:
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		path := strings.TrimSpace(req.Path)
		if path == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
			return
		}
		s.session.CmdCh <- Command{Action: "set_workdir", WorkDir: path}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ─── 权限配置管理 ───

// permissionConfigResponse 权限配置的对外 JSON 响应体。
type permissionConfigResponse struct {
	Mode        permission.Mode   `json:"mode"`
	Interactive bool              `json:"interactive"`
	DenyRules   []permission.Rule `json:"deny_rules"`
	AllowRules  []permission.Rule `json:"allow_rules"`
}

// handlePermissionConfig 处理 /api/permission/config：
// GET 返回当前权限配置快照，PUT 在运行时更新模式与 deny/allow 规则。
//
// permission.Manager 内部以读写锁保护，故此处可直接同步更新并立即返回最新快照，
// 无需经过 REPL 循环。
func (s *Server) handlePermissionConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.buildPermissionConfigResponse())

	case http.MethodPut:
		var req struct {
			Mode       string            `json:"mode"`
			DenyRules  []permission.Rule `json:"deny_rules"`
			AllowRules []permission.Rule `json:"allow_rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		// 模式为空时保持不变；非空时由 SetMode 校验（非法值回退到 default）
		if strings.TrimSpace(req.Mode) != "" {
			s.permissionMgr.SetMode(permission.Mode(strings.TrimSpace(req.Mode)))
		}
		// 规范化规则行为：deny 列表统一为 deny，allow 列表统一为 allow，
		// 与 Check 中"按列表归属决定行为"的语义保持一致
		deny := normalizeRules(req.DenyRules, permission.BehaviorDeny)
		allow := normalizeRules(req.AllowRules, permission.BehaviorAllow)
		s.permissionMgr.UpdateRules(deny, allow)

		writeJSON(w, http.StatusOK, s.buildPermissionConfigResponse())

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// buildPermissionConfigResponse 基于权限管理器快照与全局配置组装响应体。
func (s *Server) buildPermissionConfigResponse() permissionConfigResponse {
	snap := s.permissionMgr.Snapshot()
	interactive := false
	if s.config != nil {
		interactive = s.config.Permission.Interactive
	}
	return permissionConfigResponse{
		Mode:        snap.Mode,
		Interactive: interactive,
		DenyRules:   snap.DenyRules,
		AllowRules:  snap.AllowRules,
	}
}

// normalizeRules 拷贝规则列表并强制其行为字段与所属列表语义一致，
// 同时去除首尾空白，避免前端传入空值或错误的 behavior 影响判定。
func normalizeRules(rules []permission.Rule, behavior permission.Behavior) []permission.Rule {
	out := make([]permission.Rule, 0, len(rules))
	for _, r := range rules {
		out = append(out, permission.Rule{
			Tool:     strings.TrimSpace(r.Tool),
			Behavior: behavior,
			Path:     strings.TrimSpace(r.Path),
			Content:  strings.TrimSpace(r.Content),
		})
	}
	return out
}

// ─── 辅助函数 ───

// writeJSON 将数据序列化为 JSON 写入响应。
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
