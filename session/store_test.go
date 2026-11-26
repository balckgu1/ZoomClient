package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"zoomClient/clients"
	"zoomClient/logger"

	"go.uber.org/zap"
)

func init() {
	logger.Log, _ = zap.NewDevelopment()
}

// tempStore 创建一个由临时目录支持的 Store；清理是自动的。
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
	tests := []struct {
		name    string
		record  *SessionRecord
		wantErr bool
	}{
		{
			name: "basic save and load with messages",
			record: &SessionRecord{
				ID:        "test-001",
				Title:     "hello session",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Model:     "gpt-4o",
				TurnCount: 2,
				Messages: []clients.Message{
					{Role: "user", Content: "hello"},
					{Role: "assistant", Content: "hi! how can I help?"},
				},
			},
			wantErr: false,
		},
		{
			name: "record with no messages",
			record: &SessionRecord{
				ID:        "test-002",
				Title:     "empty session",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				TurnCount: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tempStore(t)

			err := s.Save(tt.record)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Save() error = %v, wantErr %v", err, tt.wantErr)
			}

			// 验证文件存在于磁盘上
			path := filepath.Join(s.Dir(), tt.record.ID+".json")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Fatal("session file not created")
			}

			loaded, err := s.Load(tt.record.ID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
			if loaded.ID != tt.record.ID {
				t.Errorf("ID mismatch: got %s, want %s", loaded.ID, tt.record.ID)
			}
			if loaded.Title != tt.record.Title {
				t.Errorf("Title mismatch: got %s, want %s", loaded.Title, tt.record.Title)
			}
			if len(loaded.Messages) != len(tt.record.Messages) {
				t.Errorf("Messages count: got %d, want %d", len(loaded.Messages), len(tt.record.Messages))
			}
		})
	}
}

func TestStore_List(t *testing.T) {
	tests := []struct {
		name      string
		records   []*SessionRecord
		wantCount int
		wantFirst string // 最近一次的 ID（列表中的第一个）
	}{
		{
			name: "three sessions sorted by UpdatedAt desc",
			records: []*SessionRecord{
				{ID: "list-a", Title: "session A", CreatedAt: time.Now(), UpdatedAt: time.Now(), TurnCount: 1},
				{ID: "list-b", Title: "session B", CreatedAt: time.Now(), UpdatedAt: time.Now().Add(1 * time.Minute), TurnCount: 2},
				{ID: "list-c", Title: "session C", CreatedAt: time.Now(), UpdatedAt: time.Now().Add(2 * time.Minute), TurnCount: 3},
			},
			wantCount: 3,
			wantFirst: "list-c",
		},
		{
			name:      "empty store returns no sessions",
			records:   nil,
			wantCount: 0,
			wantFirst: "",
		},
		{
			name: "single session",
			records: []*SessionRecord{
				{ID: "only-one", Title: "only session", CreatedAt: time.Now(), UpdatedAt: time.Now(), TurnCount: 1},
			},
			wantCount: 1,
			wantFirst: "only-one",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tempStore(t)

			for _, r := range tt.records {
				if err := s.Save(r); err != nil {
					t.Fatalf("Save failed: %v", err)
				}
			}

			metas, err := s.List()
			if err != nil {
				t.Fatalf("List failed: %v", err)
			}
			if len(metas) != tt.wantCount {
				t.Fatalf("expected %d sessions, got %d", tt.wantCount, len(metas))
			}
			if tt.wantFirst != "" && metas[0].ID != tt.wantFirst {
				t.Errorf("first should be most recent, got %s want %s", metas[0].ID, tt.wantFirst)
			}
		})
	}
}

func TestStore_Delete(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		setupFirst bool // 删除前是否保存记录
		wantErr    bool
	}{
		{
			name:       "delete existing session",
			id:         "del-001",
			setupFirst: true,
			wantErr:    false,
		},
		{
			name:       "delete non-existent session silently succeeds",
			id:         "del-missing",
			setupFirst: false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tempStore(t)

			if tt.setupFirst {
				record := &SessionRecord{
					ID:        tt.id,
					Title:     "to be deleted",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				if err := s.Save(record); err != nil {
					t.Fatalf("Save failed: %v", err)
				}
			}

			err := s.Delete(tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}

			// 验证文件已删除
			path := filepath.Join(s.Dir(), tt.id+".json")
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Error("session file still exists after delete")
			}

			// 验证索引已清理
			metas, _ := s.List()
			for _, m := range metas {
				if m.ID == tt.id {
					t.Errorf("index still contains deleted id %s", tt.id)
				}
			}
		})
	}
}

func TestStore_Rename(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		oldTitle string
		newTitle string
		wantErr  bool
	}{
		{
			name:     "rename existing session",
			id:       "ren-001",
			oldTitle: "old title",
			newTitle: "new title",
			wantErr:  false,
		},
		{
			name:     "rename to empty string",
			id:       "ren-002",
			oldTitle: "some title",
			newTitle: "",
			wantErr:  false,
		},
		{
			name:     "rename non-existent session returns error",
			id:       "ren-missing",
			oldTitle: "",
			newTitle: "anything",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tempStore(t)

			if tt.oldTitle != "" || tt.id != "ren-missing" {
				record := &SessionRecord{
					ID:        tt.id,
					Title:     tt.oldTitle,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				if err := s.Save(record); err != nil {
					t.Fatalf("Save failed: %v", err)
				}
			}

			err := s.Rename(tt.id, tt.newTitle)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Rename() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			loaded, err := s.Load(tt.id)
			if err != nil {
				t.Fatalf("Load after Rename failed: %v", err)
			}
			if loaded.Title != tt.newTitle {
				t.Errorf("Title not updated: got %q, want %q", loaded.Title, tt.newTitle)
			}
		})
	}
}

func TestStore_InvalidID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		op   string // "load" 或 "delete"
	}{
		{name: "load with path traversal", id: "../etc/passwd", op: "load"},
		{name: "delete with path traversal", id: "../../bad", op: "delete"},
		{name: "load with empty id", id: "", op: "load"},
		{name: "delete with special chars", id: "abc/def", op: "delete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tempStore(t)

			var err error
			switch tt.op {
			case "load":
				_, err = s.Load(tt.id)
			case "delete":
				err = s.Delete(tt.id)
			}
			if err == nil {
				t.Errorf("%s should reject invalid ID %q", tt.op, tt.id)
			}
		})
	}
}

func TestStore_Reconcile(t *testing.T) {
	tests := []struct {
		name       string
		setupFiles []*SessionRecord // NewStore 之前写入磁盘的文件
		wantCount  int
		wantIDs    []string
	}{
		{
			name: "orphan file is picked up by reconcile",
			setupFiles: []*SessionRecord{
				{ID: "orphan-001", Title: "orphan session", CreatedAt: time.Now(), UpdatedAt: time.Now()},
			},
			wantCount: 1,
			wantIDs:   []string{"orphan-001"},
		},
		{
			name: "multiple orphan files",
			setupFiles: []*SessionRecord{
				{ID: "orphan-a", Title: "session A", CreatedAt: time.Now(), UpdatedAt: time.Now()},
				{ID: "orphan-b", Title: "session B", CreatedAt: time.Now(), UpdatedAt: time.Now().Add(1 * time.Minute)},
			},
			wantCount: 2,
			wantIDs:   []string{"orphan-a", "orphan-b"},
		},
		{
			name:       "empty directory yields no sessions",
			setupFiles: nil,
			wantCount:  0,
			wantIDs:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			for _, r := range tt.setupFiles {
				data := mustMarshal(t, r)
				if err := os.WriteFile(filepath.Join(dir, r.ID+".json"), data, 0644); err != nil {
					t.Fatalf("write failed: %v", err)
				}
			}

			s, err := NewStore(dir)
			if err != nil {
				t.Fatalf("NewStore failed: %v", err)
			}

			metas, err := s.List()
			if err != nil {
				t.Fatalf("List failed: %v", err)
			}
			if len(metas) != tt.wantCount {
				t.Fatalf("expected %d sessions after reconcile, got %d", tt.wantCount, len(metas))
			}
			idSet := make(map[string]bool)
			for _, m := range metas {
				idSet[m.ID] = true
			}
			for _, wantID := range tt.wantIDs {
				if !idSet[wantID] {
					t.Errorf("expected id %q not found in reconciled index", wantID)
				}
			}
		})
	}
}

// TestStore_ContextRoundtrip 验证上下文洞察快照的保存/读取/非法 ID 拒绝
func TestStore_ContextRoundtrip(t *testing.T) {
	s := tempStore(t)
	data := []byte(`{"seq":7,"model":"m"}`)
	if err := s.SaveContext("ctx-test", data); err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}
	got, err := s.LoadContext("ctx-test")
	if err != nil || string(got) != string(data) {
		t.Fatalf("LoadContext = %q, %v; want roundtrip", got, err)
	}
	// 不存在的快照返回可识别的 not-exist 错误（调用方据此降级）
	if _, err := s.LoadContext("missing"); !os.IsNotExist(err) {
		t.Fatalf("LoadContext(missing) err = %v, want not-exist", err)
	}
	// 非法 ID（路径穿越）必须拒绝
	if err := s.SaveContext("../evil", data); err == nil {
		t.Fatal("SaveContext should reject invalid id")
	}
	if _, err := s.LoadContext("../evil"); err == nil {
		t.Fatal("LoadContext should reject invalid id")
	}
}

// TestStore_DeleteRemovesContext 验证删除会话连带删除快照文件
func TestStore_DeleteRemovesContext(t *testing.T) {
	s := tempStore(t)
	r := &SessionRecord{ID: "del-ctx", Title: "t", CreatedAt: time.Now(), UpdatedAt: time.Now(), TurnCount: 1}
	if err := s.Save(r); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := s.SaveContext("del-ctx", []byte(`{}`)); err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}
	if err := s.Delete("del-ctx"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := s.LoadContext("del-ctx"); !os.IsNotExist(err) {
		t.Fatalf("context file should be removed with session, err = %v", err)
	}
}

// TestStore_ReconcileIgnoresContextFiles 验证快照文件不会被 reconcile 误入索引
func TestStore_ReconcileIgnoresContextFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "orphan.context.json"), []byte(`{}`), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	metas, err := s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(metas) != 0 {
		t.Fatalf("context file leaked into index: %+v", metas)
	}
}
