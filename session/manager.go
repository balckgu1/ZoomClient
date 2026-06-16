// session/manager.go
//
// SessionManager 管理会话的完整生命周期：创建、保存、加载、删除、重命名。
// 内部使用 sync.Mutex 保护并发写操作，维护 current（当前活跃会话 ID）。
package session

import (
	"fmt"
	"sync"
	"time"
	"zoomClient/clients"
	"zoomClient/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Manager 管理多个会话的生命周期。
type Manager struct {
	store   *Store
	client  clients.ChatClient
	model   string
	current string // 当前活跃会话 ID
	mu      sync.Mutex
}

// NewManager 创建会话管理器。
// dir: 存储目录；client: LLM 客户端（用于自动命名）；model: 模型名称。
func NewManager(dir string, client clients.ChatClient, model string) (*Manager, error) {
	store, err := NewStore(dir)
	if err != nil {
		return nil, fmt.Errorf("init session store: %w", err)
	}
	return &Manager{
		store:  store,
		client: client,
		model:  model,
	}, nil
}

// Store 返回底层持久化存储（供外部直接访问，如 API handler）。
func (m *Manager) Store() *Store {
	return m.store
}

// CreateSession 新建一个空会话并设为当前活跃会话。
func (m *Manager) CreateSession() *SessionRecord {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	record := &SessionRecord{
		ID:        uuid.NewString(),
		Title:     "NewSession",
		CreatedAt: now,
		UpdatedAt: now,
		Model:     m.model,
	}
	m.current = record.ID
	logger.Log.Info("session created", zap.String("id", record.ID))
	return record
}

// Current 返回当前活跃会话的 ID。
func (m *Manager) Current() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

// SetCurrent 设置当前活跃会话 ID。
func (m *Manager) SetCurrent(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = id
}

// Save 保存会话到磁盘并更新索引。
// 空会话（无用户消息且 TurnCount == 0）不写入磁盘。
func (m *Manager) Save(record *SessionRecord) error {
	if record.IsEmpty() {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	record.UpdatedAt = time.Now()
	if err := m.store.Save(record); err != nil {
		logger.Log.Error("save session failed",
			zap.String("id", record.ID), zap.Error(err))
		return err
	}
	return nil
}

// List 返回所有会话的元信息列表（按时间倒序）。
func (m *Manager) List() ([]SessionMeta, error) {
	return m.store.List()
}

// Load 加载指定会话的完整数据并设为当前活跃会话。
func (m *Manager) Load(id string) (*SessionRecord, error) {
	record, err := m.store.Load(id)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.current = id
	m.mu.Unlock()

	logger.Log.Info("session loaded", zap.String("id", id),
		zap.Int("messages", len(record.Messages)))
	return record, nil
}

// Delete 删除指定会话。如果删除的是当前活跃会话，自动切换到最新会话或新建。
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	isCurrent := m.current == id
	m.mu.Unlock()

	if err := m.store.Delete(id); err != nil {
		return err
	}
	logger.Log.Info("session deleted", zap.String("id", id))

	// 如果删除的是当前会话，需要切换
	if isCurrent {
		metas, _ := m.store.List()
		if len(metas) > 0 {
			m.mu.Lock()
			m.current = metas[0].ID
			m.mu.Unlock()
		} else {
			// 没有剩余会话，新建一个
			m.CreateSession()
		}
	}
	return nil
}

// Rename 重命名指定会话的标题。
func (m *Manager) Rename(id, newTitle string) error {
	if newTitle == "" {
		return fmt.Errorf("title cannot be empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.store.Rename(id, newTitle)
}
