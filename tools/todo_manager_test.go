package tools

import (
	"strings"
	"testing"
)

// ---------- 测试辅助 ----------

// ptr 返回字符串指针，便于构造 planItemPatch
func ptr(s string) *string {
	return &s
}

// itemByID 在切片中按 id 查找条目，找不到返回 nil
func itemByID(items []PlanItem, id string) *PlanItem {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

// ---------- Update（replace 模式）----------

func TestUpdate(t *testing.T) {
	tests := []struct {
		name      string
		patches   []planItemPatch
		wantErr   bool
		errSubstr string                              // 期望错误信息包含的子串
		checkFn   func(t *testing.T, tm *TodoManager) // 成功后对状态的断言
	}{
		{
			name: "valid replace with multiple items",
			patches: []planItemPatch{
				{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusPending)},
				{ID: "s2", Content: ptr("step 2"), Status: ptr(StatusInProgress)},
				{ID: "s3", Content: ptr("step 3"), Status: ptr(StatusCompleted)},
			},
			wantErr: false,
			checkFn: func(t *testing.T, tm *TodoManager) {
				items := tm.Items()
				if len(items) != 3 {
					t.Fatalf("expected 3 items, got %d", len(items))
				}
				if items[1].Status != StatusInProgress {
					t.Errorf("expected s2 in_progress, got %q", items[1].Status)
				}
			},
		},
		{
			name:    "empty items clears the plan",
			patches: []planItemPatch{},
			wantErr: false,
			checkFn: func(t *testing.T, tm *TodoManager) {
				if len(tm.Items()) != 0 {
					t.Errorf("expected plan cleared, got %d items", len(tm.Items()))
				}
			},
		},
		{
			name: "missing content fails",
			patches: []planItemPatch{
				{ID: "s1", Status: ptr(StatusPending)},
			},
			wantErr:   true,
			errSubstr: "content is required",
		},
		{
			name: "empty content fails",
			patches: []planItemPatch{
				{ID: "s1", Content: ptr(""), Status: ptr(StatusPending)},
			},
			wantErr:   true,
			errSubstr: "content is required",
		},
		{
			name: "missing status fails",
			patches: []planItemPatch{
				{ID: "s1", Content: ptr("step 1")},
			},
			wantErr:   true,
			errSubstr: "status is required",
		},
		{
			name: "empty id fails",
			patches: []planItemPatch{
				{ID: "", Content: ptr("step 1"), Status: ptr(StatusPending)},
			},
			wantErr:   true,
			errSubstr: "id is empty",
		},
		{
			name: "duplicate id fails",
			patches: []planItemPatch{
				{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusPending)},
				{ID: "s1", Content: ptr("dup"), Status: ptr(StatusPending)},
			},
			wantErr:   true,
			errSubstr: "duplicate id",
		},
		{
			name: "illegal status fails",
			patches: []planItemPatch{
				{ID: "s1", Content: ptr("step 1"), Status: ptr("done")},
			},
			wantErr:   true,
			errSubstr: "status is illegal",
		},
		{
			name: "more than one in_progress fails",
			patches: []planItemPatch{
				{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusInProgress)},
				{ID: "s2", Content: ptr("step 2"), Status: ptr(StatusInProgress)},
			},
			wantErr:   true,
			errSubstr: "at most 1 item in progress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTodoManager()
			_, err := tm.Update(tt.patches)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkFn != nil {
				tt.checkFn(t, tm)
			}
		})
	}
}

// TestUpdateFailureDoesNotMutate 验证 Update 校验失败时不污染已有状态
func TestUpdateFailureDoesNotMutate(t *testing.T) {
	tm := NewTodoManager()
	if _, err := tm.Update([]planItemPatch{
		{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusInProgress)},
	}); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// 非法更新（缺 content）应失败且不改变原状态
	_, err := tm.Update([]planItemPatch{
		{ID: "s2", Status: ptr(StatusPending)},
	})
	if err == nil {
		t.Fatalf("expected error")
	}

	items := tm.Items()
	if len(items) != 1 || items[0].ID != "s1" {
		t.Errorf("plan was mutated on failed update: %+v", items)
	}
}

// ---------- Merge（字段级合并模式）----------

func TestMerge(t *testing.T) {
	tests := []struct {
		name      string
		initial   []planItemPatch // 初始计划（通过 Update 构造）
		patches   []planItemPatch // 本次 merge 的补丁
		wantErr   bool
		errSubstr string
		checkFn   func(t *testing.T, tm *TodoManager)
	}{
		{
			name: "partial update only status keeps content",
			initial: []planItemPatch{
				{ID: "s1", Content: ptr("write tests"), Status: ptr(StatusPending)},
			},
			patches: []planItemPatch{
				{ID: "s1", Status: ptr(StatusInProgress)},
			},
			wantErr: false,
			checkFn: func(t *testing.T, tm *TodoManager) {
				it := itemByID(tm.Items(), "s1")
				if it == nil {
					t.Fatal("s1 missing")
				}
				if it.Content != "write tests" {
					t.Errorf("content changed unexpectedly: %q", it.Content)
				}
				if it.Status != StatusInProgress {
					t.Errorf("status not updated: %q", it.Status)
				}
			},
		},
		{
			name: "merge appends new item",
			initial: []planItemPatch{
				{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusPending)},
			},
			patches: []planItemPatch{
				{ID: "s2", Content: ptr("step 2"), Status: ptr(StatusPending)},
			},
			wantErr: false,
			checkFn: func(t *testing.T, tm *TodoManager) {
				if len(tm.Items()) != 2 {
					t.Fatalf("expected 2 items, got %d", len(tm.Items()))
				}
				if itemByID(tm.Items(), "s2") == nil {
					t.Error("new item s2 not appended")
				}
			},
		},
		{
			name: "merge new item without content fails",
			initial: []planItemPatch{
				{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusPending)},
			},
			patches: []planItemPatch{
				{ID: "s2", Status: ptr(StatusPending)},
			},
			wantErr:   true,
			errSubstr: "content is required for new item",
		},
		{
			name: "merge new item without status fails",
			initial: []planItemPatch{
				{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusPending)},
			},
			patches: []planItemPatch{
				{ID: "s2", Content: ptr("step 2")},
			},
			wantErr:   true,
			errSubstr: "status is required for new item",
		},
		{
			name: "merge cannot set existing content to empty",
			initial: []planItemPatch{
				{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusPending)},
			},
			patches: []planItemPatch{
				{ID: "s1", Content: ptr("")},
			},
			wantErr:   true,
			errSubstr: "content cannot be set to empty",
		},
		{
			name: "merge updates progressLabel and can clear it",
			initial: []planItemPatch{
				{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusInProgress), ProgressLabel: ptr("reading files")},
			},
			patches: []planItemPatch{
				{ID: "s1", ProgressLabel: ptr("")}, // 显式清空
			},
			wantErr: false,
			checkFn: func(t *testing.T, tm *TodoManager) {
				it := itemByID(tm.Items(), "s1")
				if it.ProgressLabel != "" {
					t.Errorf("expected progressLabel cleared, got %q", it.ProgressLabel)
				}
				// content/status 应保持不变
				if it.Content != "step 1" || it.Status != StatusInProgress {
					t.Errorf("other fields changed: %+v", it)
				}
			},
		},
		{
			name: "merge resulting in two in_progress fails",
			initial: []planItemPatch{
				{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusInProgress)},
				{ID: "s2", Content: ptr("step 2"), Status: ptr(StatusPending)},
			},
			patches: []planItemPatch{
				{ID: "s2", Status: ptr(StatusInProgress)},
			},
			wantErr:   true,
			errSubstr: "at most 1 item in progress",
		},
		{
			name: "merge with illegal status fails",
			initial: []planItemPatch{
				{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusPending)},
			},
			patches: []planItemPatch{
				{ID: "s1", Status: ptr("invalid")},
			},
			wantErr:   true,
			errSubstr: "status is illegal",
		},
		{
			name: "merge duplicate id in batch fails",
			initial: []planItemPatch{
				{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusPending)},
			},
			patches: []planItemPatch{
				{ID: "s1", Status: ptr(StatusInProgress)},
				{ID: "s1", Status: ptr(StatusCompleted)},
			},
			wantErr:   true,
			errSubstr: "duplicate id",
		},
		{
			name: "merge empty id fails",
			initial: []planItemPatch{
				{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusPending)},
			},
			patches: []planItemPatch{
				{ID: "", Status: ptr(StatusInProgress)},
			},
			wantErr:   true,
			errSubstr: "id is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTodoManager()
			if len(tt.initial) > 0 {
				if _, err := tm.Update(tt.initial); err != nil {
					t.Fatalf("setup failed: %v", err)
				}
			}

			_, err := tm.Merge(tt.patches)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkFn != nil {
				tt.checkFn(t, tm)
			}
		})
	}
}

// TestMergeFailureDoesNotMutate 验证 Merge 校验失败时不污染已有状态
func TestMergeFailureDoesNotMutate(t *testing.T) {
	tm := NewTodoManager()
	if _, err := tm.Update([]planItemPatch{
		{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusInProgress)},
		{ID: "s2", Content: ptr("step 2"), Status: ptr(StatusPending)},
	}); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// 这次 merge 会导致两个 in_progress，应失败
	_, err := tm.Merge([]planItemPatch{
		{ID: "s2", Status: ptr(StatusInProgress)},
	})
	if err == nil {
		t.Fatalf("expected error")
	}

	// s2 应保持 pending，未被污染
	it := itemByID(tm.Items(), "s2")
	if it == nil || it.Status != StatusPending {
		t.Errorf("plan was mutated on failed merge: %+v", tm.Items())
	}
}

// ---------- Render ----------

func TestRender(t *testing.T) {
	tests := []struct {
		name        string
		patches     []planItemPatch
		wantContain []string
		wantEqual   string // 非空时做完整相等断言
	}{
		{
			name:      "empty plan",
			patches:   []planItemPatch{},
			wantEqual: "No plan items.",
		},
		{
			name: "markers and progress label",
			patches: []planItemPatch{
				{ID: "s1", Content: ptr("done step"), Status: ptr(StatusCompleted)},
				{ID: "s2", Content: ptr("doing step"), Status: ptr(StatusInProgress), ProgressLabel: ptr("running")},
				{ID: "s3", Content: ptr("todo step"), Status: ptr(StatusPending)},
			},
			wantContain: []string{
				"[√] [s1] done step",
				"[>] [s2] doing step (running)",
				"[ ] [s3] todo step",
				"(1/3 completed)",
			},
		},
		{
			name: "progress label not shown when not in_progress",
			patches: []planItemPatch{
				// pending 状态即使带 label 也不应渲染
				{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusPending), ProgressLabel: ptr("should not show")},
			},
			wantContain: []string{"[ ] [s1] step 1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTodoManager()
			if _, err := tm.Update(tt.patches); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			got := tm.Render()

			if tt.wantEqual != "" {
				if got != tt.wantEqual {
					t.Errorf("Render() = %q, want %q", got, tt.wantEqual)
				}
				return
			}
			for _, sub := range tt.wantContain {
				if !strings.Contains(got, sub) {
					t.Errorf("Render() output missing %q\nfull output:\n%s", sub, got)
				}
			}
			// progressLabel 不应在非 in_progress 行出现
			if tt.name == "progress label not shown when not in_progress" &&
				strings.Contains(got, "should not show") {
				t.Errorf("progressLabel leaked into non-in_progress render:\n%s", got)
			}
		})
	}
}

// ---------- Reminder & rounds ----------

func TestReminder(t *testing.T) {
	tests := []struct {
		name        string
		increments  int
		threshold   int
		wantEmpty   bool
		wantContain string
	}{
		{
			name:       "below threshold returns empty",
			increments: 2,
			threshold:  3,
			wantEmpty:  true,
		},
		{
			name:        "at threshold returns reminder",
			increments:  3,
			threshold:   3,
			wantEmpty:   false,
			wantContain: "3 rounds",
		},
		{
			name:        "above threshold shows actual rounds",
			increments:  5,
			threshold:   3,
			wantEmpty:   false,
			wantContain: "5 rounds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTodoManager()
			for i := 0; i < tt.increments; i++ {
				tm.IncrementRoundsSinceUpdate()
			}

			got := tm.Reminder(tt.threshold)

			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty reminder, got %q", got)
				}
				return
			}
			if got == "" {
				t.Fatalf("expected reminder, got empty")
			}
			if tt.wantContain != "" && !strings.Contains(got, tt.wantContain) {
				t.Errorf("reminder %q does not contain %q", got, tt.wantContain)
			}
		})
	}
}

// TestUpdateResetsRounds 验证更新计划后未更新轮次被重置
func TestUpdateResetsRounds(t *testing.T) {
	tm := NewTodoManager()
	tm.IncrementRoundsSinceUpdate()
	tm.IncrementRoundsSinceUpdate()
	tm.IncrementRoundsSinceUpdate()

	if r := tm.Reminder(3); r == "" {
		t.Fatalf("expected reminder before update")
	}

	// Update 应重置计数
	if _, err := tm.Update([]planItemPatch{
		{ID: "s1", Content: ptr("step 1"), Status: ptr(StatusPending)},
	}); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if r := tm.Reminder(3); r != "" {
		t.Errorf("expected rounds reset after update, got reminder %q", r)
	}
}

// ---------- Call（端到端，含参数解析）----------

func TestCall(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]interface{}
		wantErr     bool
		wantContain string // 成功时校验返回内容，失败时校验错误内容
	}{
		{
			name:        "missing items",
			args:        map[string]interface{}{},
			wantErr:     true,
			wantContain: "missing items",
		},
		{
			name: "items not an array",
			args: map[string]interface{}{
				"items": "not-array",
			},
			wantErr:     true,
			wantContain: "must be an array",
		},
		{
			name: "item not an object",
			args: map[string]interface{}{
				"items": []interface{}{"oops"},
			},
			wantErr:     true,
			wantContain: "not a valid object",
		},
		{
			name: "valid replace",
			args: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":      "s1",
						"content": "step 1",
						"status":  StatusPending,
					},
				},
			},
			wantErr:     false,
			wantContain: "[ ] [s1] step 1",
		},
		{
			name: "merge as string true is parsed",
			args: map[string]interface{}{
				"merge": "true",
				"items": []interface{}{
					map[string]interface{}{
						"id":      "s1",
						"content": "new step",
						"status":  StatusPending,
					},
				},
			},
			wantErr:     false,
			wantContain: "[s1] new step",
		},
		{
			name: "replace missing content reports error",
			args: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":     "s1",
						"status": StatusPending,
					},
				},
			},
			wantErr:     true,
			wantContain: "content is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTodoManager()
			res := tm.Call(tt.args, nil) // ctx 为 nil，验证不 panic

			if tt.wantErr {
				if !res.IsError || res.Ok {
					t.Fatalf("expected error result, got %+v", res)
				}
			} else {
				if res.IsError || !res.Ok {
					t.Fatalf("expected ok result, got %+v", res)
				}
			}
			if tt.wantContain != "" && !strings.Contains(res.Content, tt.wantContain) {
				t.Errorf("result content %q does not contain %q", res.Content, tt.wantContain)
			}
		})
	}
}

// TestCallMergePartialUpdate 端到端验证 merge 只改 status 不丢 content
func TestCallMergePartialUpdate(t *testing.T) {
	tm := NewTodoManager()

	// 先建立计划
	res := tm.Call(map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"id": "s1", "content": "write tests", "status": StatusPending},
		},
	}, nil)
	if res.IsError {
		t.Fatalf("setup failed: %s", res.Content)
	}

	// merge 只发 id + status
	res = tm.Call(map[string]interface{}{
		"merge": true,
		"items": []interface{}{
			map[string]interface{}{"id": "s1", "status": StatusInProgress},
		},
	}, nil)
	if res.IsError {
		t.Fatalf("merge failed: %s", res.Content)
	}

	it := itemByID(tm.Items(), "s1")
	if it == nil || it.Content != "write tests" || it.Status != StatusInProgress {
		t.Errorf("partial merge did not preserve content / update status: %+v", it)
	}
}
