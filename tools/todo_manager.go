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

// planItemPatch 表示一次工具调用中传入的单个条目补丁
type planItemPatch struct {
	ID            string  // id 必填，因此用值类型
	Content       *string // nil 表示未提供
	Status        *string // nil 表示未提供
	ProgressLabel *string // nil 表示未提供
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
	return "Manage the current session plan for multi-step work. " +
		"Use merge=false to replace the entire plan, or merge=true to partially update existing items by id. " +
		"Keep exactly one step in_progress when a task has multiple steps."
}

func (tm *TodoManager) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"merge": map[string]interface{}{
				"type": "boolean",
				"description": "If true, partially update existing items by id (only the fields you provide are changed) and append items with new ids. " +
					"If false, replace the entire plan (every item must include content and status). Default false.",
			},
			"items": map[string]interface{}{
				"type": "array",
				"description": "Plan items. When merge=false, this replaces the entire plan and each item MUST include content and status. " +
					"When merge=true, each item only needs id plus the fields you want to change; existing fields you omit are kept unchanged. " +
					"A brand-new id under merge=true is treated as a new item and MUST include content and status.",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "Unique identifier for this plan item.",
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
					"required": []string{"id"},
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
	merge := parseBool(args["merge"])

	// 将 []interface{} 解析为 []planItemPatch（保留字段是否提供的信息）
	patches := make([]planItemPatch, 0, len(itemsSlice))
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
		patches = append(patches, planItemPatch{
			ID:            id,
			Content:       strPtr(itemMap, "content"),
			Status:        strPtr(itemMap, "status"),
			ProgressLabel: strPtr(itemMap, "progressLabel"),
		})
	}

	if ctx != nil && ctx.Logger != nil {
		ctx.Logger.Info("Updating plan", zap.String("session", ctx.SessionID), zap.Bool("merge", merge), zap.Int("itemCount", len(patches)))
	}

	var renderedPlan string
	var err error
	if merge {
		renderedPlan, err = tm.Merge(patches)
	} else {
		renderedPlan, err = tm.Update(patches)
	}
	if err != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Plan update failed: %v", err), IsError: true}
	}

	return ToolResult{Ok: true, Content: renderedPlan, IsError: false}
}

// strPtr 从 itemMap 中取出字符串字段；若 key 不存在则返回 nil
func strPtr(itemMap map[string]interface{}, key string) *string {
	raw, ok := itemMap[key]
	if !ok {
		return nil
	}
	s, ok := raw.(string)
	if !ok {
		return nil
	}
	return &s
}

func parseBool(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		b, _ := strconv.ParseBool(x)
		return b
	}
	return false
}

// normalizeStatus 校验 status 并返回规范化后的值
func normalizeStatus(status string, idx int) (string, error) {
	if status == "" {
		return "", fmt.Errorf("items[%d]: status is empty", idx)
	}
	if !validStatus[status] {
		return "", fmt.Errorf("items[%d]: status is illegal: %q", idx, status)
	}
	return status, nil
}

// checkSingleInProgress 校验整份计划中最多只有一个 in_progress 条目
func checkSingleInProgress(items []PlanItem) error {
	count := 0
	for _, item := range items {
		if item.Status == StatusInProgress {
			count++
		}
	}
	if count > 1 {
		return fmt.Errorf("there can be at most 1 item in progress")
	}
	return nil
}

// buildFullItems 用于 replace 模式：要求每个 patch 都包含 content 与 status，构造完整条目列表
func buildFullItems(patches []planItemPatch) ([]PlanItem, error) {
	result := make([]PlanItem, 0, len(patches))
	seenIDs := make(map[string]bool, len(patches))
	for i, p := range patches {
		if p.ID == "" {
			return nil, fmt.Errorf("items[%d]: id is empty", i)
		}
		if seenIDs[p.ID] {
			return nil, fmt.Errorf("items[%d]: duplicate id %q", i, p.ID)
		}
		seenIDs[p.ID] = true
		if p.Content == nil || *p.Content == "" {
			return nil, fmt.Errorf("items[%d]: content is required when merge=false", i)
		}
		if p.Status == nil {
			return nil, fmt.Errorf("items[%d]: status is required when merge=false", i)
		}
		status, err := normalizeStatus(*p.Status, i)
		if err != nil {
			return nil, err
		}
		progressLabel := ""
		if p.ProgressLabel != nil {
			progressLabel = *p.ProgressLabel
		}
		result = append(result, PlanItem{
			ID:            p.ID,
			Content:       *p.Content,
			Status:        status,
			ProgressLabel: progressLabel,
		})
	}
	if err := checkSingleInProgress(result); err != nil {
		return nil, err
	}
	return result, nil
}

// Update 整体替换当前计划（replace 模式）
func (tm *TodoManager) Update(patches []planItemPatch) (string, error) {
	items, err := buildFullItems(patches)
	if err != nil {
		return "", err
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.planItems = items
	tm.roundsSinceUpdate = 0
	return tm.renderLocked(), nil
}

// Merge 按 id 进行字段级合并：
//   - 已存在 id：只覆盖本次提供的字段，未提供的字段保持不变
//   - 新 id：当作新建条目，必须提供 content 与 status
func (tm *TodoManager) Merge(patches []planItemPatch) (string, error) {
	// 先做与现有状态无关的基础校验（id 非空 / 批次内不重复 / status 合法）
	seenIDs := make(map[string]bool, len(patches))
	for i, p := range patches {
		if p.ID == "" {
			return "", fmt.Errorf("items[%d]: id is empty", i)
		}
		if seenIDs[p.ID] {
			return "", fmt.Errorf("items[%d]: duplicate id %q", i, p.ID)
		}
		seenIDs[p.ID] = true
		if p.Status != nil {
			if _, err := normalizeStatus(*p.Status, i); err != nil {
				return "", err
			}
		}
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	// 在副本上操作，校验通过后再原子替换
	merged := make([]PlanItem, len(tm.planItems))
	copy(merged, tm.planItems)
	idxMap := make(map[string]int, len(merged))
	for i, existing := range merged {
		idxMap[existing.ID] = i
	}
	for i, p := range patches {
		if idx, exists := idxMap[p.ID]; exists {
			// 已存在：字段级合并
			cur := merged[idx]
			if p.Content != nil {
				if *p.Content == "" {
					return "", fmt.Errorf("items[%d]: content cannot be set to empty", i)
				}
				cur.Content = *p.Content
			}
			if p.Status != nil {
				cur.Status = *p.Status // 已在前面校验过合法性
			}
			if p.ProgressLabel != nil {
				cur.ProgressLabel = *p.ProgressLabel
			}
			merged[idx] = cur
		} else {
			// 新增：必须提供 content 与 status
			if p.Content == nil || *p.Content == "" {
				return "", fmt.Errorf("items[%d]: content is required for new item %q", i, p.ID)
			}
			if p.Status == nil {
				return "", fmt.Errorf("items[%d]: status is required for new item %q", i, p.ID)
			}
			progressLabel := ""
			if p.ProgressLabel != nil {
				progressLabel = *p.ProgressLabel
			}
			newItem := PlanItem{
				ID:            p.ID,
				Content:       *p.Content,
				Status:        *p.Status,
				ProgressLabel: progressLabel,
			}
			merged = append(merged, newItem)
			idxMap[p.ID] = len(merged) - 1
		}
	}
	if err := checkSingleInProgress(merged); err != nil {
		return "", fmt.Errorf("%w (after merge)", err)
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

		line := fmt.Sprintf("%s [%s] %s", marker, item.ID, item.Content)
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
//   - 当连续 todoRoundsThreshold 轮未更新计划时触发，返回提醒文本以注入到下一轮对话；否则返回空字符串
func (tm *TodoManager) Reminder(todoRoundsThreshold int) string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.roundsSinceUpdate >= todoRoundsThreshold {
		return "<reminder>Your plan has not been updated for " + strconv.Itoa(tm.roundsSinceUpdate) + " rounds. Refresh your current plan before continuing.</reminder>"
	}
	return ""
}
