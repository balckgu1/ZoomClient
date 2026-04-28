package tools

import (
	"fmt"
	"strings"
)

// PlanItem 表示计划中的一个步骤
type PlanItem struct {
	Content      string `json:"content"`      // 这一步要做什么
	Status       string `json:"status"`       // 这一步现在处在什么状态：pending | in_progress | completed
	ActivateForm string `json:"activateForm"` // 当它处于进行中时的自然语言描述（如"正在读取测试文件"）
}

// PlanningState 表示计划的运行状态
type PlanningState struct {
	PlanItems         []PlanItem // 当前计划条目列表
	RoundsSinceUpdate int        // 连续多少轮模型没有更新该计划
}

// TodoManager 会话内计划管理器，同时实现 Tool 接口以便注册到工具注册表
type TodoManager struct {
	PlanningState PlanningState
}

// validStatus 合法的计划状态集合
var validStatus = map[string]bool{
	"pending":     true,
	"in_progress": true,
	"completed":   true,
}

// NewTodoManager 创建新的会话内计划管理器
func NewTodoManager() *TodoManager {
	return &TodoManager{
		PlanningState: PlanningState{
			PlanItems:         make([]PlanItem, 0),
			RoundsSinceUpdate: 0,
		},
	}
}

// ===================== Tool 接口实现 =====================

// Name 返回工具名称，用于工具注册表和模型调用
func (tm *TodoManager) Name() string {
	return "todo"
}

// Description 返回工具的功能描述，模型据此判断何时使用该工具
func (tm *TodoManager) Description() string {
	return "Rewrite the current session plan for multi-step work. Keep exactly one step in_progress when a task has multiple steps. Refresh the plan as work advances."
}

// Parameters 返回工具的参数定义，遵循 JSON Schema 格式
func (tm *TodoManager) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"items": map[string]interface{}{
				"type":        "array",
				"description": "完整的计划条目列表，整体替换当前计划",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"content": map[string]interface{}{
							"type":        "string",
							"description": "这一步要做什么",
						},
						"status": map[string]interface{}{
							"type": "string",
							"enum": []string{"pending", "in_progress", "completed"},
						},
						"activateForm": map[string]interface{}{
							"type":        "string",
							"description": "Optional present-continuous label shown when step is in_progress.",
						},
					},
					"required": []string{"content", "status"},
				},
			},
		},
		"required": []string{"items"},
	}
}

// Call 实现 Tool 接口，解析模型传来的 args 并调用 Update
func (tm *TodoManager) Call(args map[string]interface{}, ctx *ToolContext) ToolResult {
	// 提取 items 参数
	itemsRaw, ok := args["items"]
	if !ok {
		return ToolResult{Ok: false, Content: "错误：缺少 items 参数", IsError: true}
	}

	itemsSlice, ok := itemsRaw.([]interface{})
	if !ok {
		return ToolResult{Ok: false, Content: "错误：items 参数必须是数组类型", IsError: true}
	}

	// 将 []interface{} 转换为 []PlanItem
	planItems := make([]PlanItem, 0, len(itemsSlice))
	for i, itemRaw := range itemsSlice {
		itemMap, ok := itemRaw.(map[string]interface{})
		if !ok {
			return ToolResult{
				Ok:      false,
				Content: fmt.Sprintf("错误：items[%d] 不是有效的对象类型", i),
				IsError: true,
			}
		}

		content, _ := itemMap["content"].(string)
		status, _ := itemMap["status"].(string)
		activateForm, _ := itemMap["activateForm"].(string)

		planItems = append(planItems, PlanItem{
			Content:      content,
			Status:       status,
			ActivateForm: activateForm,
		})
	}

	renderedPlan, err := tm.Update(planItems)
	if err != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("计划更新失败：%v", err), IsError: true}
	}

	return ToolResult{Ok: true, Content: renderedPlan}
}

// ===================== 核心计划管理方法 =====================

// Update 允许模型整体更新当前计划
// 接收模型传来的新计划条目列表，校验合法性后原子覆盖旧计划，返回渲染后的计划文本
// 校验规则：
//   - content 不能为空
//   - status 必须是 pending / in_progress / completed 之一，空值默认为 pending
//   - 同一时间最多只能有一个 in_progress（教学约束，强制模型聚焦当前一步）
func (tm *TodoManager) Update(items []PlanItem) (string, error) {
	validatedPlanItems := make([]PlanItem, 0, len(items))
	inProgressCount := 0

	for index, item := range items {
		if item.Content == "" {
			return "", fmt.Errorf("the %d item's content is empty", index+1)
		}

		status := item.Status
		if status == "" {
			status = "pending"
		}
		if !validStatus[status] {
			return "", fmt.Errorf("the %d item's status is illegal", index+1)
		}
		if status == "in_progress" {
			inProgressCount++
		}

		validatedPlanItems = append(validatedPlanItems, PlanItem{
			Content:      item.Content,
			Status:       status,
			ActivateForm: item.ActivateForm,
		})
	}

	// 教学约束：同一时间最多一个 in_progress，强制模型聚焦当前一步
	if inProgressCount > 1 {
		return "", fmt.Errorf("there can be at most 1 item in progress")
	}

	// 原子性覆盖旧计划，并重置未更新轮次计数
	tm.PlanningState.PlanItems = validatedPlanItems
	tm.PlanningState.RoundsSinceUpdate = 0

	return tm.Render(), nil
}

// Render 将当前计划渲染为可读文本
// pending 显示为 [ ]，in_progress 显示为 [>]，completed 显示为 [√]
// 当步骤处于 in_progress 且设置了 ActivateForm 时，附加进行时描述
func (tm *TodoManager) Render() string {
	if len(tm.PlanningState.PlanItems) == 0 {
		return "No plan items."
	}

	lines := make([]string, 0, len(tm.PlanningState.PlanItems))
	completedCount := 0

	for _, item := range tm.PlanningState.PlanItems {
		var marker string
		switch item.Status {
		case "pending":
			marker = "[ ]"
		case "in_progress":
			marker = "[>]"
		case "completed":
			marker = "[√]"
			completedCount++
		default:
			marker = "[ ]"
		}

		line := fmt.Sprintf("%s %s", marker, item.Content)
		// 进行中的步骤若有进行时描述，则附加展示
		if item.Status == "in_progress" && item.ActivateForm != "" {
			line += fmt.Sprintf(" (%s)", item.ActivateForm)
		}
		lines = append(lines, line)
	}

	// 在底部追加完成进度统计
	lines = append(lines, fmt.Sprintf("\n(%d/%d completed)", completedCount, len(tm.PlanningState.PlanItems)))
	return strings.Join(lines, "\n")
}

// ===================== 提醒机制 =====================

// IncrementRoundsSinceUpdate 增加未更新轮次计数，由主循环在每轮结束（未调用 todo）后调用
func (tm *TodoManager) IncrementRoundsSinceUpdate() {
	tm.PlanningState.RoundsSinceUpdate++
}

// Reminder 返回提醒文本
// 当连续 3 轮未更新计划时触发，返回提醒文本以注入到下一轮对话；否则返回空字符串
func (tm *TodoManager) Reminder() string {
	if tm.PlanningState.RoundsSinceUpdate >= 3 {
		return "<reminder>Your plan has not been updated for 3 rounds. Refresh your current plan before continuing.</reminder>"
	}
	return ""
}
