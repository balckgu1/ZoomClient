package task

// 本文件为任务系统的单元测试，覆盖创建/列举/依赖阻塞/认领/完成解锁/
// 非法状态流转/缺失任务/路径穿越等关键路径。每个用例使用独立临时目录，互不影响。

import (
	"os"
	"strings"
	"testing"
)

// newTestManager 创建指向独立临时目录的任务管理器，保证测试隔离
func newTestManager(t *testing.T) *TaskManager {
	t.Helper()
	return NewTaskManager(t.TempDir())
}

func TestCreateTask_落盘并可读取(t *testing.T) {
	m := newTestManager(t)
	task, err := m.CreateTask("setup schema", "create db tables", nil)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	if task.Status != StatusPending {
		t.Errorf("expected pending, got %s", task.Status)
	}
	// 文件应真实落盘
	if _, err := os.Stat(m.dir + string(os.PathSeparator) + task.ID + ".json"); err != nil {
		t.Fatalf("task file not persisted: %v", err)
	}
	got, err := m.GetTask(task.ID)
	if err != nil || got.Subject != "setup schema" {
		t.Fatalf("GetTask mismatch: %v, %+v", err, got)
	}
}

func TestCreateTask_空标题报错(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.CreateTask("  ", "", nil); err == nil {
		t.Error("expected error for empty subject")
	}
}

func TestClaimTask_依赖未完成时被阻塞(t *testing.T) {
	m := newTestManager(t)
	schema, _ := m.CreateTask("schema", "", nil)
	api, _ := m.CreateTask("api", "", []string{schema.ID})

	msg, err := m.ClaimTask(api.ID, "agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, "Blocked by") {
		t.Errorf("expected blocked message, got %q", msg)
	}
	// 被阻塞时状态不应改变
	apiNow, _ := m.GetTask(api.ID)
	if apiNow.Status != StatusPending {
		t.Errorf("blocked task should stay pending, got %s", apiNow.Status)
	}
}

func TestCompleteTask_完成后解锁下游(t *testing.T) {
	m := newTestManager(t)
	schema, _ := m.CreateTask("schema", "", nil)
	api, _ := m.CreateTask("api", "", []string{schema.ID})

	if _, err := m.ClaimTask(schema.ID, "agent"); err != nil {
		t.Fatalf("claim schema failed: %v", err)
	}
	msg, err := m.CompleteTask(schema.ID)
	if err != nil {
		t.Fatalf("complete schema failed: %v", err)
	}
	if !strings.Contains(msg, "Unblocked") || !strings.Contains(msg, "api") {
		t.Errorf("expected api to be unblocked, got %q", msg)
	}
	// 解锁后 api 可被认领
	if msg, _ := m.ClaimTask(api.ID, "agent"); !strings.Contains(msg, "Claimed") {
		t.Errorf("expected api claimable after unlock, got %q", msg)
	}
}

func TestClaimTask_非法状态流转被拒绝(t *testing.T) {
	m := newTestManager(t)
	task, _ := m.CreateTask("t", "", nil)
	_, _ = m.ClaimTask(task.ID, "agent") // → in_progress

	// 重复认领应被拒绝
	if msg, _ := m.ClaimTask(task.ID, "other"); !strings.Contains(msg, "cannot claim") {
		t.Errorf("expected reject on re-claim, got %q", msg)
	}
	// 未完成前直接 complete 一个 pending 任务应被拒绝
	other, _ := m.CreateTask("other", "", nil)
	if msg, _ := m.CompleteTask(other.ID); !strings.Contains(msg, "cannot complete") {
		t.Errorf("expected reject completing a pending task, got %q", msg)
	}
}

func TestCanStart_缺失依赖视为阻塞(t *testing.T) {
	m := newTestManager(t)
	task, _ := m.CreateTask("t", "", []string{"task_not_exist"})
	ok, err := m.CanStart(task.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("missing dependency should be treated as blocked")
	}
}

func TestGetTask_不存在返回NotExist(t *testing.T) {
	m := newTestManager(t)
	_, err := m.GetTask("task_missing")
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

func TestListTasks_空目录返回空列表(t *testing.T) {
	m := newTestManager(t)
	list, err := m.ListTasks()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

func TestTaskPath_拒绝路径穿越(t *testing.T) {
	m := newTestManager(t)
	for _, bad := range []string{"../escape", "..", "a/b", `a\b`} {
		if _, err := m.GetTask(bad); err == nil {
			t.Errorf("expected error for malicious id %q", bad)
		}
	}
}
