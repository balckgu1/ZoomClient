package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchMemoryTool_Name(t *testing.T) {
	tool := NewSearchMemoryTool("")
	if tool.Name() != "search_memory" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "search_memory")
	}
}

func TestSearchMemoryTool_Call(t *testing.T) {
	tests := []struct {
		name        string
		memDir      string // "temp" means create temp dir
		args        map[string]any
		wantOk      bool
		wantErr     bool
		wantContain string
		setupFunc   func(t *testing.T, dir string)
	}{
		{
			name:        "missing keyword",
			memDir:      "temp",
			args:        map[string]any{},
			wantOk:      false,
			wantErr:     true,
			wantContain: "keyword parameter is required",
		},
		{
			name:        "keyword not string",
			memDir:      "temp",
			args:        map[string]any{"keyword": 42},
			wantOk:      false,
			wantErr:     true,
			wantContain: "keyword parameter must be a non-empty string",
		},
		{
			name:        "keyword is empty",
			memDir:      "temp",
			args:        map[string]any{"keyword": "  "},
			wantOk:      false,
			wantErr:     true,
			wantContain: "keyword parameter must be a non-empty string",
		},
		{
			name:        "empty memoryDir",
			memDir:      "",
			args:        map[string]any{"keyword": "test"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "MemoryDir is not configured",
		},
		{
			name:        "no matches found",
			memDir:      "temp",
			args:        map[string]any{"keyword": "nonexistent"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "No memories found",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "test", "test", "a test", "user", "body content")
				buildIndex(t, dir)
			},
		},
		{
			name:        "match by name",
			memDir:      "temp",
			args:        map[string]any{"keyword": "redis"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "redis_config",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "redis_config", "redis_config", "Redis settings", "project", "host=localhost")
				writeMemoryFile(t, dir, "other", "other", "other memory", "user", "unrelated")
				buildIndex(t, dir)
			},
		},
		{
			name:        "match by description",
			memDir:      "temp",
			args:        map[string]any{"keyword": "database"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "db_conn",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "db_conn", "db_conn", "Database connection string", "project", "postgres://localhost")
				buildIndex(t, dir)
			},
		},
		{
			name:        "match by body (searches name and description only)",
			memDir:      "temp",
			args:        map[string]any{"keyword": "pytest"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "No memories found",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "test_framework", "test_framework", "Test preferences", "user", "Use pytest for testing")
				buildIndex(t, dir)
			},
		},
		{
			name:        "case insensitive search",
			memDir:      "temp",
			args:        map[string]any{"keyword": "REDIS"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "redis_config",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "redis_config", "redis_config", "Redis settings", "project", "host=localhost")
				buildIndex(t, dir)
			},
		},
		{
			name:        "skips MEMORY.md index content from search",
			memDir:      "temp",
			args:        map[string]any{"keyword": "zzz_no_match"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "No memories found",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "real", "real", "A real memory", "user", "body")
				buildIndex(t, dir)
			},
		},
		{
			name:        "multiple matches",
			memDir:      "temp",
			args:        map[string]any{"keyword": "go"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "Found 2 memory(ies)",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "go_style", "go_style", "Go style guide", "project", "Use gofmt")
				writeMemoryFile(t, dir, "go_mod", "go_mod", "Go module config", "project", "go 1.21")
				writeMemoryFile(t, dir, "python", "python", "Python notes", "user", "pip install")
				buildIndex(t, dir)
			},
		},
		{
			name:        "full_text matches body",
			memDir:      "temp",
			args:        map[string]any{"keyword": "pytest", "full_text": true},
			wantOk:      true,
			wantErr:     false,
			wantContain: "test_framework",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "test_framework", "test_framework", "Test preferences", "user", "Use pytest for testing")
				buildIndex(t, dir)
			},
		},
		{
			name:        "full_text still matches name",
			memDir:      "temp",
			args:        map[string]any{"keyword": "redis", "full_text": true},
			wantOk:      true,
			wantErr:     false,
			wantContain: "redis_config",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "redis_config", "redis_config", "Redis settings", "project", "host=localhost")
				buildIndex(t, dir)
			},
		},
		{
			name:        "full_text wrong type defaults to false",
			memDir:      "temp",
			args:        map[string]any{"keyword": "pytest", "full_text": "true"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "No memories found",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "test_framework", "test_framework", "Test preferences", "user", "Use pytest for testing")
				buildIndex(t, dir)
			},
		},
		{
			name:        "full_text with missing body file returns summary",
			memDir:      "temp",
			args:        map[string]any{"keyword": "orphan", "full_text": true},
			wantOk:      true,
			wantErr:     false,
			wantContain: "orphan",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "orphan", "orphan", "Orphan memory", "user", "body text")
				buildIndex(t, dir)
				// 删除正文文件，模拟索引项对应文件缺失
				if err := os.Remove(filepath.Join(dir, "orphan.md")); err != nil {
					t.Fatalf("failed to remove orphan.md: %v", err)
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

			tool := NewSearchMemoryTool(dir)
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
		})
	}
}

// TestSearchMemoryTool_BodyTruncation 验证正文按 MaxBodyChars（15KB）截断的行为。
func TestSearchMemoryTool_BodyTruncation(t *testing.T) {
	toolCtx := newTestToolCtx(t)

	t.Run("body over limit is truncated with note", func(t *testing.T) {
		dir := createTempMemoryDir(t)
		longBody := strings.Repeat("a", MaxBodyChars+1)
		writeMemoryFile(t, dir, "big", "big", "A big memory", "user", longBody)
		buildIndex(t, dir)

		tool := NewSearchMemoryTool(dir)
		res := tool.Call(map[string]any{"keyword": "big"}, toolCtx)
		if !res.Ok || res.IsError {
			t.Fatalf("expected ok result, got Ok=%v IsError=%v", res.Ok, res.IsError)
		}
		if !strings.Contains(res.Content, "[truncated at 15KB") {
			t.Errorf("expected truncation note, got %q", res.Content)
		}
		if !strings.Contains(res.Content, "read_file") {
			t.Errorf("expected read_file hint in note, got %q", res.Content)
		}
	})

	t.Run("body exactly at limit is not truncated", func(t *testing.T) {
		dir := createTempMemoryDir(t)
		exactBody := strings.Repeat("a", MaxBodyChars)
		writeMemoryFile(t, dir, "exact", "exact", "Exact size memory", "user", exactBody)
		buildIndex(t, dir)

		tool := NewSearchMemoryTool(dir)
		res := tool.Call(map[string]any{"keyword": "exact"}, toolCtx)
		if !res.Ok || res.IsError {
			t.Fatalf("expected ok result, got Ok=%v IsError=%v", res.Ok, res.IsError)
		}
		if strings.Contains(res.Content, "[truncated") {
			t.Errorf("expected no truncation at exact limit, got %q", res.Content)
		}
	})
}
