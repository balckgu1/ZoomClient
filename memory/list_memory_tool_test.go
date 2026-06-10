package memory

import (
	"strings"
	"testing"
)

func TestListMemoryTool_Name(t *testing.T) {
	tool := NewListMemoryTool("")
	if tool.Name() != "list_memory" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "list_memory")
	}
}

func TestListMemoryTool_Call(t *testing.T) {
	tests := []struct {
		name        string
		memDir      string // "temp" means create temp dir
		wantOk      bool
		wantErr     bool
		wantContain string
		setupFunc   func(t *testing.T, dir string)
	}{
		{
			name:        "empty memoryDir",
			memDir:      "",
			wantOk:      false,
			wantErr:     true,
			wantContain: "MemoryDir is not configured",
		},
		{
			name:        "MEMORY.md does not exist",
			memDir:      "temp",
			wantOk:      false,
			wantErr:     true,
			wantContain: "failed to read MEMORY.md",
		},
		{
			name:        "success - empty index",
			memDir:      "temp",
			wantOk:      true,
			wantErr:     false,
			wantContain: "# Memory Index",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFileRaw(t, dir, "MEMORY.md", "# Memory Index\n")
			},
		},
		{
			name:        "success - index with entries",
			memDir:      "temp",
			wantOk:      true,
			wantErr:     false,
			wantContain: "prefer_tabs",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "prefer_tabs", "prefer_tabs", "Use tabs", "user", "tabs content")
				writeMemoryFile(t, dir, "no_secrets", "no_secrets", "No secrets in code", "feedback", "never commit keys")
				buildIndex(t, dir)
			},
		},
		{
			name:        "success - multiple entries listed",
			memDir:      "temp",
			wantOk:      true,
			wantErr:     false,
			wantContain: "mem2",
			setupFunc: func(t *testing.T, dir string) {
				writeMemoryFile(t, dir, "mem1", "mem1", "First memory", "user", "body1")
				writeMemoryFile(t, dir, "mem2", "mem2", "Second memory", "project", "body2")
				buildIndex(t, dir)
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

			tool := NewListMemoryTool(dir)
			toolCtx := newTestToolCtx(t)
			result := tool.Call(map[string]any{}, toolCtx)

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
