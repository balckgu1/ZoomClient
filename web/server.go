// web/server.go
//
// HTTP Server 的组装：依赖注入、路由注册、CORS 中间件与生命周期管理。
package web

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"

	"zoomClient/model"
	"zoomClient/permission"
	"zoomClient/prompt"
	"zoomClient/session"
	"zoomClient/skills"
	"zoomClient/tools"
	"zoomClient/utils"
)

// ServerDeps 聚合 Web Server 运行所需的全部后端依赖。
//
// 使用结构体而非一长串位置参数，便于后续扩展新能力（如新增管理器）时
// 只需增加字段，而不必改动所有调用方的参数顺序，提升可维护性。
type ServerDeps struct {
	Session       *Session                // Web 会话（命令/事件通道、权限交互、工作目录镜像）
	SessionMgr    *session.Manager        // 会话持久化管理器
	ModelRegistry *model.Registry         // 模型预设注册表
	SkillRegistry *skills.SkillRegistry   // skill 注册表（对外提供技能目录）
	ToolCtx       *tools.ToolContext      // 工具上下文（含工作目录 WorkPath）
	PermissionMgr *permission.Manager     // 权限管理器（模式 + deny/allow 规则）
	Pipeline      *prompt.MessagePipeline // 系统提示词组装管道（切换工作目录时热更新）
	Config        *utils.Config           // 全局配置（用于展示只读信息，如 interactive）
}

// Server 封装 HTTP Server
type Server struct {
	session       *Session
	sessionMgr    *session.Manager
	modelRegistry *model.Registry
	skillRegistry *skills.SkillRegistry
	toolCtx       *tools.ToolContext
	permissionMgr *permission.Manager
	pipeline      *prompt.MessagePipeline
	config        *utils.Config

	mux        *http.ServeMux
	port       int
	httpServer *http.Server
}

// NewServer 依据注入的依赖创建 HTTP Server
func NewServer(deps ServerDeps, port int) *Server {
	s := &Server{
		session:       deps.Session,
		sessionMgr:    deps.SessionMgr,
		modelRegistry: deps.ModelRegistry,
		skillRegistry: deps.SkillRegistry,
		toolCtx:       deps.ToolCtx,
		permissionMgr: deps.PermissionMgr,
		pipeline:      deps.Pipeline,
		config:        deps.Config,
		mux:           http.NewServeMux(),
		port:          port,
	}
	s.registerRoutes()
	addr := fmt.Sprintf(":%d", port)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: corsMiddleware(s.mux),
	}
	return s
}

// registerRoutes 注册所有路由
func (s *Server) registerRoutes() {
	// ─── 核心交互端点 ───
	s.mux.HandleFunc("/api/events", s.handleSSE)
	s.mux.HandleFunc("/api/chat", s.handleChat)
	s.mux.HandleFunc("/api/clear", s.handleClear)
	s.mux.HandleFunc("/api/compact", s.handleCompact)
	s.mux.HandleFunc("/api/exit", s.handleExit)
	s.mux.HandleFunc("/api/stop", s.handleStop)
	s.mux.HandleFunc("/api/permission", s.handlePermission)
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/context-usage", s.handleContextUsage)

	// ─── 会话管理端点 ───
	s.mux.HandleFunc("/api/sessions", s.handleSessions)
	s.mux.HandleFunc("/api/sessions/", s.handleSessionByID)

	// ─── 模型管理端点 ───
	s.mux.HandleFunc("/api/models", s.handleModels)
	s.mux.HandleFunc("/api/model/select", s.handleSelectModel)
	s.mux.HandleFunc("/api/models/", s.handleModelByID)

	// ─── 工作目录端点（GET 查询 / POST 切换）───
	s.mux.HandleFunc("/api/workdir", s.handleWorkDir)

	// ─── 技能目录端点（GET 列出全部已加载 skill，供输入框 "/" 扩展框使用）───
	s.mux.HandleFunc("/api/skills", s.handleSkills)

	// ─── 权限配置端点（GET 查询 / PUT 运行时更新模式与规则）───
	s.mux.HandleFunc("/api/permission/config", s.handlePermissionConfig)

	// ─── 静态文件（go:embed 的前端构建产物）───
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		panic("failed to create sub FS: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(distFS))
	s.mux.Handle("/", fileServer)
}

// ListenAndServe 启动 HTTP Server
func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown 关闭 HTTP Server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Addr 返回服务器监听地址（供日志和 openBrowser 使用）
func (s *Server) Addr() string {
	return fmt.Sprintf("http://localhost:%d", s.port)
}

// corsMiddleware 为开发环境添加 CORS 头
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
