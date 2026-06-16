package session

import (
	"testing"
	"time"
	"zoomClient/clients"
	"zoomClient/fsm"
	"zoomClient/logger"
	"zoomClient/tools"

	"go.uber.org/zap"
)

func init() {
	// Initialize logger for tests
	logger.Log, _ = zap.NewDevelopment()
}

// mockClient 是一个简单的 mock ChatClient，用于测试。
type mockClient struct{}

func (m *mockClient) Chat(model string, messages []fsm.Message, toolList []tools.Tool, options map[string]interface{}) (*clients.ChatResponse, error) {
	return &clients.ChatResponse{
		Message: fsm.Message{Content: "Go语言排序算法讲解"},
	}, nil
}

func tempManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	mgr, err := NewManager(dir, &mockClient{}, "test-model")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	return mgr
}

func TestManager_CreateAndCurrent(t *testing.T) {
	mgr := tempManager(t)

	r1 := mgr.CreateSession()
	if r1.ID == "" {
		t.Fatal("CreateSession returned empty ID")
	}
	if mgr.Current() != r1.ID {
		t.Errorf("Current() = %s, want %s", mgr.Current(), r1.ID)
	}

	r2 := mgr.CreateSession()
	if mgr.Current() != r2.ID {
		t.Errorf("Current() should point to latest session")
	}
}

func TestManager_SaveAndLoad(t *testing.T) {
	mgr := tempManager(t)

	r := mgr.CreateSession()
	r.Messages = []fsm.Message{
		{Role: "user", Content: "你好"},
		{Role: "assistant", Content: "你好！"},
	}
	r.TurnCount = 1

	if err := mgr.Save(r); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := mgr.Load(r.ID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.TurnCount != 1 {
		t.Errorf("TurnCount = %d, want 1", loaded.TurnCount)
	}
	if mgr.Current() != r.ID {
		t.Error("Load should set current")
	}
}

func TestManager_SaveEmptySkipped(t *testing.T) {
	mgr := tempManager(t)

	r := mgr.CreateSession()
	// Empty session - no messages, TurnCount=0
	if err := mgr.Save(r); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	metas, _ := mgr.List()
	if len(metas) != 0 {
		t.Errorf("empty session should not be saved, got %d", len(metas))
	}
}

func TestManager_Delete(t *testing.T) {
	mgr := tempManager(t)

	r1 := mgr.CreateSession()
	r1.TurnCount = 1
	r1.Messages = []fsm.Message{{Role: "user", Content: "test"}}
	mgr.Save(r1)

	r2 := mgr.CreateSession()
	r2.TurnCount = 1
	r2.Messages = []fsm.Message{{Role: "user", Content: "test2"}}
	mgr.Save(r2)

	// Delete current (r2), should switch to r1
	if err := mgr.Delete(r2.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if mgr.Current() != r1.ID {
		t.Errorf("after deleting current, should switch to r1, got %s", mgr.Current())
	}

	metas, _ := mgr.List()
	if len(metas) != 1 {
		t.Errorf("should have 1 session, got %d", len(metas))
	}
}

func TestManager_DeleteLastCreatesNew(t *testing.T) {
	mgr := tempManager(t)

	r := mgr.CreateSession()
	r.TurnCount = 1
	r.Messages = []fsm.Message{{Role: "user", Content: "test"}}
	mgr.Save(r)

	mgr.Delete(r.ID)
	// Should auto-create a new session
	if mgr.Current() == "" {
		t.Error("should have a current session after deleting the last one")
	}
}

func TestManager_Rename(t *testing.T) {
	mgr := tempManager(t)

	r := mgr.CreateSession()
	r.TurnCount = 1
	r.Messages = []fsm.Message{{Role: "user", Content: "test"}}
	mgr.Save(r)

	if err := mgr.Rename(r.ID, "新名称"); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	loaded, _ := mgr.Load(r.ID)
	if loaded.Title != "新名称" {
		t.Errorf("Title = %s, want 新名称", loaded.Title)
	}
}

func TestManager_RenameEmpty(t *testing.T) {
	mgr := tempManager(t)

	r := mgr.CreateSession()
	if err := mgr.Rename(r.ID, ""); err == nil {
		t.Error("should reject empty title")
	}
}

func TestManager_GenerateTitle(t *testing.T) {
	mgr := tempManager(t)

	record := &SessionRecord{
		ID:        "title-001",
		Title:     "新会话",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages: []fsm.Message{
			{Role: "user", Content: "请用Go实现快速排序"},
			{Role: "assistant", Content: "好的，以下是Go语言实现快速排序的代码..."},
		},
	}

	title, err := mgr.GenerateTitle(record)
	if err != nil {
		t.Fatalf("GenerateTitle failed: %v", err)
	}
	if title == "" {
		t.Error("title should not be empty")
	}
	// mock returns "Go语言排序算法讲解"
	if title != "Go语言排序算法讲解" {
		t.Errorf("title = %s", title)
	}
}

func TestManager_GenerateTitleFallback(t *testing.T) {
	mgr := tempManager(t)

	record := &SessionRecord{
		ID:        "title-002",
		Title:     "新会话",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  []fsm.Message{}, // no messages
	}

	title, err := mgr.GenerateTitle(record)
	if err != nil {
		t.Fatalf("GenerateTitle failed: %v", err)
	}
	if title != "新会话" {
		t.Errorf("fallback title = %s, want 新会话", title)
	}
}
