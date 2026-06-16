package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"zoomClient/fsm"
	"zoomClient/logger"

	"go.uber.org/zap"
)

func init() {
	logger.Log, _ = zap.NewDevelopment()
}

// tempStore 创建一个临时目录下的 Store，测试结束后自动清理。
func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	return s
}

func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return data
}

func TestStore_SaveAndLoad(t *testing.T) {
	s := tempStore(t)

	record := &SessionRecord{
		ID:        "test-001",
		Title:     "测试会话",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Model:     "gpt-4o",
		TurnCount: 2,
		Messages: []fsm.Message{
			{Role: "user", Content: "你好"},
			{Role: "assistant", Content: "你好！有什么可以帮你的？"},
		},
	}

	if err := s.Save(record); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(s.Dir(), "test-001.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("session file not created")
	}

	loaded, err := s.Load("test-001")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.ID != record.ID {
		t.Errorf("ID mismatch: got %s, want %s", loaded.ID, record.ID)
	}
	if loaded.Title != record.Title {
		t.Errorf("Title mismatch: got %s, want %s", loaded.Title, record.Title)
	}
	if len(loaded.Messages) != 2 {
		t.Errorf("Messages count: got %d, want 2", len(loaded.Messages))
	}
}

func TestStore_List(t *testing.T) {
	s := tempStore(t)

	for i := 0; i < 3; i++ {
		record := &SessionRecord{
			ID:        []string{"list-a", "list-b", "list-c"}[i],
			Title:     []string{"会话 A", "会话 B", "会话 C"}[i],
			CreatedAt: time.Now(),
			UpdatedAt: time.Now().Add(time.Duration(i) * time.Minute),
			TurnCount: i + 1,
		}
		if err := s.Save(record); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	metas, err := s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(metas) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(metas))
	}
	// Most recent first
	if metas[0].ID != "list-c" {
		t.Errorf("first should be most recent, got %s", metas[0].ID)
	}
}

func TestStore_Delete(t *testing.T) {
	s := tempStore(t)

	record := &SessionRecord{
		ID:        "del-001",
		Title:     "要删除的",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.Save(record); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if err := s.Delete("del-001"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	path := filepath.Join(s.Dir(), "del-001.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("session file still exists after delete")
	}

	metas, _ := s.List()
	if len(metas) != 0 {
		t.Errorf("index should be empty, got %d entries", len(metas))
	}
}

func TestStore_Rename(t *testing.T) {
	s := tempStore(t)

	record := &SessionRecord{
		ID:        "ren-001",
		Title:     "旧标题",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.Save(record); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if err := s.Rename("ren-001", "新标题"); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	loaded, _ := s.Load("ren-001")
	if loaded.Title != "新标题" {
		t.Errorf("Title not updated: got %s", loaded.Title)
	}
}

func TestStore_InvalidID(t *testing.T) {
	s := tempStore(t)

	if _, err := s.Load("../etc/passwd"); err == nil {
		t.Error("should reject invalid ID")
	}
	if err := s.Delete("../../bad"); err == nil {
		t.Error("should reject invalid ID for delete")
	}
}

func TestStore_Reconcile(t *testing.T) {
	dir := t.TempDir()

	record := &SessionRecord{
		ID:        "orphan-001",
		Title:     "孤儿会话",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	data := mustMarshal(t, record)
	if err := os.WriteFile(filepath.Join(dir, "orphan-001.json"), data, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	metas, _ := s.List()
	if len(metas) != 1 {
		t.Fatalf("reconcile should find orphan, got %d entries", len(metas))
	}
	if metas[0].ID != "orphan-001" {
		t.Errorf("expected orphan-001, got %s", metas[0].ID)
	}
}
