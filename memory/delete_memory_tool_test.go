package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeleteMemoryTool_Name(t *testing.T) {
	tool := NewDeleteMemoryTool("")
	if tool.Name() != "delete_memory" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "delete_memory")
	}
}

func TestDeleteMemoryTool_Call(t *testing.T) {
	tests := []struct {
		name        string
		memDir      string // "temp" means create temp dir
		args        map[string]any
		wantOk      bool
		wantErr     bool
		wantContain string
		setupFunc   func(t *testing.T, dir string)
		verifyFunc  func(t *testing.T, dir string)
	}{
		{
			name:        "missing name param",
			memDir:      "temp",
			args:        map[string]any{},
			wantOk:      false,
			wantErr:     true,
			wantContain: "name parameter is required",
		},
		{
			name:        "name not string",
			memDir:      "temp",
			args:        map[string]any{"name": 123},
			wantOk:      false,
			wantErr:     true,
			wantContain: "name parameter must be a non-empty string",
		},
		{
			name:        "name is empty",
			memDir:      "temp",
			args:        map[string]any{"name": "  "},
			wantOk:      false,
			wantErr:     true,
			wantContain: "name parameter must be a non-empty string",
		},
		{
			name:        "empty memoryDir",
			memDir:      "",
			args:        map[string]any{"name": "test"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "MemoryDir is not configured",
		},
		{
			name:        "memory not in index",
			memDir:      "temp",
			args:        map[string]any{"name": "nonexistent"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "does not exist",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFileRaw(t, dir, "MEMORY.md", "# Memory Index\n- other: something [user]\n")
			},
		},
		{
			name:        "success - delete existing memory",
			memDir:      "temp",
			args:        map[string]any{"name": "old_memory"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "deleted successfully",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "old_memory", "old_memory", "Old stuff", "user", "Old content")
				buildIndex(t, dir)
			},
			verifyFunc: func(t *testing.T, dir string) {
				// File should be gone
				if _, err := os.Stat(filepath.Join(dir, "old_memory.md")); !os.IsNotExist(err) {
					t.Error("expected old_memory.md to be deleted")
				}
				// MEMORY.md should not contain it
				data, _ := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
				if strings.Contains(string(data), "old_memory") {
					t.Error("expected MEMORY.md to not contain old_memory after delete")
				}
			},
		},
		{
			name:        "success - delete and index still has other entries",
			memDir:      "temp",
			args:        map[string]any{"name": "to_delete"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "deleted successfully",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "to_delete", "to_delete", "Will be deleted", "feedback", "Delete me")
				writeMemoryFile(t, dir, "keep_this", "keep_this", "Should remain", "user", "Keep me")
				buildIndex(t, dir)
			},
			verifyFunc: func(t *testing.T, dir string) {
				// Deleted file gone
				if _, err := os.Stat(filepath.Join(dir, "to_delete.md")); !os.IsNotExist(err) {
					t.Error("expected to_delete.md to be deleted")
				}
				// Kept file still there
				if _, err := os.Stat(filepath.Join(dir, "keep_this.md")); err != nil {
					t.Error("expected keep_this.md to still exist")
				}
				// Index updated
				data, _ := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
				content := string(data)
				if strings.Contains(content, "to_delete") {
					t.Error("expected MEMORY.md to not contain to_delete")
				}
				if !strings.Contains(content, "keep_this") {
					t.Error("expected MEMORY.md to still contain keep_this")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dir string
			if tt.memDir == "temp" {
				dir = createTempMemoryDir(t)
			} else {
				dir = tt.memDir
			}

			if tt.setupFunc != nil {
				tt.setupFunc(t, dir)
			}

			tool := NewDeleteMemoryTool(dir)
			toolCtx := newTestToolCtx(t)
			result := tool.Call(tt.args, toolCtx)

			if result.Ok != tt.wantOk {
				t.Errorf("Ok = %v, want %v", result.Ok, tt.wantOk)
			}
			if result.IsError != tt.wantErr {
				t.Errorf("IsError = %v, want %v", result.IsError, tt.wantErr)
			}
			if !strings.Contains(result.Content, tt.wantContain) {
				t.Errorf("Content = %q, want to contain %q", result.Content, tt.wantContain)
			}
			if tt.verifyFunc != nil {
				tt.verifyFunc(t, dir)
			}
		})
	}
}
