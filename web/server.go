package web

import (
	"fmt"
	"io/fs"
	"net/http"
)

// Server 封装 HTTP Server
type Server struct {
	session *Session
	mux     *http.ServeMux
	port    int
}

// NewServer 创建 HTTP Server
func NewServer(sess *Session, port int) *Server {
	s := &Server{
		session: sess,
		mux:     http.NewServeMux(),
		port:    port,
	}
	s.registerRoutes()
	return s
}

// registerRoutes 注册所有路由
func (s *Server) registerRoutes() {
	// API 端点
	s.mux.HandleFunc("/api/events", s.handleSSE)
	s.mux.HandleFunc("/api/chat", s.handleChat)
	s.mux.HandleFunc("/api/clear", s.handleClear)
	s.mux.HandleFunc("/api/compact", s.handleCompact)
	s.mux.HandleFunc("/api/exit", s.handleExit)
	s.mux.HandleFunc("/api/permission", s.handlePermission)
	s.mux.HandleFunc("/api/status", s.handleStatus)

	// 静态文件（go:embed 的前端构建产物）
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		panic("failed to create sub FS: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(distFS))
	s.mux.Handle("/", fileServer)
}

// ListenAndServe 启动 HTTP 服务器
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf(":%d", s.port)
	handler := corsMiddleware(s.mux)
	return http.ListenAndServe(addr, handler)
}

// Addr 返回服务器监听地址（供日志和 openBrowser 使用）
func (s *Server) Addr() string {
	return fmt.Sprintf("http://localhost:%d", s.port)
}

// corsMiddleware 为开发环境添加 CORS 头
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
