package tools

import (
	"strings"
	"testing"
)

// TestUpdate_NormalScenario_FullFlow 验证计划的创建、更新、推进全流程
func TestUpdate_NormalScenario_FullFlow(t *testing.T) {
	tm := NewTodoManager()

	// 第一轮：创建计划，包含 pending 和一个 in_progress
	items := []PlanItem{
		{Content: "阅读失败测试", Status: "in_progress", ActivateForm: "正在阅读失败测试"},
		{Content: "定位 bug 根因", Status: "pending"},
		{Content: "修复代码", Status: "pending"},
		{Content: "运行回归测试", Status: "pending"},
	}

	rendered, err := tm.Update(items)
	if err != nil {
		t.Fatalf("首次更新计划失败：%v", err)
	}

	if !strings.Contains(rendered, "[>] 阅读失败测试") {
		t.Errorf("渲染结果应包含进行中标记，实际：%s", rendered)
	}
	if !strings.Contains(rendered, "[ ] 定位 bug 根因") {
		t.Errorf("渲染结果应包含待办标记，实际：%s", rendered)
	}
	if tm.PlanningState.RoundsSinceUpdate != 0 {
		t.Errorf("更新后 RoundsSinceUpdate 应为 0，实际：%d", tm.PlanningState.RoundsSinceUpdate)
	}
	if len(tm.PlanningState.PlanItems) != 4 {
		t.Errorf("计划条目数应为 4，实际：%d", len(tm.PlanningState.PlanItems))
	}

	// 第二轮：推进计划——第一步完成，第二步开始
	items2 := []PlanItem{
		{Content: "阅读失败测试", Status: "completed"},
		{Content: "定位 bug 根因", Status: "in_progress", ActivateForm: "正在定位 bug 根因"},
		{Content: "修复代码", Status: "pending"},
		{Content: "运行回归测试", Status: "pending"},
	}

	rendered2, err := tm.Update(items2)
	if err != nil {
		t.Fatalf("推进计划失败：%v", err)
	}

	if !strings.Contains(rendered2, "[√] 阅读失败测试") {
		t.Errorf("渲染结果应包含已完成标记，实际：%s", rendered2)
	}
	if !strings.Contains(rendered2, "[>] 定位 bug 根因") {
		t.Errorf("渲染结果应包含新的进行中标记，实际：%s", rendered2)
	}

	// 第三轮：全部完成
	items3 := []PlanItem{
		{Content: "阅读失败测试", Status: "completed"},
		{Content: "定位 bug 根因", Status: "completed"},
		{Content: "修复代码", Status: "completed"},
		{Content: "运行回归测试", Status: "completed"},
	}

	rendered3, err := tm.Update(items3)
	if err != nil {
		t.Fatalf("完成所有计划失败：%v", err)
	}

	xCount := strings.Count(rendered3, "[√]")
	if xCount != 4 {
		t.Errorf("全部完成后应显示 4 个已完成标记，实际：%d\n%s", xCount, rendered3)
	}
	t.Logf("\n测试结果: \n%s\n", rendered3)
}

// TestUpdate_EmptyStatus 验证未指定 status 时默认为 pending
func TestUpdate_EmptyStatus(t *testing.T) {
	tm := NewTodoManager()

	items := []PlanItem{
		{Content: "第一步", Status: ""},
		{Content: "第二步", Status: ""},
	}

	rendered, err := tm.Update(items)
	if err != nil {
		t.Fatalf("更新空状态计划失败：%v", err)
	}

	if strings.Count(rendered, "[ ]") != 2 {
		t.Errorf("未指定状态时应默认显示待办标记，实际：%s", rendered)
	}

	// 验证内部存储的状态为 pending
	if tm.PlanningState.PlanItems[0].Status != "pending" {
		t.Errorf("内部状态应为 pending，实际：%s", tm.PlanningState.PlanItems[0].Status)
	}
}

// TestUpdate_MutiInProgress 验证同时存在多个 in_progress 时报错（教学约束）
func TestUpdate_MutiInProgress(t *testing.T) {
	tm := NewTodoManager()

	items := []PlanItem{
		{Content: "任务A", Status: "in_progress"},
		{Content: "任务B", Status: "in_progress"},
		{Content: "任务C", Status: "pending"},
	}

	_, err := tm.Update(items)
	if err == nil {
		t.Fatal("同时存在两个 in_progress 应返回错误，但未返回")
	}
	if !strings.Contains(err.Error(), "most 1 item") {
		t.Errorf("错误信息应提示 in_progress 过多，实际：%v", err)
	}
}

// TestUpdate_IllegalStatus 验证非法状态值 status 白名单校验
func TestUpdate_IllegalStatus(t *testing.T) {
	tm := NewTodoManager()

	items := []PlanItem{
		{Content: "任务A", Status: "done"}, // 非法状态
	}

	_, err := tm.Update(items)
	if err == nil {
		t.Fatal("非法状态值应返回错误，但未返回")
	}
	if !strings.Contains(err.Error(), "illegal") {
		t.Errorf("错误信息应提示状态不合法，实际：%v", err)
	}
}

// TestUpdate_EmptyContent 验证 content 为空时报错
func TestUpdate_EmptyContent(t *testing.T) {
	tm := NewTodoManager()

	items := []PlanItem{
		{Content: "", Status: "pending"},
	}

	_, err := tm.Update(items)
	if err == nil {
		t.Fatal("content 为空应返回错误，但未返回")
	}
}

// TestUpdate_EmptyPlanItem 验证更新为空列表时正确处理
func TestUpdate_EmptyPlanItem(t *testing.T) {
	tm := NewTodoManager()

	// 先设置一个计划
	_, _ = tm.Update([]PlanItem{
		{Content: "旧任务", Status: "in_progress"},
	})

	// 再更新为空列表
	rendered, err := tm.Update([]PlanItem{})
	if err != nil {
		t.Fatalf("更新为空列表失败：%v", err)
	}

	if !strings.Contains(rendered, "No plan") {
		t.Errorf("空计划应显示提示，实际：%s", rendered)
	}
}

// TestRender_EmptyPlan 验证空状态渲染
func TestRender_EmptyPlan(t *testing.T) {
	tm := NewTodoManager()

	rendered := tm.Render()
	if !strings.Contains(rendered, "No plan") {
		t.Errorf("空计划 Render 应显示提示，实际：%s", rendered)
	}
}

// TestRender_MixedStatus 验证三种状态标记的渲染
func TestRender_MixedStatus(t *testing.T) {
	tm := NewTodoManager()
	tm.PlanningState.PlanItems = []PlanItem{
		{Content: "待办任务", Status: "pending"},
		{Content: "进行中任务", Status: "in_progress"},
		{Content: "已完成任务", Status: "completed"},
	}

	rendered := tm.Render()

	if !strings.Contains(rendered, "[ ] 待办任务") {
		t.Error("缺少待办标记")
	}
	if !strings.Contains(rendered, "[>] 进行中任务") {
		t.Error("缺少进行中标记")
	}
	if !strings.Contains(rendered, "[√] 已完成任务") {
		t.Error("缺少已完成标记")
	}
}

// TestRender_ActiveFormAppended 验证 in_progress 步骤的 ActivateForm 附加显示
func TestRender_ActiveFormAppended(t *testing.T) {
	tm := NewTodoManager()
	tm.PlanningState.PlanItems = []PlanItem{
		{Content: "读取文件", Status: "in_progress", ActivateForm: "正在读取文件"},
	}

	rendered := tm.Render()
	if !strings.Contains(rendered, "(正在读取文件)") {
		t.Errorf("in_progress 步骤应附加 ActivateForm，实际：%s", rendered)
	}
}

// TestRoundsSinceUpdate_Reminder 验证轮次计数与提醒逻辑
func TestRoundsSinceUpdate_Reminder(t *testing.T) {
	tm := NewTodoManager()

	// 初始状态不应提醒
	if tm.Reminder() != "" {
		t.Error("初始状态不应触发提醒")
	}

	// 经过 2 轮，仍不应提醒
	tm.IncrementRoundsSinceUpdate()
	tm.IncrementRoundsSinceUpdate()
	if tm.Reminder() != "" {
		t.Error("2 轮未更新不应触发提醒")
	}

	// 第 3 轮，应触发提醒
	tm.IncrementRoundsSinceUpdate()
	if tm.Reminder() == "" {
		t.Error("3 轮未更新应触发提醒")
	}
	reminder := tm.Reminder()
	if !strings.Contains(reminder, "Refresh your current plan before continuing") || !strings.Contains(reminder, "<reminder>") {
		t.Errorf("提醒文本格式不正确：%s", reminder)
	}

	// 更新计划后，计数归零
	_, _ = tm.Update([]PlanItem{{Content: "新任务", Status: "in_progress"}})
	if tm.Reminder() != "" {
		t.Error("更新计划后不应再触发提醒")
	}
	if tm.PlanningState.RoundsSinceUpdate != 0 {
		t.Errorf("更新计划后 RoundsSinceUpdate 应为 0，实际：%d", tm.PlanningState.RoundsSinceUpdate)
	}
}

// TestCall_ToolInterface 验证 Call 方法（Tool 接口实现）
func TestCall_ToolInterface(t *testing.T) {
	tm := NewTodoManager()

	// 模拟模型传来的 JSON 参数
	args := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{
				"content":      "编写单元测试",
				"status":       "in_progress",
				"activateForm": "正在编写单元测试",
			},
			map[string]interface{}{
				"content":      "代码审查",
				"status":       "pending",
				"activateForm": "",
			},
		},
	}

	result := tm.Call(args, nil)
	if !result.Ok {
		t.Fatalf("Call 应返回成功，实际错误：%s", result.Content)
	}
	if result.IsError {
		t.Fatal("Call 成功时 IsError 应为 false")
	}
	if !strings.Contains(result.Content, "[>] 编写单元测试") {
		t.Errorf("返回内容应包含计划渲染结果，实际：%s", result.Content)
	}
	if len(tm.PlanningState.PlanItems) != 2 {
		t.Errorf("计划条目数应为 2，实际：%d", len(tm.PlanningState.PlanItems))
	}
}

// TestCall_MissingItemsParam 验证缺少 items 参数时报错
func TestCall_MissingItemsParam(t *testing.T) {
	tm := NewTodoManager()

	result := tm.Call(map[string]interface{}{}, nil)
	if result.Ok {
		t.Fatal("缺少 items 参数应返回失败")
	}
	if !strings.Contains(result.Content, "items") {
		t.Errorf("错误信息应提到 items，实际：%s", result.Content)
	}
}

// TestCall_ItemsTypeError 验证 items 类型错误时报错
func TestCall_ItemsTypeError(t *testing.T) {
	tm := NewTodoManager()

	result := tm.Call(map[string]interface{}{"items": "not an array"}, nil)
	if result.Ok {
		t.Fatal("items 类型错误应返回失败")
	}
	if !strings.Contains(result.Content, "数组") {
		t.Errorf("错误信息应提示数组类型，实际：%s", result.Content)
	}
}

// TestToolInterfaceMethods 验证 Name / Description / Parameters 基本方法
func TestToolInterfaceMethods(t *testing.T) {
	tm := NewTodoManager()

	if tm.Name() != "todo" {
		t.Errorf("工具名称应为 todo，实际：%s", tm.Name())
	}

	if tm.Description() == "" {
		t.Error("工具描述不应为空")
	}

	params := tm.Parameters()
	if params == nil {
		t.Fatal("参数定义不应为空")
	}

	required, ok := params["required"].([]string)
	if !ok || len(required) == 0 || required[0] != "items" {
		t.Error("参数定义中 required 应包含 items")
	}
}
