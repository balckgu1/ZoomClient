// web/server.go
//
// HTTP Server 的组装：依赖注入、路由注册、CORS 中间件与生命周期管理。
package web

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"

	"zoomClient/ctxinsight"
	"zoomClient/model"
	"zoomClient/permission"
	"zoomClient/prompt"
	"zoomClient/session"
	"zoomClient/skills"
	"zoomClient/tools"
	"zoomClient/utils"
)

// ServerDeps 聚合 Web Server 运行所需的全部后端依赖。
type ServerDeps struct {
	Session       *Session                // Web 会话（命令/事件通道、权限交互、工作目录镜像）
	SessionMgr    *session.Manager        // 会话持久化管理器
	ModelRegistry *model.Registry         // 模型预设注册表
	SkillRegistry *skills.SkillRegistry   // skill 注册表（对外提供技能目录）
	ToolCtx       *tools.ToolContext      // 工具上下文（含工作目录 WorkPath）
	PermissionMgr *permission.Manager     // 权限管理器（模式 + deny/allow 规则）
	Pipeline      *prompt.MessagePipeline // 系统提示词组装管道（切换工作目录时热更新）
	Config        *utils.Config           // 全局配置（用于展示只读信息，如 interactive）
	Recorder      *ctxinsight.Recorder    // 上下文洞察折叠器（nil 时洞察端点返回空快照）
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
	recorder      *ctxinsight.Recorder
	mux           *http.ServeMux
	port          int
	httpServer    *http.Server
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
		recorder:      deps.Recorder,
		mux:           http.NewServeMux(),
		port:          port,
	}

	// 注册所有路由
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
	s.mux.HandleFunc("/api/question", s.handleQuestion)
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/context-usage", s.handleContextUsage)

	// ─── 上下文洞察端点（slim head 高频 / detail 按需）───
	s.mux.HandleFunc("/api/context/timeline", s.handleContextTimeline)
	s.mux.HandleFunc("/api/context/timeline/detail", s.handleContextTimelineDetail)

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

// corsMiddleware 只对本机来源放行跨域请求。
//
// 前端由同一个进程在 / 提供（生产走 go:embed，开发走 Vite 代理），本来就同源，
// 根本不需要 CORS 头。此前无条件返回 "*"，等于允许任意网页读取 /api/models、
// /api/permission/config 等接口；这里只在来源是本机时才回显，保留本地多端口开发
// 的便利（例如直连 8080 调试页面），同时让外部站点既读不到响应也发不出预检。
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && isLocalOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLocalOrigin 判断 Origin 是否指向本机（localhost / 127.0.0.1 / ::1，端口不限）。
func isLocalOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
