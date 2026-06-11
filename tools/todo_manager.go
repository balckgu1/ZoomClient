package tools

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// 计划条目状态常量
const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
)

// validStatus 合法的计划状态集合
var validStatus = map[string]bool{
	StatusPending:    true,
	StatusInProgress: true,
	StatusCompleted:  true,
}

// PlanItem 表示计划中的一个步骤
type PlanItem struct {
	ID            string `json:"id"`            // 条目唯一标识
	Content       string `json:"content"`       // 这一步要做什么
	Status        string `json:"status"`        // 状态：pending | in_progress | completed
	ProgressLabel string `json:"progressLabel"` // 处于进行中时的自然语言描述（如"正在读取测试文件"）
}

// TodoManager 会话内计划管理器，同时实现 Tool 接口以便注册到工具注册表
type TodoManager struct {
	mu                sync.RWMutex
	planItems         []PlanItem // 当前计划条目列表
	roundsSinceUpdate int        // 连续多少轮模型没有更新该计划
}

// NewTodoManager 创建新的会话内计划管理器
func NewTodoManager() *TodoManager {
	return &TodoManager{
		planItems:         make([]PlanItem, 0),
		roundsSinceUpdate: 0,
	}
}

func (tm *TodoManager) Name() string {
	return "todo"
}

func (tm *TodoManager) Description() string {
	return "Manage the current session plan for multi-step work. Supports full replacement or merge-by-id. Keep exactly one step in_progress when a task has multiple steps."
}

func (tm *TodoManager) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"merge": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, merge items by id into existing plan; if false, replace the entire plan. Default false.",
			},
			"items": map[string]interface{}{
				"type":        "array",
				"description": "Plan items. When merge=false, replaces the entire plan; when merge=true, updates existing items by id and appends new ones.",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "Unique identifier for this plan item",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "What this step needs to do",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"enum":        []string{StatusPending, StatusInProgress, StatusCompleted},
							"description": fmt.Sprintf("Item status. One of %s / %s / %s.", StatusPending, StatusInProgress, StatusCompleted),
						},
						"progressLabel": map[string]interface{}{
							"type":        "string",
							"description": "Optional present-continuous label shown when step is in_progress.",
						},
					},
					"required": []string{"id", "content", "status"},
				},
			},
		},
		"required": []string{"items"},
	}
}

func (tm *TodoManager) Call(args map[string]interface{}, ctx *ToolContext) ToolResult {
	itemsRaw, exist := args["items"]
	if !exist {
		return ToolResult{Ok: false, Content: "Error: missing items parameter", IsError: true}
	}

	itemsSlice, ok := itemsRaw.([]interface{})
	if !ok {
		return ToolResult{Ok: false, Content: "Error: items parameter must be an array", IsError: true}
	}

	// 解析 merge 参数，默认 false
	merge, _ := args["merge"].(bool)

	// 将 []interface{} 转换为 []PlanItem
	planItems := make([]PlanItem, 0, len(itemsSlice))
	for i, itemRaw := range itemsSlice {
		itemMap, ok := itemRaw.(map[string]interface{})
		if !ok {
			return ToolResult{
				Ok:      false,
				Content: fmt.Sprintf("Error: items[%d] is not a valid object type", i),
				IsError: true,
			}
		}

		id, _ := itemMap["id"].(string)
		content, _ := itemMap["content"].(string)
		status, _ := itemMap["status"].(string)
		progressLabel, _ := itemMap["progressLabel"].(string)

		planItems = append(planItems, PlanItem{
			ID:            id,
			Content:       content,
			Status:        status,
			ProgressLabel: progressLabel,
		})
	}

	if ctx != nil && ctx.Logger != nil {
		ctx.Logger.Info("Updating plan", zap.String("session", ctx.SessionID), zap.Bool("merge", merge), zap.Any("planItems", planItems))
	}

	var renderedPlan string
	var err error
	if merge {
		renderedPlan, err = tm.Merge(planItems)
	} else {
		renderedPlan, err = tm.Update(planItems)
	}
	if err != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Plan update failed: %v", err), IsError: true}
	}

	return ToolResult{Ok: true, Content: renderedPlan}
}

// validateItems 校验计划条目列表的合法性，返回校验后的条目列表
// 校验规则：
//   - id 不能为空，不能重复
//   - content 不能为空
//   - status 必须是 pending / in_progress / completed 之一，空值默认为 pending
//   - 同一时间最多只能有一个 in_progress
func validateItems(items []PlanItem) ([]PlanItem, error) {
	validated := make([]PlanItem, 0, len(items))
	inProgressCount := 0
	seenIDs := make(map[string]bool, len(items))

	for index, item := range items {
		if item.ID == "" {
			return nil, fmt.Errorf("the %d item's id is empty", index+1)
		}
		if seenIDs[item.ID] {
			return nil, fmt.Errorf("duplicate id %q at item %d", item.ID, index+1)
		}
		seenIDs[item.ID] = true

		if item.Content == "" {
			return nil, fmt.Errorf("the %d item's content is empty", index+1)
		}

		status := item.Status
		if status == "" {
			return nil, fmt.Errorf("the %d item's status is empty", index+1)
		}
		if !validStatus[status] {
			return nil, fmt.Errorf("the %d item's status is illegal: %q", index+1, status)
		}
		if status == StatusInProgress {
			inProgressCount++
		}

		validated = append(validated, PlanItem{
			ID:            item.ID,
			Content:       item.Content,
			Status:        status,
			ProgressLabel: item.ProgressLabel,
		})
	}

	if inProgressCount > 1 {
		return nil, fmt.Errorf("there can be at most 1 item in progress")
	}

	return validated, nil
}

// Update 整体替换当前计划
func (tm *TodoManager) Update(items []PlanItem) (string, error) {
	validated, err := validateItems(items)
	if err != nil {
		return "", err
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.planItems = validated
	tm.roundsSinceUpdate = 0

	return tm.renderLocked(), nil
}

// Merge 按 ID 合并更新计划条目
func (tm *TodoManager) Merge(items []PlanItem) (string, error) {
	validated, err := validateItems(items)
	if err != nil {
		return "", err
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 在副本上操作
	merged := make([]PlanItem, len(tm.planItems))
	copy(merged, tm.planItems)

	idxMap := make(map[string]int, len(merged))
	for i, existing := range merged {
		idxMap[existing.ID] = i
	}

	for _, item := range validated {
		if idx, exists := idxMap[item.ID]; exists {
			merged[idx] = item
		} else {
			merged = append(merged, item)
			idxMap[item.ID] = len(merged) - 1
		}
	}

	// 校验通过前不修改真实状态
	inProgressCount := 0
	for _, item := range merged {
		if item.Status == StatusInProgress {
			inProgressCount++
		}
	}
	if inProgressCount > 1 {
		return "", fmt.Errorf("there can be at most 1 item in progress after merge")
	}

	tm.planItems = merged
	tm.roundsSinceUpdate = 0

	return tm.renderLocked(), nil
}

// Render 将当前计划渲染为可读文本
//   - pending 显示为 [ ]，in_progress 显示为 [>]，completed 显示为 [√]
func (tm *TodoManager) Render() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.renderLocked()

}

func (tm *TodoManager) renderLocked() string {
	if len(tm.planItems) == 0 {
		return "No plan items."
	}

	lines := make([]string, 0, len(tm.planItems))
	completedCount := 0

	for _, item := range tm.planItems {
		var marker string
		switch item.Status {
		case StatusPending:
			marker = "[ ]"
		case StatusInProgress:
			marker = "[>]"
		case StatusCompleted:
			marker = "[√]"
			completedCount++
		default:
			marker = "[ ]"
		}

		line := fmt.Sprintf("%s %s", marker, item.Content)
		if item.Status == StatusInProgress && item.ProgressLabel != "" {
			line += fmt.Sprintf(" (%s)", item.ProgressLabel)
		}
		lines = append(lines, line)
	}

	lines = append(lines, fmt.Sprintf("\n(%d/%d completed)", completedCount, len(tm.planItems)))
	return strings.Join(lines, "\n")
}

// Items 返回当前计划条目的副本
func (tm *TodoManager) Items() []PlanItem {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	result := make([]PlanItem, len(tm.planItems))
	copy(result, tm.planItems)
	return result
}

// IncrementRoundsSinceUpdate 增加未更新轮次计数，由主循环在每轮结束（未调用 todo）后调用
func (tm *TodoManager) IncrementRoundsSinceUpdate() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.roundsSinceUpdate++
}

// Reminder 返回提醒文本
//   - 当连续 TodoRoundsThreshold 轮未更新计划时触发，返回提醒文本以注入到下一轮对话；否则返回空字符串
func (tm *TodoManager) Reminder(todoRoundsThreshold int) string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.roundsSinceUpdate >= todoRoundsThreshold {
		return "<reminder>Your plan has not been updated for " + strconv.Itoa(todoRoundsThreshold) + " rounds. Refresh your current plan before continuing.</reminder>"
	}
	return ""
}
