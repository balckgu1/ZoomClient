package tools

// 本文件实现后台任务管理器：慢速工具调用转入后台 goroutine 执行，
// 主循环不被阻塞；任务完成后以 <task_notification> 通知注入下一轮对话。

import (
	"fmt"
	"strings"
	"sync"
)

// 后台任务状态常量
const (
	BackgroundStatusRunning   = "running"   // 任务正在后台执行中
	BackgroundStatusCompleted = "completed" // 任务已完成，等待收集通知
)

// notificationSummaryLimit 通知中结果摘要的最大字符数，防止大输出撑爆上下文
const notificationSummaryLimit = 200

// BackgroundTask 表示一个后台任务及其生命周期信息
type BackgroundTask struct {
	ID         string     // 任务唯一标识，如 bg_0001
	ToolCallID string     // 触发该任务的原始工具调用 ID
	Command    string     // 任务执行的命令（用于通知展示）
	Status     string     // running / completed
	Result     ToolResult // 最终执行结果（完成前为零值）
}

// BackgroundTaskManager 管理后台任务的启动与通知收集。
// 使用互斥锁保护任务表，支持主循环与后台 goroutine 并发访问。
type BackgroundTaskManager struct {
	mu       sync.Mutex
	tasks    map[string]*BackgroundTask // bg_id → 任务状态
	seq      int                        // 自增序号，用于生成 bg_0001 形式的 ID
	registry *ToolRegister              // 执行后台工具所需的注册表
	ctx      *ToolContext               // 执行后台工具所需的上下文
}

// NewBackgroundTaskManager 创建后台任务管理器。
// registry 与 ctx 会被后台 goroutine 复用，调用方需保证其生命周期覆盖所有后台任务。
func NewBackgroundTaskManager(registry *ToolRegister, ctx *ToolContext) *BackgroundTaskManager {
	return &BackgroundTaskManager{
		tasks:    make(map[string]*BackgroundTask),
		registry: registry,
		ctx:      ctx,
	}
}

// Start 将工具调用转入后台 goroutine 执行，立即返回占位结果。
// 占位结果按原工具调用位置回填消息历史，保证 tool_use ↔ tool_result 一一配对不被破坏。
func (m *BackgroundTaskManager) Start(toolCall ToolCall) (bgID string, placeholder ToolResult) {
	m.mu.Lock()
	m.seq++
	bgID = fmt.Sprintf("bg_%04d", m.seq)
	m.tasks[bgID] = &BackgroundTask{
		ID:         bgID,
		ToolCallID: toolCall.ID,
		Command:    formatBackgroundCommand(toolCall),
		Status:     BackgroundStatusRunning,
	}
	m.mu.Unlock()

	// 后台 goroutine 执行工具，主循环立即返回继续处理其他任务
	go m.run(bgID, toolCall)

	return bgID, ToolResult{
		Ok:      true,
		Content: fmt.Sprintf("[Background task %s started] Result will be available when complete.", bgID),
	}
}

// run 在后台 goroutine 中执行工具，完成后更新任务状态。
// 通过注册表执行以复用权限判定等既有闸门；上下文修改器不做延迟应用，
// 后台任务的副作用按其完成时机直接生效。
func (m *BackgroundTaskManager) run(bgID string, toolCall ToolCall) {
	result := m.registry.RunTool(toolCall.Name, toolCall.Arguments, m.ctx)

	m.mu.Lock()
	defer m.mu.Unlock()
	if task, ok := m.tasks[bgID]; ok {
		task.Status = BackgroundStatusCompleted
		task.Result = result
	}
}

// CollectCompleted 收集所有已完成的后台任务，生成 <task_notification> 通知文本。
// 已收集的任务从管理器中移除，每条通知只注入一次。
func (m *BackgroundTaskManager) CollectCompleted() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	notifications := make([]string, 0)
	for id, task := range m.tasks {
		if task.Status != BackgroundStatusCompleted {
			continue
		}
		summary := task.Result.Content
		if len(summary) > notificationSummaryLimit {
			summary = summary[:notificationSummaryLimit] + "..."
		}
		notifications = append(notifications, fmt.Sprintf(
			"<task_notification>\n  <task_id>%s</task_id>\n  <status>completed</status>\n  <command>%s</command>\n  <summary>%s</summary>\n</task_notification>",
			task.ID, task.Command, summary))
		delete(m.tasks, id)
	}
	return notifications
}

// PendingCount 返回尚未完成的后台任务数量（含已完成但未收集的任务），用于日志与测试观测。
func (m *BackgroundTaskManager) PendingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.tasks)
}

// slowCommandKeywords 启发式判定慢命令的关键词。
// 模型未显式指定 run_in_background 时，用这些关键词兜底识别长耗时命令。
var slowCommandKeywords = []string{
	"install", "build", "test", "deploy", "compile",
	"docker build", "pip install", "npm install",
	"cargo build", "pytest", "make",
}

// ShouldRunBackground 判断工具调用是否应转入后台执行。
// 判定优先级：模型显式指定的 run_in_background 参数（true/false 均生效）> 慢命令关键词启发式兜底。
// 仅对 run_bash 工具生效，其余工具保持同步执行。
func ShouldRunBackground(toolCall ToolCall) bool {
	if toolCall.Name != "run_bash" {
		return false
	}
	// 模型显式指定了后台参数时，以其意图为准（显式 false 覆盖启发式）
	if flag, ok := toolCall.Arguments["run_in_background"].(bool); ok {
		return flag
	}
	// 模型未指定时，用慢命令关键词启发式兜底
	cmd, _ := toolCall.Arguments["command"].(string)
	lowered := strings.ToLower(cmd)
	for _, kw := range slowCommandKeywords {
		if strings.Contains(lowered, kw) {
			return true
		}
	}
	return false
}

// ExecuteWithBackground 按后台策略执行工具调用：
// 命中后台条件的调用转入 BackgroundTaskManager 异步执行并回填占位结果，
// 其余调用沿用原有并发批次策略同步执行。结果严格按原始顺序返回。
func ExecuteWithBackground(toolCalls []ToolCall, bgMgr *BackgroundTaskManager, registry *ToolRegister, toolCtx *ToolContext) []ToolResult {
	results := make([]ToolResult, len(toolCalls))
	syncCalls := make([]ToolCall, 0, len(toolCalls))
	syncIndex := make([]int, 0, len(toolCalls))

	for i, call := range toolCalls {
		if bgMgr != nil && ShouldRunBackground(call) {
			_, placeholder := bgMgr.Start(call)
			results[i] = placeholder
			continue
		}
		syncCalls = append(syncCalls, call)
		syncIndex = append(syncIndex, i)
	}

	if len(syncCalls) > 0 {
		syncResults := ExecuteToolCalls(syncCalls, registry, toolCtx)
		for j, r := range syncResults {
			results[syncIndex[j]] = r
		}
	}
	return results
}

// formatBackgroundCommand 提取后台任务的可读命令描述（通知展示用）。
// 命令缺失时退化为工具名，保证通知文本不为空。
func formatBackgroundCommand(toolCall ToolCall) string {
	if cmd, ok := toolCall.Arguments["command"].(string); ok && cmd != "" {
		return cmd
	}
	return toolCall.Name
}
