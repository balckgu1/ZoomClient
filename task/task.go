package task

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	StatusPending    = "pending"
	StatusInprogress = "in_progress"
	StatusCompleted  = "completed"
)

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
