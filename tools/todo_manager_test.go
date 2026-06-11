package tools

import (
	"strings"
	"sync"
	"testing"
)

// TestUpdate 使用表驱动测试验证 Update（全量替换）的各种场景
func TestUpdate(t *testing.T) {
	tests := []struct {
		name        string
		items       []PlanItem
		wantErr     bool
		errContains string
		wantRender  []string // 渲染结果应包含的子字符串
	}{
		{
			name: "正常创建计划_含进行中步骤",
			items: []PlanItem{
				{ID: "s1", Content: "阅读失败测试", Status: StatusInProgress, ProgressLabel: "正在阅读"},
				{ID: "s2", Content: "定位 bug 根因", Status: StatusPending},
				{ID: "s3", Content: "修复代码", Status: StatusPending},
			},
			wantErr:    false,
			wantRender: []string{"[>] 阅读失败测试", "[ ] 定位 bug 根因", "(正在阅读)", "(0/3 completed)"},
		},
		{
			name: "全部完成",
			items: []PlanItem{
				{ID: "s1", Content: "任务A", Status: StatusCompleted},
				{ID: "s2", Content: "任务B", Status: StatusCompleted},
			},
			wantErr:    false,
			wantRender: []string{"[√] 任务A", "[√] 任务B", "(2/2 completed)"},
		},
		{
			name: "空状态默认为pending",
			items: []PlanItem{
				{ID: "s1", Content: "第一步", Status: ""},
				{ID: "s2", Content: "第二步", Status: ""},
			},
			wantErr:    false,
			wantRender: []string{"[ ] 第一步", "[ ] 第二步"},
		},
		{
			name:  "空列表合法_显示提示",
			items: []PlanItem{},

			wantErr:    false,
			wantRender: []string{"No plan"},
		},
		{
			name: "多个in_progress报错",
			items: []PlanItem{
				{ID: "s1", Content: "任务A", Status: StatusInProgress},
				{ID: "s2", Content: "任务B", Status: StatusInProgress},
			},
			wantErr:     true,
			errContains: "most 1 item",
		},
		{
			name: "非法状态值报错",
			items: []PlanItem{
				{ID: "s1", Content: "任务A", Status: "done"},
			},
			wantErr:     true,
			errContains: "illegal",
		},
		{
			name: "content为空报错",
			items: []PlanItem{
				{ID: "s1", Content: "", Status: StatusPending},
			},
			wantErr:     true,
			errContains: "content is empty",
		},
		{
			name: "id为空报错",
			items: []PlanItem{
				{ID: "", Content: "任务A", Status: StatusPending},
			},
			wantErr:     true,
			errContains: "id is empty",
		},
		{
			name: "重复id报错",
			items: []PlanItem{
				{ID: "dup", Content: "任务A", Status: StatusPending},
				{ID: "dup", Content: "任务B", Status: StatusPending},
			},
			wantErr:     true,
			errContains: "duplicate id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTodoManager()
			rendered, err := tm.Update(tt.items)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望返回错误，但未返回")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("错误信息应包含 %q，实际：%v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("不期望错误，但返回了：%v", err)
			}
			for _, want := range tt.wantRender {
				if !strings.Contains(rendered, want) {
					t.Errorf("渲染结果应包含 %q，实际：%s", want, rendered)
				}
			}
		})
	}
}

// TestMerge 使用表驱动测试验证 Merge（按ID合并）的各种场景
func TestMerge(t *testing.T) {
	tests := []struct {
		name         string
		initialItems []PlanItem // 初始计划
		mergeItems   []PlanItem // 合并传入
		wantErr      bool
		errContains  string
		wantRender   []string
		wantCount    int // 合并后条目数量
	}{
		{
			name: "更新已有条目状态",
			initialItems: []PlanItem{
				{ID: "s1", Content: "读文件", Status: StatusInProgress},
				{ID: "s2", Content: "写代码", Status: StatusPending},
			},
			mergeItems: []PlanItem{
				{ID: "s1", Content: "读文件", Status: StatusCompleted},
				{ID: "s2", Content: "写代码", Status: StatusInProgress},
			},
			wantErr:    false,
			wantRender: []string{"[√] 读文件", "[>] 写代码"},
			wantCount:  2,
		},
		{
			name: "追加新条目",
			initialItems: []PlanItem{
				{ID: "s1", Content: "任务A", Status: StatusCompleted},
			},
			mergeItems: []PlanItem{
				{ID: "s2", Content: "任务B", Status: StatusInProgress},
			},
			wantErr:    false,
			wantRender: []string{"[√] 任务A", "[>] 任务B"},
			wantCount:  2,
		},
		{
			name: "合并后多个in_progress报错",
			initialItems: []PlanItem{
				{ID: "s1", Content: "任务A", Status: StatusInProgress},
			},
			mergeItems: []PlanItem{
				{ID: "s2", Content: "任务B", Status: StatusInProgress},
			},
			wantErr:     true,
			errContains: "most 1 item in progress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTodoManager()
			// 设置初始计划
			if len(tt.initialItems) > 0 {
				_, err := tm.Update(tt.initialItems)
				if err != nil {
					t.Fatalf("初始化计划失败：%v", err)
				}
			}

			rendered, err := tm.Merge(tt.mergeItems)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望返回错误，但未返回")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("错误信息应包含 %q，实际：%v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("不期望错误，但返回了：%v", err)
			}
			for _, want := range tt.wantRender {
				if !strings.Contains(rendered, want) {
					t.Errorf("渲染结果应包含 %q，实际：%s", want, rendered)
				}
			}
			if tt.wantCount > 0 && len(tm.Items()) != tt.wantCount {
				t.Errorf("合并后条目数应为 %d，实际：%d", tt.wantCount, len(tm.Items()))
			}
		})
	}
}

// TestCall 使用表驱动测试验证 Call 方法（Tool 接口实现）
func TestCall(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]interface{}
		wantOk      bool
		wantContain string
	}{
		{
			name: "正常调用_全量替换",
			args: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":            "t1",
						"content":       "编写单元测试",
						"status":        "in_progress",
						"progressLabel": "正在编写单元测试",
					},
					map[string]interface{}{
						"id":      "t2",
						"content": "代码审查",
						"status":  "pending",
					},
				},
			},
			wantOk:      true,
			wantContain: "[>] 编写单元测试",
		},
		{
			name: "merge模式调用",
			args: map[string]interface{}{
				"merge": true,
				"items": []interface{}{
					map[string]interface{}{
						"id":      "t1",
						"content": "新增任务",
						"status":  "pending",
					},
				},
			},
			wantOk:      true,
			wantContain: "[ ] 新增任务",
		},
		{
			name:        "缺少items参数",
			args:        map[string]interface{}{},
			wantOk:      false,
			wantContain: "items",
		},
		{
			name:        "items类型错误",
			args:        map[string]interface{}{"items": "not an array"},
			wantOk:      false,
			wantContain: "array",
		},
		{
			name: "items元素类型错误",
			args: map[string]interface{}{
				"items": []interface{}{"not an object"},
			},
			wantOk:      false,
			wantContain: "not a valid object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTodoManager()
			result := tm.Call(tt.args, nil)

			if result.Ok != tt.wantOk {
				t.Fatalf("期望 Ok=%v，实际 Ok=%v，Content: %s", tt.wantOk, result.Ok, result.Content)
			}
			if tt.wantContain != "" && !strings.Contains(result.Content, tt.wantContain) {
				t.Errorf("返回内容应包含 %q，实际：%s", tt.wantContain, result.Content)
			}
		})
	}
}

// TestRender 验证渲染逻辑
func TestRender(t *testing.T) {
	tests := []struct {
		name       string
		items      []PlanItem
		wantRender []string
	}{
		{
			name:       "空计划",
			items:      nil,
			wantRender: []string{"No plan"},
		},
		{
			name: "三种状态混合",
			items: []PlanItem{
				{ID: "a", Content: "待办任务", Status: StatusPending},
				{ID: "b", Content: "进行中任务", Status: StatusInProgress},
				{ID: "c", Content: "已完成任务", Status: StatusCompleted},
			},
			wantRender: []string{"[ ] 待办任务", "[>] 进行中任务", "[√] 已完成任务", "(1/3 completed)"},
		},
		{
			name: "进行中步骤附加ProgressLabel",
			items: []PlanItem{
				{ID: "a", Content: "读取文件", Status: StatusInProgress, ProgressLabel: "正在读取文件"},
			},
			wantRender: []string{"[>] 读取文件 (正在读取文件)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTodoManager()
			if tt.items != nil {
				_, _ = tm.Update(tt.items)
			}
			rendered := tm.Render()
			for _, want := range tt.wantRender {
				if !strings.Contains(rendered, want) {
					t.Errorf("渲染结果应包含 %q，实际：%s", want, rendered)
				}
			}
		})
	}
}

// TestReminderAndRounds 验证轮次计数与提醒逻辑
func TestReminderAndRounds(t *testing.T) {
	tests := []struct {
		name       string
		increments int
		threshold  int
		wantEmpty  bool
	}{
		{name: "初始状态不提醒", increments: 0, threshold: 3, wantEmpty: true},
		{name: "2轮未更新不提醒", increments: 2, threshold: 3, wantEmpty: true},
		{name: "3轮未更新触发提醒", increments: 3, threshold: 3, wantEmpty: false},
		{name: "5轮未更新触发提醒", increments: 5, threshold: 3, wantEmpty: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTodoManager()
			for i := 0; i < tt.increments; i++ {
				tm.IncrementRoundsSinceUpdate()
			}

			reminder := tm.Reminder(tt.threshold)
			if tt.wantEmpty && reminder != "" {
				t.Errorf("不应触发提醒，实际：%s", reminder)
			}
			if !tt.wantEmpty {
				if reminder == "" {
					t.Error("应触发提醒，但为空")
				}
				if !strings.Contains(reminder, "<reminder>") {
					t.Errorf("提醒格式不正确：%s", reminder)
				}
			}
		})
	}

	// 额外验证：更新计划后计数归零
	t.Run("更新后计数归零", func(t *testing.T) {
		tm := NewTodoManager()
		tm.IncrementRoundsSinceUpdate()
		tm.IncrementRoundsSinceUpdate()
		tm.IncrementRoundsSinceUpdate()

		if tm.Reminder(3) == "" {
			t.Fatal("3轮未更新应触发提醒")
		}

		_, _ = tm.Update([]PlanItem{{ID: "x", Content: "新任务", Status: StatusInProgress}})
		if tm.Reminder(3) != "" {
			t.Error("更新计划后不应再触发提醒")
		}
	})
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

// TestConcurrency 验证并发安全性
func TestConcurrency(t *testing.T) {
	tm := NewTodoManager()
	var wg sync.WaitGroup

	// 并发写入
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			items := []PlanItem{
				{ID: "c1", Content: "并发任务", Status: StatusInProgress},
			}
			_, _ = tm.Update(items)
		}(i)
	}

	// 并发读取
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tm.Render()
			_ = tm.Items()
			tm.IncrementRoundsSinceUpdate()
			_ = tm.Reminder(3)
		}()
	}

	wg.Wait()

	// 只要没有 panic 或 data race 即通过
	items := tm.Items()
	if len(items) != 1 {
		t.Errorf("并发后条目数应为 1，实际：%d", len(items))
	}
}

// TestFullFlow 验证计划的创建、推进、完成全流程
func TestFullFlow(t *testing.T) {
	tm := NewTodoManager()

	// 第一轮：创建计划
	items := []PlanItem{
		{ID: "s1", Content: "阅读失败测试", Status: StatusInProgress, ProgressLabel: "正在阅读失败测试"},
		{ID: "s2", Content: "定位 bug 根因", Status: StatusPending},
		{ID: "s3", Content: "修复代码", Status: StatusPending},
		{ID: "s4", Content: "运行回归测试", Status: StatusPending},
	}
	rendered, err := tm.Update(items)
	if err != nil {
		t.Fatalf("首次更新计划失败：%v", err)
	}
	if !strings.Contains(rendered, "[>] 阅读失败测试") {
		t.Errorf("渲染结果应包含进行中标记，实际：%s", rendered)
	}
	if len(tm.Items()) != 4 {
		t.Errorf("计划条目数应为 4，实际：%d", len(tm.Items()))
	}

	// 第二轮：merge 推进——第一步完成，第二步开始
	mergeItems := []PlanItem{
		{ID: "s1", Content: "阅读失败测试", Status: StatusCompleted},
		{ID: "s2", Content: "定位 bug 根因", Status: StatusInProgress, ProgressLabel: "正在分析日志"},
	}
	rendered2, err := tm.Merge(mergeItems)
	if err != nil {
		t.Fatalf("merge 推进失败：%v", err)
	}
	if !strings.Contains(rendered2, "[√] 阅读失败测试") {
		t.Errorf("渲染结果应包含已完成标记，实际：%s", rendered2)
	}
	if !strings.Contains(rendered2, "[>] 定位 bug 根因") {
		t.Errorf("渲染结果应包含进行中标记，实际：%s", rendered2)
	}

	// 第三轮：全部完成
	items3 := []PlanItem{
		{ID: "s1", Content: "阅读失败测试", Status: StatusCompleted},
		{ID: "s2", Content: "定位 bug 根因", Status: StatusCompleted},
		{ID: "s3", Content: "修复代码", Status: StatusCompleted},
		{ID: "s4", Content: "运行回归测试", Status: StatusCompleted},
	}
	rendered3, err := tm.Update(items3)
	if err != nil {
		t.Fatalf("完成所有计划失败：%v", err)
	}
	if strings.Count(rendered3, "[√]") != 4 {
		t.Errorf("全部完成后应显示 4 个已完成标记，实际渲染：%s", rendered3)
	}
}
