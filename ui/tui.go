// ui/tui.go
//
// bubbletea TUI 主入口：Model 定义、Init/Update/View、事件路由。
package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TUIModel 是 bubbletea 的核心 Model，持有所有子组件和运行时状态。
type TUIModel struct {
	conversation *ConversationView
	inputArea    *InputArea
	statusBar    *StatusBar
	slashOverlay *SlashOverlay
	helpOverlay  *HelpOverlay

	width, height int

	showLogo    bool
	logoShownAt time.Time

	eventCh      chan UIEvent
	agentSession AgentSessionBridge

	isAgentRunning bool
	quit           bool
}

// AgentSessionBridge 定义 TUI 需要从 AgentSession 调用的最小接口。
type AgentSessionBridge interface {
	RunAgentLoop(eventCh chan UIEvent, userMessage string)
	SlashCommand(input string) string // 返回 "exit" 表示退出
	GetModelName() string
	WorkDir() string
	LogPath() string
	TurnCount() int
	SessionTitle() string   // 当前会话标题
	TokenEstimate() int     // 累计对话 token 估算
}

// listenEvents 返回一个 tea.Cmd，在 goroutine 中监听 eventCh 并转发为 tea.Msg。
func listenEvents(ch chan UIEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return ev
	}
}

// NewTUIModel 创建一个新的 TUIModel。
func NewTUIModel(eventCh chan UIEvent, bridge AgentSessionBridge) TUIModel {
	return TUIModel{
		conversation: NewConversationView(),
		inputArea:    NewInputArea(),
		statusBar:    NewStatusBar(),
		slashOverlay: NewSlashOverlay(),
		helpOverlay:  NewHelpOverlay(),

		showLogo:     true,
		logoShownAt:  time.Now(),
		eventCh:      eventCh,
		agentSession: bridge,
	}
}

// logoDisplayDuration 启动 LOGO 显示时长。
const logoDisplayDuration = 1200 * time.Millisecond

// Init 是 bubbletea 的初始化 Cmd。
func (m TUIModel) Init() tea.Cmd {
	return tea.Batch(
		listenEvents(m.eventCh),
		m.inputArea.Init(),
		tea.Tick(logoDisplayDuration, func(t time.Time) tea.Msg {
			return logoTimeoutMsg{}
		}),
	)
}

// logoTimeoutMsg 是 LOGO 显示超时的信号。
type logoTimeoutMsg struct{}

// Update 是 bubbletea 的主事件路由。
func (m TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// 将消息转发给 conversation viewport（用于滚动等）
	convCmd := m.conversation.Update(msg)
	if convCmd != nil {
		cmds = append(cmds, convCmd)
	}

	// 普通 Enter 不转发给 textarea（由 handleKey 处理发送）
	// Shift+Enter 正常转发给 textarea（换行）
	// up/down/home/end 在无卡片焦点时不转发给 textarea（留给 viewport 滚动）
	forwardToTextarea := true
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			if !keyMsg.Alt {
				s := keyMsg.String()
				if s == "enter" {
					forwardToTextarea = false
				}
			}
		case tea.KeyUp, tea.KeyDown:
			// 无卡片焦点时留给 viewport 滚动，不发给 textarea
			if !m.conversation.HasFocusedCard() && !m.slashOverlay.Visible() {
				forwardToTextarea = false
			}
		}
	}
	if forwardToTextarea {
		cmd := m.inputArea.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		// 实时检测 slash 输入
		m.updateSlashOverlay()
	}

	switch msg := msg.(type) {

	case tea.KeyMsg:
		m, keyCmd := m.handleKey(msg)
		if keyCmd != nil {
			cmds = append(cmds, keyCmd)
		}
		return m, tea.Batch(cmds...)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, tea.Batch(cmds...)

	case UIEvent:
		m, uiCmd := m.handleUIEvent(msg)
		if uiCmd != nil {
			cmds = append(cmds, uiCmd)
		}
		return m, tea.Batch(cmds...)

	case logoTimeoutMsg:
		m.showLogo = false
		return m, tea.Batch(cmds...)

	}
	return m, tea.Batch(cmds...)
}

// View 渲染整个 TUI 界面。
func (m TUIModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	// 启动 LOGO 画面
	if m.showLogo {
		return LogoScreen(m.width)
	}

	// 动态布局计算：根据终端宽度自适应
	inputH := calcInputHeight(m.width)
	convH := m.height - inputH - 1 - 2 // -1 状态栏, -2 面板边框
	minConvH := 8
	if convH < minConvH {
		convH = minConvH
	}

	// 对话流（全宽），传递实际可用宽度
	convView := m.conversation.View(m.width, convH)
	convPanel := StylePanelBorder.Width(m.width - 2).Render(convView)

	// Slash 下拉浮层（仅在激活时渲染，浮于对话流上方）
	var slashView string
	if m.slashOverlay.Visible() {
		slashView = m.slashOverlay.View(m.width)
	}

	// 帮助浮层（覆盖在对话流上方）
	var helpView string
	if m.helpOverlay.Visible() {
		helpView = m.helpOverlay.View(m.width)
	}

	// 输入区
	inputView := m.inputArea.View(m.width, inputH)

	// 状态栏
	bar := m.statusBar.View(m.width, m.agentSession.GetModelName(),
		m.agentSession.TurnCount(), m.agentSession.SessionTitle(),
		m.agentSession.TokenEstimate(), m.agentSession.WorkDir(), m.agentSession.LogPath())

	return lipgloss.JoinVertical(lipgloss.Top, convPanel, slashView, helpView, inputView, bar)
}

// calcInputHeight 根据终端宽度计算输入区高度。
// 窄终端(≤60)给更多行数方便编辑，宽终端(≥120)可适度扩展。
func calcInputHeight(width int) int {
	switch {
	case width <= 60:
		return 7 // 窄屏：textarea 4行 + border 2 + hint 1
	case width <= 100:
		return 5 // 中屏：textarea 2行 + border 2 + hint 1
	default:
		return 6 // 宽屏：textarea 3行 + border 2 + hint 1
	}
}

// handleKey 处理键盘事件。
func (m TUIModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Slash overlay 按键拦截
	if m.slashOverlay.Visible() {
		switch key {
		case "up", "k":
			m.slashOverlay.Select(-1)
			return m, nil
		case "down", "j":
			m.slashOverlay.Select(1)
			return m, nil
		case "tab":
			selected := m.slashOverlay.GetSelected()
			m.slashOverlay.Hide()
			m.inputArea.SetValue(selected + " ")
			return m, nil
		case "esc":
			m.slashOverlay.Hide()
			return m, nil
		}
		// 其他按键正常传给 textarea
	}

	// Help overlay 按键拦截（Esc 关闭）
	if m.helpOverlay.Visible() {
		if key == "esc" {
			m.helpOverlay.Hide()
			return m, nil
		}
		// 帮助面板打开时，其他按键关闭面板并正常处理
		m.helpOverlay.Hide()
	}

	// 全局快捷键
	switch key {
	case "?", "ctrl+h":
		m.helpOverlay.Toggle()
		return m, nil
	case "ctrl+q":
		m.quit = true
		return m, tea.Quit
	case "ctrl+c":
		if m.isAgentRunning {
			m.isAgentRunning = false
			m.conversation.AppendInfo("Generation stopped by user")
			m.conversation.markNewContent()
			return m, listenEvents(m.eventCh)
		}
		return m, tea.Quit
	case "ctrl+l":
		m.conversation.Clear()
		return m, nil
	}

	// 滚动快捷键（agentLoop 运行中也允许）
	switch key {
	case "pgup", "ctrl+up":
		m.conversation.ScrollUp()
		return m, nil
	case "pgdown", "ctrl+down":
		m.conversation.ScrollDown()
		return m, nil
	case "home":
		m.conversation.ScrollToTop()
		return m, nil
	case "end":
		m.conversation.ScrollToBottom()
		return m, nil
	}

	// agentLoop 运行中，输入框不响应（但滚动仍然可用，上面已处理）
	if m.isAgentRunning {
		return m, nil
	}

	// 输入历史导航（↑↓），仅在无卡片焦点且无 slash 浮层时生效
	if key == "up" && !m.conversation.HasFocusedCard() {
		m.inputArea.HistoryUp()
		return m, nil
	}
	if key == "down" && !m.conversation.HasFocusedCard() {
		m.inputArea.HistoryDown()
		return m, nil
	}

	// 卡片导航（← → 切换焦点）
	switch key {
	case "left":
		m.conversation.FocusCard(-1)
		return m, nil
	case "right":
		m.conversation.FocusCard(1)
		return m, nil
	}

	// 输入框按键处理：Enter 发送（或切换卡片）
	if msg.Type == tea.KeyEnter && !msg.Alt {
		// 如果输入为空且有聚焦的卡片，Enter 切换折叠
		if m.inputArea.Value() == "" && m.conversation.HasFocusedCard() {
			m.conversation.ToggleCard()
			return m, nil
		}
		return m.submitInput()
	}

	return m, nil
}

// submitInput 提交用户输入。
func (m TUIModel) submitInput() (tea.Model, tea.Cmd) {
	text := m.inputArea.Value()
	if text == "" {
		return m, nil
	}

	// 关闭 slash overlay（如果有）
	m.slashOverlay.Hide()

	// 斜杠命令
	if strings.HasPrefix(text, "/") {
		result := m.agentSession.SlashCommand(text)
		if result == "exit" {
			return m, tea.Quit
		}
		m.inputArea.Reset()
		m.inputArea.ResetHistoryNav()
		return m, nil
	}

	// 保存到输入历史
	m.inputArea.AddHistory()

	// 启动 agentLoop
	m.isAgentRunning = true
	m.inputArea.SetThinking(true)
	m.inputArea.Reset()

	// 显示用户输入到对话流
	m.conversation.AppendUser(text)
	m.conversation.markNewContent()

	go m.agentSession.RunAgentLoop(m.eventCh, text)

	// 重新开始监听事件
	return m, listenEvents(m.eventCh)
}

// handleUIEvent 处理来自 agentLoop 的事件。
func (m TUIModel) handleUIEvent(ev UIEvent) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case EventSessionStart:
		if data, ok := ev.Data.([2]string); ok {
			m.conversation.AppendInfo("Model: " + data[0] + " | Logs: " + data[1])
		}

	case EventAssistant:
		if text, ok := ev.Data.(string); ok {
			m.conversation.AppendAssistant(text)
			m.conversation.CollapseAllCards()
			m.conversation.markNewContent()
		}

	case EventReasoning:
		if text, ok := ev.Data.(string); ok {
			m.conversation.AppendReasoningCard(text)
			m.conversation.markNewContent()
		}

	case EventToolCall:
		if data, ok := ev.Data.(ToolCallData); ok {
			m.conversation.AppendToolCard(data)
			m.conversation.markNewContent()
		}

	case EventToolResult:
		if data, ok := ev.Data.(ToolResultData); ok {
			m.conversation.CompleteToolCard(data)
			m.conversation.markNewContent()
		}

	case EventSubAgent:
		if text, ok := ev.Data.(string); ok {
			m.conversation.AppendSubAgent(text)
			m.conversation.markNewContent()
		}

	case EventHookBlocked:
		if data, ok := ev.Data.([2]string); ok {
			m.conversation.AppendHookBlocked(data[0], data[1])
			m.conversation.markNewContent()
		}

	case EventTodoPanel:
		if text, ok := ev.Data.(string); ok {
			m.conversation.AppendTodo(text)
			m.conversation.markNewContent()
		}

	case EventCompact:
		if data, ok := ev.Data.([2]int); ok {
			m.conversation.AppendCompact(data[0], data[1])
			m.conversation.markNewContent()
		}

	case EventError:
		if data, ok := ev.Data.([2]string); ok {
			m.conversation.AppendError(data[0], data[1])
			m.conversation.markNewContent()
		}

	case EventInfo:
		if text, ok := ev.Data.(string); ok {
			m.conversation.AppendInfo(text)
			m.conversation.markNewContent()
		}

	case EventTurnSeparator:
		m.conversation.AppendSeparator()

	case EventSessionEnd:
		// 会话结束，不做特殊渲染

	case EventAgentDone:
		m.isAgentRunning = false
		m.inputArea.SetThinking(false)
		// 恢复 slash 检测
		m.updateSlashOverlay()
	}

	// 继续监听事件
	return m, listenEvents(m.eventCh)
}

// updateSlashOverlay 根据当前输入更新 slash overlay 状态。
func (m *TUIModel) updateSlashOverlay() {
	raw := m.inputArea.RawValue()
	if strings.HasPrefix(raw, "/") && !strings.Contains(raw, " ") {
		m.slashOverlay.Show()
		m.slashOverlay.UpdateFilter(raw)
	} else {
		m.slashOverlay.Hide()
	}
}
