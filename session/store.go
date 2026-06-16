// session/store.go
//
// JSON 文件持久化层。
// 采用双层存储：index.json（会话索引）+ {id}.json（单会话完整数据）。
// 启动时执行 reconcile 修复索引与实际文件的不一致。
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"zoomClient/logger"

	"go.uber.org/zap"
)

const indexFileName = "index.json"

// Store 负责会话数据的 JSON 文件持久化。
type Store struct {
	dir string
}

// NewStore 创建 Store，并确保存储目录存在。
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		dir = "./.sessions"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}
	s := &Store{dir: dir}
	if err := s.reconcile(); err != nil {
		logger.Log.Warn("session store reconcile failed", zap.Error(err))
	}
	return s, nil
}

// Dir 返回存储目录路径。
func (s *Store) Dir() string {
	return s.dir
}

// indexPath 返回 index.json 的完整路径。
func (s *Store) indexPath() string {
	return filepath.Join(s.dir, indexFileName)
}

// sessionPath 返回指定会话 JSON 文件的完整路径。
func (s *Store) sessionPath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

// readIndex 读取 index.json，文件不存在时返回空切片。
func (s *Store) readIndex() ([]SessionMeta, error) {
	data, err := os.ReadFile(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []SessionMeta{}, nil
		}
		return nil, fmt.Errorf("read index: %w", err)
	}
	var metas []SessionMeta
	if err := json.Unmarshal(data, &metas); err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}
	return metas, nil
}

// writeIndex 将索引写入 index.json。
func (s *Store) writeIndex(metas []SessionMeta) error {
	data, err := json.MarshalIndent(metas, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}
	if err := os.WriteFile(s.indexPath(), data, 0644); err != nil {
		return fmt.Errorf("write index: %w", err)
	}
	return nil
}

// List 返回所有会话的元信息列表，按 UpdatedAt 倒序排列。
func (s *Store) List() ([]SessionMeta, error) {
	metas, err := s.readIndex()
	if err != nil {
		return nil, err
	}
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].UpdatedAt.After(metas[j].UpdatedAt)
	})
	return metas, nil
}

// Load 读取指定会话的完整数据（含消息历史）。
func (s *Store) Load(id string) (*SessionRecord, error) {
	if !isValidID(id) {
		return nil, fmt.Errorf("invalid session id: %s", id)
	}
	data, err := os.ReadFile(s.sessionPath(id))
	if err != nil {
		return nil, fmt.Errorf("read session %s: %w", id, err)
	}
	var record SessionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("parse session %s: %w", id, err)
	}
	return &record, nil
}

// Save 保存会话数据到磁盘，并更新索引。
func (s *Store) Save(record *SessionRecord) error {
	// 写入会话文件
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	if err := os.WriteFile(s.sessionPath(record.ID), data, 0644); err != nil {
		return fmt.Errorf("write session: %w", err)
	}

	// 更新索引
	metas, err := s.readIndex()
	if err != nil {
		return err
	}
	meta := record.ToMeta()
	found := false
	for i, m := range metas {
		if m.ID == record.ID {
			metas[i] = meta
			found = true
			break
		}
	}
	if !found {
		metas = append(metas, meta)
	}
	return s.writeIndex(metas)
}

// Delete 删除指定会话的文件并更新索引。
func (s *Store) Delete(id string) error {
	if !isValidID(id) {
		return fmt.Errorf("invalid session id: %s", id)
	}
	// 删除会话文件（不存在不报错）
	if err := os.Remove(s.sessionPath(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete session file: %w", err)
	}

	// 更新索引
	metas, err := s.readIndex()
	if err != nil {
		return err
	}
	filtered := make([]SessionMeta, 0, len(metas))
	for _, m := range metas {
		if m.ID != id {
			filtered = append(filtered, m)
		}
	}
	return s.writeIndex(filtered)
}

// Rename 更新指定会话的标题。同时更新会话文件和索引。
func (s *Store) Rename(id, newTitle string) error {
	record, err := s.Load(id)
	if err != nil {
		return err
	}
	record.Title = newTitle
	return s.Save(record)
}

// reconcile 扫描存储目录，修复 index.json 与实际文件的不一致。
// - 磁盘有文件但索引没有 → 补充到索引
// - 索引有条目但磁盘无文件 → 从索引移除
func (s *Store) reconcile() error {
	// 读取当前索引
	metas, err := s.readIndex()
	if err != nil {
		metas = []SessionMeta{}
	}
	indexMap := make(map[string]int, len(metas))
	for i, m := range metas {
		indexMap[m.ID] = i
	}

	// 扫描目录中的 JSON 文件
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("readdir: %w", err)
	}

	diskIDs := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == indexFileName || !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if !isValidID(id) {
			continue
		}
		diskIDs[id] = true

		// 磁盘有但索引没有 → 加载并补充
		if _, ok := indexMap[id]; !ok {
			record, lerr := s.Load(id)
			if lerr != nil {
				logger.Log.Warn("reconcile: skip corrupt session file",
					zap.String("id", id), zap.Error(lerr))
				continue
			}
			metas = append(metas, record.ToMeta())
			indexMap[id] = len(metas) - 1
		}
	}

	// 索引有但磁盘没有 → 移除
	filtered := make([]SessionMeta, 0, len(metas))
	for _, m := range metas {
		if diskIDs[m.ID] {
			filtered = append(filtered, m)
		}
	}

	return s.writeIndex(filtered)
}

// isValidID 校验会话 ID 是否为合法格式（UUID 或仅含字母数字连字符）。
func isValidID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
