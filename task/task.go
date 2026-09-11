package task

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
)

// statusIcon 返回状态对应的可读图标，用于 list_tasks 渲染
func statusIcon(status string) string {
	switch status {
	case StatusPending:
		return "○"
	case StatusInProgress:
		return "●"
	case StatusCompleted:
		return "✓"
	default:
		return "?"
	}
}

type Task struct {
	ID          string   `json:"id"`          // 任务ID
	Subject     string   `json:"subject"`     // 简短标题
	Description string   `json:"description"` // 详细描述
	Status      string   `json:"status"`      // 任务状态 pending | in_progress | completed
	Owner       string   `json:"owner"`       // 哪个agent持有该任务
	BlockedBy   []string `json:"blockedBy"`   // 依赖上游任务的ID列表，全部completed时才能开始该任务
}

// TaskManager 任务管理器，负责任务的持久化与状态流转
type TaskManager struct {
	mu  sync.Mutex
	dir string // 任务文件存储目录
}

// NewTaskManager 实例化一个任务管理器
func NewTaskManager(dir string) *TaskManager {
	if strings.TrimSpace(dir) == "" {
		dir = ".task"
	}
	return &TaskManager{dir: dir}
}

// CreateTask 新建一个 pending 任务并持久化保存
func (m *TaskManager) CreateTask(subject string, description string, blockedBy []string) (*Task, error) {
	if strings.TrimSpace(subject) == "" {
		return nil, fmt.Errorf("subject is required")
	}
	task := &Task{
		ID:          newTaskID(),
		Subject:     subject,
		Description: description,
		Status:      StatusPending,
		BlockedBy:   blockedBy,
	}
	if task.BlockedBy == nil {
		task.BlockedBy = []string{}
	}
	if err := m.save(task); err != nil {
		return nil, err
	}
	return task, nil
}

// GetTask 根据 ID 获取单个任务
func (m *TaskManager) GetTask(id string) (*Task, error) {
	path, err := m.taskPath(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Task
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("corrupted task file %s: %w", path, err)
	}
	return &t, nil
}

// ListTasks 返回全部任务，按 ID 排序以保证输出稳定。目录不存在时返回空列表。
func (m *TaskManager) ListTasks() ([]*Task, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	tasks := make([]*Task, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		t, lerr := m.GetTask(id)
		if lerr != nil {
			continue // 跳过损坏文件，避免单个坏文件导致整体列举失败
		}
		tasks = append(tasks, t)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks, nil
}

// ClaimTask 认领任务：设置 owner，状态 pending → in_progress。
// 已非 pending（被他人认领/已完成）或依赖未完成时拒绝认领，返回可读原因。
func (m *TaskManager) ClaimTask(id, owner string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, err := m.GetTask(id)
	if err != nil {
		return "", err
	}
	if t.Status != StatusPending {
		return fmt.Sprintf("Task %s is %s, cannot claim", id, t.Status), nil
	}
	ok, err := m.CanStart(id)
	if err != nil {
		return "", err
	}
	if !ok {
		// 收集仍未完成的依赖，给模型明确反馈
		blocked := make([]string, 0, len(t.BlockedBy))
		for _, dep := range t.BlockedBy {
			d, derr := m.GetTask(dep)
			if derr != nil || d.Status != StatusCompleted {
				blocked = append(blocked, dep)
			}
		}
		return fmt.Sprintf("Blocked by: %s", strings.Join(blocked, ", ")), nil
	}
	if strings.TrimSpace(owner) == "" {
		owner = "agent"
	}
	t.Owner = owner
	t.Status = StatusInProgress
	if err := m.save(t); err != nil {
		return "", err
	}
	return fmt.Sprintf("Claimed %s (%s)", t.ID, t.Subject), nil
}

// CompleteTask 完成任务：in_progress → completed，并扫描报告刚被解锁的下游任务。
func (m *TaskManager) CompleteTask(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, err := m.GetTask(id)
	if err != nil {
		return "", err
	}
	if t.Status != StatusInProgress {
		return fmt.Sprintf("Task %s is %s, cannot complete", id, t.Status), nil
	}
	t.Status = StatusCompleted
	if err := m.save(t); err != nil {
		return "", err
	}
	// 找出因本次完成而变为可开始的 pending 下游任务
	unblocked := make([]string, 0)
	all, _ := m.ListTasks()
	for _, other := range all {
		if other.Status != StatusPending || len(other.BlockedBy) == 0 {
			continue
		}
		if ok, _ := m.CanStart(other.ID); ok {
			unblocked = append(unblocked, other.Subject)
		}
	}
	msg := fmt.Sprintf("Completed %s (%s)", t.ID, t.Subject)
	if len(unblocked) > 0 {
		msg += "\nUnblocked: " + strings.Join(unblocked, ", ")
	}
	return msg, nil
}

// CanStart 判断任务的所有 blockedBy 依赖是否都已 completed。
// 缺失或损坏的依赖一律视为"阻塞"，避免引用错误 ID 时被误判为可开始。
func (m *TaskManager) CanStart(id string) (bool, error) {
	t, err := m.GetTask(id)
	if err != nil {
		return false, err
	}
	for _, dep := range t.BlockedBy {
		d, derr := m.GetTask(dep)
		if derr != nil || d.Status != StatusCompleted {
			return false, nil
		}
	}
	return true, nil
}

// save 将任务写入 {dir}/{id}.json，必要时创建目录。
func (m *TaskManager) save(task *Task) error {
	path, err := m.taskPath(task.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	err = os.WriteFile(path, data, 0o644)
	if err != nil {
		return err
	}
	return nil
}

// taskPath 返回任务文件路径，并校验 id 不会造成路径穿越。
// id 可能来自模型输入（get/claim/complete），因此必须做安全校验。
func (m *TaskManager) taskPath(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("task id is empty")
	}
	// 拒绝路径分隔符与 . / .. ，防止 ../ 逃逸出任务目录
	if id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("invalid task id: %q", id)
	}
	base, err := filepath.Abs(m.dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, filepath.Base(id)+".json"), nil
}

// newTaskID 生成 task_{unix}_{4字节hex} 形式的唯一 ID
func newTaskID() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		// 极端情况下回退到纳秒时间戳，仍保证基本唯一性
		return fmt.Sprintf("task_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("task_%d_%s", time.Now().Unix(), hex.EncodeToString(buf))
}
