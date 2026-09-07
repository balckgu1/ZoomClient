package main

// 本文件实现各输出模式（web / cli）的 REPL 运行循环：
// runWebREPL 驱动 web 服务与消息分发，runCLIREPL 驱动逐行 CLI 前端。

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"zoomClient/fsm"
	"zoomClient/logger"
	"zoomClient/session"
	"zoomClient/ui"
	"zoomClient/web"

	"go.uber.org/zap"
)

// runWebREPL starts the web server and runs the web mode REPL loop.
func runWebREPL(ctx context.Context, s *AgentSession, webSess *web.Session, webPort int, sessMgr *session.Manager) {
	log := logger.Log
	// 初始化工作目录镜像，使 GET /api/workdir 立即可返回当前目录
	webSess.SetWorkDir(s.ToolCtx.WorkPath)
	// 通过依赖注入创建 Web Server，聚合会话、模型、工具上下文、权限与提示词管道等后端能力
	webServer := web.NewServer(web.ServerDeps{
		Session:       webSess,
		SessionMgr:    sessMgr,
		ModelRegistry: s.ModelRegistry,
		ToolCtx:       s.ToolCtx,
		PermissionMgr: s.PermissionMgr,
		Pipeline:      s.Pipeline,
		Config:        s.Cfg,
	}, webPort)
	// run http server
	go func() {
		log.Info("Web server starting", zap.String("addr", webServer.Addr()))
		if err := webServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Web server error", zap.Error(err))
		}
	}()
	// 启动时推送一次上下文占用快照：前端首屏指示器与 GET /api/context-usage 缓存立即可用
	emitContextUsageWeb(s, s.Pipeline.BuildSystemPrompt(), s.Registry.GetAll())
	// Open browser after short delay
	go func() {
		time.Sleep(500 * time.Millisecond)
		url := webServer.Addr()
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", url)
		case "darwin":
			cmd = exec.Command("open", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}
		if err := cmd.Start(); err != nil {
			logger.Log.Warn("Failed to open browser", zap.String("url", url), zap.Error(err))
		}
	}()
	log.Info("Web UI available at", zap.String("url", webServer.Addr()))

	// Web REPL loop: read commands from CmdCh, with ctx cancellation support
	for {
		select {
		case <-ctx.Done():
			log.Info("Received interrupt signal, shutting down web server...")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := webServer.Shutdown(shutdownCtx); err != nil {
				log.Warn("Web server shutdown error", zap.Error(err))
			}
			cancel()
			return

		case cmd, ok := <-webSess.CmdCh:
			if !ok {
				log.Info("Web session CmdCh closed")
				return
			}
			switch cmd.Action {
			case "chat":
				webSess.Busy.Store(true)
				s.Em.EmitEmotion("thinking", nil)
				s.State.Messages = append(s.State.Messages, fsm.Message{Role: "user", Content: cmd.Message})
				// Create a fresh stop channel for this generation
				webSess.StopCh = make(chan struct{})
				agentLoop(s, webSess.StopCh)
				webSess.Busy.Store(false)

				// Save session after each turn
				title := webSess.ExistingTitle
				if title == "" || title == "NewSession" {
					title = "NewSession"
				}
				createdAt := time.Now()
				if !webSess.ExistingCreatedAt.IsZero() {
					createdAt = webSess.ExistingCreatedAt
				}
				record := &session.SessionRecord{
					ID:        webSess.RecordID,
					Title:     title,
					CreatedAt: createdAt,
					UpdatedAt: time.Now(),
					Model:     s.ModelName,
					WorkDir:   webSess.WorkDir(),
					TurnCount: s.State.TurnCount,
					Messages:  s.State.Messages,
				}
				if err := sessMgr.Save(record); err != nil {
					log.Warn("save session failed", zap.Error(err))
				}

				// Generate title only for new sessions (not loaded from disk)
				if webSess.IsNew {
					webSess.IsNew = false
					go func() {
						generatedTitle, err := sessMgr.GenerateTitle(record)
						if err != nil {
							log.Warn("generate title failed", zap.Error(err))
							return
						}
						if generatedTitle != "" {
							record.Title = generatedTitle
							webSess.ExistingTitle = generatedTitle
							if serr := sessMgr.Save(record); serr != nil {
								log.Warn("save title failed", zap.Error(serr))
							}
							// Push title update via SSE
							if sseEm, ok := s.Em.(*web.SseEmitter); ok {
								sseEm.EmitSessionRenamed(record.ID, generatedTitle)
							}
						}
					}()
				}

			case "select_model":
				if err := s.SwitchModel(cmd.ModelName); err != nil {
					s.Em.EmitError("model", fmt.Sprintf("switch failed: %s", err.Error()))
				} else {
					s.Em.EmitInfo(fmt.Sprintf("Switched to model %q (%s)", cmd.ModelName, s.ModelName))
					if webSess != nil {
						webSess.Model = s.ModelName
					}
				}

			case "stop":
				// Close the stop channel to interrupt a running agentLoop
				if webSess.StopCh != nil {
					select {
					case <-webSess.StopCh:
						// already closed
					default:
						close(webSess.StopCh)
					}
				}

			case "set_workdir":
				// 切换工作目录：在 REPL 循环内串行执行，与 agentLoop 对 WorkPath 的读取互斥，避免数据竞争
				absDir, absErr := filepath.Abs(cmd.WorkDir)
				if absErr != nil {
					s.Em.EmitError("workdir", fmt.Sprintf("invalid path %q: %s", cmd.WorkDir, absErr.Error()))
					break
				}
				info, statErr := os.Stat(absDir)
				if statErr != nil || !info.IsDir() {
					s.Em.EmitError("workdir", fmt.Sprintf("not a valid directory: %s", absDir))
					break
				}
				// 更新工具沙箱根目录，并热更新系统提示词中的工作目录
				s.ToolCtx.WorkPath = absDir
				if s.Pipeline != nil {
					s.Pipeline.UpdateWorkDir(absDir)
				}
				webSess.SetWorkDir(absDir)
				log.Info("Working directory switched", zap.String("workdir", absDir))
				s.Em.EmitInfo(fmt.Sprintf("Working directory switched to %s", absDir))
				// 推送专用事件，便于前端可靠地同步工作目录状态（而非解析提示文本）
				if sseEm, ok := s.Em.(*web.SseEmitter); ok {
					sseEm.EmitSystem("workdir_changed", map[string]string{"path": absDir})
				}

			default:
				if handleSessionCommand(cmd.Action, s) {
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					if err := webServer.Shutdown(shutdownCtx); err != nil {
						log.Warn("Web server shutdown error", zap.Error(err))
					}
					cancel()
					return
				}
			}
		}
	}
}

// runCLIREPL runs the CLI mode REPL loop, reading input from stdin.
func runCLIREPL(s *AgentSession, view *ui.Renderer) {
	defer view.Close()
	for {
		input, ok := view.PromptUser()
		if !ok {
			s.Em.EmitInfo("EOF, exiting...")
			break
		}
		if input == "" {
			continue
		}
		// Slash command handling
		if strings.HasPrefix(input, "/") {
			if handleSlashCommand(input, s) {
				break // /exit
			}
			continue
		}
		// Append user message and run agentLoop
		s.State.Messages = append(s.State.Messages, fsm.Message{Role: "user", Content: input})
		agentLoop(s, nil)

		// Save session after each turn
		if s.SessionMgr != nil {
			record := &session.SessionRecord{
				ID:        s.SessionRecordID,
				Title:     "NewSession",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Model:     s.ModelName,
				WorkDir:   s.ToolCtx.WorkPath,
				TurnCount: s.State.TurnCount,
				Messages:  s.State.Messages,
			}
			if err := s.SessionMgr.Save(record); err != nil {
				logger.Log.Warn("save session failed", zap.Error(err))
			}
			// Generate title for new sessions (async)
			if s.IsNewSession {
				s.IsNewSession = false
				go func() {
					generatedTitle, err := s.SessionMgr.GenerateTitle(record)
					if err != nil {
						logger.Log.Warn("generate title failed", zap.Error(err))
						return
					}
					if generatedTitle != "" {
						record.Title = generatedTitle
						if serr := s.SessionMgr.Save(record); serr != nil {
							logger.Log.Warn("save title failed", zap.Error(serr))
						}
						s.Em.EmitInfo(fmt.Sprintf("Session title: %s", generatedTitle))
					}
				}()
			}
		}

		s.Em.EmitTurnSeparator()
	}
}
