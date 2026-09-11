package task

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"zoomClient/tools"

	"go.uber.org/zap"
)

// parseStringArray 将工具入参中的 JSON 数组安全转换为 []string
func parseStringArray(v interface{}) []string {
	raw, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// logTask 记录任务工具调用日志；logger 未注入时（如测试）静默跳过
func logTask(ctx *tools.ToolContext, action, id string) {
	if ctx != nil && ctx.Logger != nil {
		ctx.Logger.Info("task action",
			zap.String("session", ctx.SessionID),
			zap.String("action", action),
			zap.String("task_id", id),
		)
	}
}

// ── create_task ──

// CreateTaskTool 新建带依赖的持久化任务
type CreateTaskTool struct {
	mgr *TaskManager
}

func NewCreateTaskTool(mgr *TaskManager) *CreateTaskTool {
	return &CreateTaskTool{mgr: mgr}
}

func (t *CreateTaskTool) Name() string {
	return "create_task"
}

func (t *CreateTaskTool) Description() string {
	return "Create a new persistent task with optional blockedBy dependencies. " +
		"Tasks are stored on disk (.tasks/) and survive across sessions."
}

func (t *CreateTaskTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"subject":     map[string]interface{}{"type": "string", "description": "Short title of the task."},
			"description": map[string]interface{}{"type": "string", "description": "Optional detailed description."},
			"blockedBy": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Optional task IDs that must be completed before this task can start.",
			},
		},
		"required": []string{"subject"},
	}
}

func (t *CreateTaskTool) Call(args map[string]interface{}, ctx *tools.ToolContext) tools.ToolResult {
	subject, _ := args["subject"].(string)
	description, _ := args["description"].(string)
	blockedBy := parseStringArray(args["blockedBy"])

	task, err := t.mgr.CreateTask(subject, description, blockedBy)
	if err != nil {
		return tools.ToolResult{Ok: false, Content: "Error: " + err.Error(), IsError: true}
	}
	logTask(ctx, "create_task", task.ID)
	deps := ""
	if len(blockedBy) > 0 {
		deps = " (blockedBy: " + strings.Join(blockedBy, ", ") + ")"
	}
	return tools.ToolResult{Ok: true, Content: fmt.Sprintf("Created %s: %s%s", task.ID, task.Subject, deps)}
}

// ── list_tasks ──

// ListTasksTool 列出全部任务的状态、owner 与依赖
type ListTasksTool struct{ mgr *TaskManager }

func NewListTasksTool(mgr *TaskManager) *ListTasksTool { return &ListTasksTool{mgr: mgr} }

func (t *ListTasksTool) Name() string { return "list_tasks" }

func (t *ListTasksTool) Description() string {
	return "List all tasks with status, owner, and blockedBy dependencies."
}
func (t *ListTasksTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (t *ListTasksTool) Call(args map[string]interface{}, ctx *tools.ToolContext) tools.ToolResult {
	list, err := t.mgr.ListTasks()
	if err != nil {
		return tools.ToolResult{Ok: false, Content: "Error: " + err.Error(), IsError: true}
	}
	if len(list) == 0 {
		return tools.ToolResult{Ok: true, Content: "No tasks. Use create_task to add some."}
	}
	lines := make([]string, 0, len(list))
	for _, task := range list {
		line := fmt.Sprintf("%s %s: %s [%s]", statusIcon(task.Status), task.ID, task.Subject, task.Status)
		if task.Owner != "" {
			line += fmt.Sprintf(" [%s]", task.Owner)
		}
		if len(task.BlockedBy) > 0 {
			line += fmt.Sprintf(" (blockedBy: %s)", strings.Join(task.BlockedBy, ", "))
		}
		lines = append(lines, line)
	}
	return tools.ToolResult{Ok: true, Content: strings.Join(lines, "\n")}
}

// ── get_task ──

// GetTaskTool 返回指定任务的完整 JSON 细节（跨会话恢复时读取完整描述）
type GetTaskTool struct{ mgr *TaskManager }

func NewGetTaskTool(mgr *TaskManager) *GetTaskTool { return &GetTaskTool{mgr: mgr} }

func (t *GetTaskTool) Name() string { return "get_task" }

func (t *GetTaskTool) Description() string {
	return "Get full details (including description and dependencies) of a specific task by ID."
}

func (t *GetTaskTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task_id": map[string]interface{}{"type": "string", "description": "The ID of the task to read."},
		},
		"required": []string{"task_id"},
	}
}

func (t *GetTaskTool) Call(args map[string]interface{}, ctx *tools.ToolContext) tools.ToolResult {
	id, _ := args["task_id"].(string)
	task, err := t.mgr.GetTask(id)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.ToolResult{Ok: false, Content: fmt.Sprintf("Error: Task %s not found", id), IsError: true}
		}
		return tools.ToolResult{Ok: false, Content: "Error: " + err.Error(), IsError: true}
	}
	data, merr := json.MarshalIndent(task, "", "  ")
	if merr != nil {
		return tools.ToolResult{Ok: false, Content: "Error: " + merr.Error(), IsError: true}
	}
	return tools.ToolResult{Ok: true, Content: string(data)}
}

// ── claim_task ──

// ClaimTaskTool 认领一个 pending 任务：设置 owner，状态转为 in_progress
type ClaimTaskTool struct{ mgr *TaskManager }

func NewClaimTaskTool(mgr *TaskManager) *ClaimTaskTool { return &ClaimTaskTool{mgr: mgr} }

func (t *ClaimTaskTool) Name() string { return "claim_task" }

func (t *ClaimTaskTool) Description() string {
	return "Claim a pending task: sets owner and changes status to in_progress. " +
		"Fails if the task is already claimed/completed or its blockedBy dependencies are not completed."
}

func (t *ClaimTaskTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task_id": map[string]interface{}{"type": "string", "description": "The ID of the task to claim."},
			"owner":   map[string]interface{}{"type": "string", "description": "Optional owner name (multi-agent). Defaults to 'agent'."},
		},
		"required": []string{"task_id"},
	}
}

func (t *ClaimTaskTool) Call(args map[string]interface{}, ctx *tools.ToolContext) tools.ToolResult {
	id, _ := args["task_id"].(string)
	owner, _ := args["owner"].(string)
	msg, err := t.mgr.ClaimTask(id, owner)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.ToolResult{Ok: false, Content: fmt.Sprintf("Error: Task %s not found", id), IsError: true}
		}
		return tools.ToolResult{Ok: false, Content: "Error: " + err.Error(), IsError: true}
	}
	logTask(ctx, "claim_task", id)
	return tools.ToolResult{Ok: true, Content: msg}
}

// ── complete_task ──

// CompleteTaskTool 完成一个 in_progress 任务，并报告被解锁的下游任务
type CompleteTaskTool struct{ mgr *TaskManager }

func NewCompleteTaskTool(mgr *TaskManager) *CompleteTaskTool { return &CompleteTaskTool{mgr: mgr} }

func (t *CompleteTaskTool) Name() string { return "complete_task" }

func (t *CompleteTaskTool) Description() string {
	return "Complete an in-progress task and report which downstream tasks became unblocked."
}

func (t *CompleteTaskTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task_id": map[string]interface{}{"type": "string", "description": "The ID of the task to complete."},
		},
		"required": []string{"task_id"},
	}
}

func (t *CompleteTaskTool) Call(args map[string]interface{}, ctx *tools.ToolContext) tools.ToolResult {
	id, _ := args["task_id"].(string)
	msg, err := t.mgr.CompleteTask(id)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.ToolResult{Ok: false, Content: fmt.Sprintf("Error: Task %s not found", id), IsError: true}
		}
		return tools.ToolResult{Ok: false, Content: "Error: " + err.Error(), IsError: true}
	}
	logTask(ctx, "complete_task", id)
	return tools.ToolResult{Ok: true, Content: msg}
}
