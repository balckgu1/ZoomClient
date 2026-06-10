package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveMemoryTool_Name(t *testing.T) {
	tool := NewSaveMemoryTool("")
	if tool.Name() != "save_memory" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "save_memory")
	}
}

func TestSaveMemoryTool_Call(t *testing.T) {
	tests := []struct {
		name        string
		memDir      string // "" means use empty string; "temp" means create temp dir
		args        map[string]any
		wantOk      bool
		wantErr     bool
		wantContain string                         // substring that Content should contain
		setupFunc   func(t *testing.T, dir string) // optional setup before Call
		verifyFunc  func(t *testing.T, dir string) // optional verify after Call
	}{
		{
			name:        "missing name param",
			memDir:      "temp",
			args:        map[string]any{"description": "desc", "type": "user", "content": "body"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "name parameter is required",
		},
		{
			name:        "name is not string",
			memDir:      "temp",
			args:        map[string]any{"name": 123, "description": "desc", "type": "user", "content": "body"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "name parameter must be a non-empty string",
		},
		{
			name:        "name is empty",
			memDir:      "temp",
			args:        map[string]any{"name": "  ", "description": "desc", "type": "user", "content": "body"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "name parameter must be a non-empty string",
		},
		{
			name:        "missing description param",
			memDir:      "temp",
			args:        map[string]any{"name": "test", "type": "user", "content": "body"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "description parameter is required",
		},
		{
			name:        "description is empty",
			memDir:      "temp",
			args:        map[string]any{"name": "test", "description": "", "type": "user", "content": "body"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "description parameter must be a non-empty string",
		},
		{
			name:        "missing type param",
			memDir:      "temp",
			args:        map[string]any{"name": "test", "description": "desc", "content": "body"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "type parameter is required",
		},
		{
			name:        "invalid type param",
			memDir:      "temp",
			args:        map[string]any{"name": "test", "description": "desc", "type": "invalid", "content": "body"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "type must be one of",
		},
		{
			name:        "missing content param",
			memDir:      "temp",
			args:        map[string]any{"name": "test", "description": "desc", "type": "user"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "content parameter is required",
		},
		{
			name:        "empty memoryDir",
			memDir:      "",
			args:        map[string]any{"name": "test", "description": "desc", "type": "user", "content": "body"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "MemoryDir is not configured",
		},
		{
			name:        "duplicate name",
			memDir:      "temp",
			args:        map[string]any{"name": "dup", "description": "desc", "type": "user", "content": "body"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "already exists",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "dup", "dup", "existing", "user", "old body")
				buildIndex(t, dir)
			},
		},
		{
			name:        "success - basic save",
			memDir:      "temp",
			args:        map[string]any{"name": "my_memory", "description": "A test memory", "type": "user", "content": "Hello World"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "Memory saved successfully",
			verifyFunc: func(t *testing.T, dir string) {
				data, err := os.ReadFile(filepath.Join(dir, "my_memory.md"))
				if err != nil {
					t.Fatalf("expected file to exist: %v", err)
				}
				content := string(data)
				if !strings.Contains(content, "name: my_memory") {
					t.Error("expected frontmatter to contain name: my_memory")
				}
				if !strings.Contains(content, "Hello World") {
					t.Error("expected body to contain Hello World")
				}
				// MEMORY.md should exist
				if _, err := os.Stat(filepath.Join(dir, "MEMORY.md")); err != nil {
					t.Error("expected MEMORY.md index to be created")
				}
			},
		},
		{
			name:        "success - description with colon is quoted",
			memDir:      "temp",
			args:        map[string]any{"name": "db_cfg", "description": "DB连接: host:port", "type": "project", "content": "config details"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "Memory saved successfully",
			verifyFunc: func(t *testing.T, dir string) {
				data, err := os.ReadFile(filepath.Join(dir, "db_cfg.md"))
				if err != nil {
					t.Fatalf("expected file to exist: %v", err)
				}
				content := string(data)
				if !strings.Contains(content, `"DB连接: host:port"`) {
					t.Errorf("expected description to be quoted, got:\n%s", content)
				}
				// Verify roundtrip: parse it back
				doc := ParseFrontMatter(content)
				if doc.FrontMatter.Description != "DB连接: host:port" {
					t.Errorf("roundtrip description mismatch: got %q", doc.FrontMatter.Description)
				}
			},
		},
		{
			name:        "success - all four types",
			memDir:      "temp",
			args:        map[string]any{"name": "ref_link", "description": "docs link", "type": "reference", "content": "https://example.com"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "Memory saved successfully",
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

			tool := NewSaveMemoryTool(dir)
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

// -----------------------------------------------------------------------
// TestSanitizeFilename
// -----------------------------------------------------------------------

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal name", "my_memory", "my_memory"},
		{"with spaces", "my memory", "my_memory"},
		{"path traversal", "../../etc/passwd", "passwd"},
		{"special chars", "file:name*test?", "file_name_test"},
		{"dots", "file.name.ext", "file_name_ext"},
		{"empty after sanitize", "***", "unnamed_memory"},
		{"Windows reserved CON", "CON", "_CON"},
		{"Windows reserved NUL", "NUL", "_NUL"},
		{"Windows reserved COM1", "COM1", "_COM1"},
		{"Windows reserved lowercase", "con", "_con"},
		{"consecutive underscores", "a___b", "a_b"},
		{"long name", strings.Repeat("a", 200), strings.Repeat("a", 130)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
