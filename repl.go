package main

// 本文件实现各输出模式（web / cli）的 REPL 运行循环：
// runWebREPL 驱动 web 服务与消息分发，runCLIREPL 驱动 bubbletea TUI。

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"zoomClient/fsm"
	"zoomClient/logger"
	"zoomClient/session"
	"zoomClient/ui"
	"zoomClient/web"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"
)

// runWebREPL starts the web server and runs the web mode REPL loop.
func runWebREPL(ctx context.Context, s *AgentSession, webSess *web.Session, webPort int, sessMgr *session.Manager) {
	log := logger.Log
	webServer := web.NewServer(webSess, sessMgr, s.ModelRegistry, webPort)
	// run http server
	go func() {
		log.Info("Web server starting", zap.String("addr", webServer.Addr()))
		if err := webServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Web server error", zap.Error(err))
		}
	}()
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

// runCLIREPL runs the CLI mode using bubbletea full-screen TUI.
func runCLIREPL(s *AgentSession) {
	// 使用较大的缓冲区，防止 agentLoop 密集输出工具调用时阻塞事件通道
	eventCh := make(chan ui.UIEvent, 512)

	// Create TuiEmitter that bridges agentLoop -> bubbletea
	tuiEmitter := ui.NewTuiEmitter(eventCh)
	s.Em = tuiEmitter

	// Replace permission asker with TUI-specific event-driven asker
	if s.PermissionMgr != nil {
		s.PermissionMgr.Asker = ui.NewTuiAsker(eventCh)
	}

	// Build bubbletea Model
	model := ui.NewTUIModel(eventCh, s)

	// Start bubbletea program (this takes over the terminal)
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		logger.Log.Fatal("TUI crashed", zap.Error(err))
	}
}
