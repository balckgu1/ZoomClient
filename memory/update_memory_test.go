package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------
// TestUpdateMemoryTool_Name
// -----------------------------------------------------------------------

func TestUpdateMemoryTool_Name(t *testing.T) {
	tool := NewUpdateMemoryTool("")
	if tool.Name() != "update_memory" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "update_memory")
	}
}

// -----------------------------------------------------------------------
// TestUpdateMemoryTool_Call
// -----------------------------------------------------------------------

func TestUpdateMemoryTool_Call(t *testing.T) {
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
			args:        map[string]any{"content": "new body"},
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
			name:        "content is empty string",
			memDir:      "temp",
			args:        map[string]any{"name": "test", "content": "  "},
			wantOk:      false,
			wantErr:     true,
			wantContain: "content parameter must be a non-empty string",
		},
		{
			name:        "description is empty string",
			memDir:      "temp",
			args:        map[string]any{"name": "test", "description": "  "},
			wantOk:      false,
			wantErr:     true,
			wantContain: "description parameter must be a non-empty string",
		},
		{
			name:        "type is empty string",
			memDir:      "temp",
			args:        map[string]any{"name": "test", "type": "  "},
			wantOk:      false,
			wantErr:     true,
			wantContain: "type parameter must be a non-empty string",
		},
		{
			name:        "invalid type param",
			memDir:      "temp",
			args:        map[string]any{"name": "test", "type": "badtype"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "type must be one of",
		},
		{
			name:        "empty memoryDir",
			memDir:      "",
			args:        map[string]any{"name": "test", "content": "body"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "MemoryDir is not configured",
		},
		{
			name:        "memory not in index",
			memDir:      "temp",
			args:        map[string]any{"name": "nonexistent", "content": "body"},
			wantOk:      false,
			wantErr:     true,
			wantContain: "does not exist",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFileRaw(t, dir, "MEMORY.md", "# Memory Index\n- other: something [user]\n")
			},
		},
		{
			name:        "success - update content only",
			memDir:      "temp",
			args:        map[string]any{"name": "my_mem", "content": "New body content"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "updated successfully",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "my_mem", "my_mem", "Old description", "user", "Old body")
				buildIndex(t, dir)
			},
			verifyFunc: func(t *testing.T, dir string) {
				data, err := os.ReadFile(filepath.Join(dir, "my_mem.md"))
				if err != nil {
					t.Fatalf("failed to read file: %v", err)
				}
				doc := ParseFrontMatter(string(data))
				if doc.Body != "New body content" {
					t.Errorf("Body = %q, want %q", doc.Body, "New body content")
				}
				if doc.FrontMatter.Description != "Old description" {
					t.Errorf("Description should be retained, got %q", doc.FrontMatter.Description)
				}
				if doc.FrontMatter.Type != "user" {
					t.Errorf("Type should be retained, got %q", doc.FrontMatter.Type)
				}
			},
		},
		{
			name:        "success - update description only",
			memDir:      "temp",
			args:        map[string]any{"name": "my_mem", "description": "New description"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "updated successfully",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "my_mem", "my_mem", "Old description", "user", "Old body")
				buildIndex(t, dir)
			},
			verifyFunc: func(t *testing.T, dir string) {
				data, _ := os.ReadFile(filepath.Join(dir, "my_mem.md"))
				doc := ParseFrontMatter(string(data))
				if doc.FrontMatter.Description != "New description" {
					t.Errorf("Description = %q, want %q", doc.FrontMatter.Description, "New description")
				}
				if doc.Body != "Old body" {
					t.Errorf("Body should be retained, got %q", doc.Body)
				}
			},
		},
		{
			name:        "success - update type only",
			memDir:      "temp",
			args:        map[string]any{"name": "my_mem", "type": "feedback"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "updated successfully",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "my_mem", "my_mem", "A description", "user", "Some body")
				buildIndex(t, dir)
			},
			verifyFunc: func(t *testing.T, dir string) {
				data, _ := os.ReadFile(filepath.Join(dir, "my_mem.md"))
				doc := ParseFrontMatter(string(data))
				if doc.FrontMatter.Type != "feedback" {
					t.Errorf("Type = %q, want %q", doc.FrontMatter.Type, "feedback")
				}
			},
		},
		{
			name:        "success - update all fields",
			memDir:      "temp",
			args:        map[string]any{"name": "my_mem", "content": "Updated body", "description": "Updated desc", "type": "project"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "updated successfully",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "my_mem", "my_mem", "Original", "user", "Original body")
				buildIndex(t, dir)
			},
			verifyFunc: func(t *testing.T, dir string) {
				data, _ := os.ReadFile(filepath.Join(dir, "my_mem.md"))
				doc := ParseFrontMatter(string(data))
				if doc.Body != "Updated body" {
					t.Errorf("Body = %q, want %q", doc.Body, "Updated body")
				}
				if doc.FrontMatter.Description != "Updated desc" {
					t.Errorf("Description = %q, want %q", doc.FrontMatter.Description, "Updated desc")
				}
				if doc.FrontMatter.Type != "project" {
					t.Errorf("Type = %q, want %q", doc.FrontMatter.Type, "project")
				}
			},
		},
		{
			name:        "success - index rebuilt after update",
			memDir:      "temp",
			args:        map[string]any{"name": "my_mem", "description": "Brand new desc"},
			wantOk:      true,
			wantErr:     false,
			wantContain: "updated successfully",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "my_mem", "my_mem", "Old desc", "user", "body")
				buildIndex(t, dir)
			},
			verifyFunc: func(t *testing.T, dir string) {
				data, _ := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
				if !strings.Contains(string(data), "Brand new desc") {
					t.Error("expected MEMORY.md to contain updated description")
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

			tool := NewUpdateMemoryTool(dir)
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
// TestMemoryExists
// -----------------------------------------------------------------------

func TestMemoryExists(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		searchFor string
		wantExist bool
		wantErr   bool
	}{
		{
			name:      "name found in index",
			content:   "# Memory Index\n- prefer_tabs: Use tabs [user]\n- no_secrets: No secrets [feedback]\n",
			searchFor: "prefer_tabs",
			wantExist: true,
			wantErr:   false,
		},
		{
			name:      "name not found",
			content:   "# Memory Index\n- other: something [user]\n",
			searchFor: "missing",
			wantExist: false,
			wantErr:   false,
		},
		{
			name:      "partial name should not match",
			content:   "# Memory Index\n- prefer_tabs: Use tabs [user]\n",
			searchFor: "prefer",
			wantExist: false,
			wantErr:   false,
		},
		{
			name:      "empty index",
			content:   "# Memory Index\n",
			searchFor: "anything",
			wantExist: false,
			wantErr:   false,
		},
		{
			name:      "name with special chars",
			content:   "# Memory Index\n- db_config: DB连接 [project]\n",
			searchFor: "db_config",
			wantExist: true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := createTempMemoryDir(t)
			indexPath := filepath.Join(dir, "MEMORY.md")
			if err := os.WriteFile(indexPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write MEMORY.md: %v", err)
			}

			got, err := MemoryExists(indexPath, tt.searchFor)
			if (err != nil) != tt.wantErr {
				t.Errorf("MemoryExists() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantExist {
				t.Errorf("MemoryExists() = %v, want %v", got, tt.wantExist)
			}
		})
	}
}

func TestMemoryExists_FileNotFound(t *testing.T) {
	_, err := MemoryExists("/nonexistent/path/MEMORY.md", "test")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
