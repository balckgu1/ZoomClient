package session

import (
	"encoding/json"
	"testing"
	"time"
)

// TestToMeta_CarriesWorkDir 验证 ToMeta 会把工作目录带到元信息，供前端按工作区分组。
func TestToMeta_CarriesWorkDir(t *testing.T) {
	record := &SessionRecord{
		ID:        "id-1",
		Title:     "title",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Model:     "model",
		TurnCount: 3,
		WorkDir:   "C:/work/project-a",
	}
	meta := record.ToMeta()
	if meta.WorkDir != "C:/work/project-a" {
		t.Errorf("expected WorkDir to be carried, got %q", meta.WorkDir)
	}
}

// TestSessionMeta_JSONHasWorkDir 验证 SessionMeta 序列化会输出 workdir 字段。
func TestSessionMeta_JSONHasWorkDir(t *testing.T) {
	meta := SessionMeta{
		ID:        "id-2",
		Title:     "t",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		TurnCount: 1,
		WorkDir:   "/home/user/project-b",
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if v, ok := obj["workdir"]; !ok || v != "/home/user/project-b" {
		t.Errorf("expected workdir field %q, got %v (present=%v)", "/home/user/project-b", v, ok)
	}
}
