package tools

import (
	"os"
	"path/filepath"
	"testing"
)

// ========== ReadFileTool Basic Properties ==========

// TestReadFileTool_Name verifies the tool name is correct.
func TestReadFileTool_Name(t *testing.T) {
	tool := ReadFileTool{}
	if got := tool.Name(); got != "read_file" {
		t.Errorf("tool.Name() = %q, want %q", got, "read_file")
	}
}

// TestReadFileTool_Description verifies the description is not empty.
func TestReadFileTool_Description(t *testing.T) {
	tool := ReadFileTool{}
	if desc := tool.Description(); desc == "" {
		t.Error("tool.Description() should not be empty")
	}
}

// TestReadFileTool_Parameters verifies the parameter schema is correct.
func TestReadFileTool_Parameters(t *testing.T) {
	tool := ReadFileTool{}
	params := tool.Parameters()

	if params["type"] != "object" {
		t.Errorf("params type = %q, want %q", params["type"], "object")
	}

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("params.properties missing or wrong type")
	}
	if _, exists := props["filename"]; !exists {
		t.Error("params.properties missing required key: filename")
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("params.required missing or wrong type")
	}
	found := false
	for _, r := range required {
		if r == "filename" {
			found = true
			break
		}
	}
	if !found {
		t.Error("params.required should contain 'filename'")
	}
}

// ========== ReadFileTool.Call Functional Tests ==========

// TestReadFileTool_Call_Success verifies reading a file successfully.
func TestReadFileTool_Call_Success(t *testing.T) {
	workDir := t.TempDir()
	tool := ReadFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	testFile := filepath.Join(workDir, "sample.txt")
	if err := os.WriteFile(testFile, []byte("Hello from read test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	args := map[string]any{"filename": "sample.txt"}
	result := tool.Call(args, ctx)

	if !result.Ok {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if result.IsError {
		t.Error("success case should have IsError = false")
	}
	if result.Content != "Hello from read test" {
		t.Errorf("content = %q, want %q", result.Content, "Hello from read test")
	}
}

// TestReadFileTool_Call_FileNotFound verifies error when the file does not exist.
func TestReadFileTool_Call_FileNotFound(t *testing.T) {
	workDir := t.TempDir()
	tool := ReadFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	args := map[string]any{"filename": "nonexistent.txt"}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure for nonexistent file")
	}
	if !result.IsError {
		t.Error("expected IsError = true for nonexistent file")
	}
}

// TestReadFileTool_Call_MissingFilename verifies error when filename argument is missing.
func TestReadFileTool_Call_MissingFilename(t *testing.T) {
	workDir := t.TempDir()
	tool := ReadFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	args := map[string]any{}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure when filename is missing")
	}
	if !result.IsError {
		t.Error("expected IsError = true when filename is missing")
	}
}

// TestReadFileTool_Call_EmptyFilename verifies error when filename is an empty string.
func TestReadFileTool_Call_EmptyFilename(t *testing.T) {
	workDir := t.TempDir()
	tool := ReadFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	args := map[string]any{"filename": ""}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure when filename is empty")
	}
	if !result.IsError {
		t.Error("expected IsError = true when filename is empty")
	}
}

// TestReadFileTool_Call_FilenameWrongType verifies error when filename is not a string.
func TestReadFileTool_Call_FilenameWrongType(t *testing.T) {
	workDir := t.TempDir()
	tool := ReadFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	args := map[string]any{"filename": 12345}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure when filename type is wrong")
	}
	if !result.IsError {
		t.Error("expected IsError = true when filename type is wrong")
	}
}

// TestReadFileTool_Call_PathTraversal verifies path traversal attempts are blocked.
func TestReadFileTool_Call_PathTraversal(t *testing.T) {
	workDir := t.TempDir()
	tool := ReadFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	args := map[string]any{"filename": "../../etc/passwd"}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure for path traversal")
	}
	if !result.IsError {
		t.Error("expected IsError = true for path traversal")
	}
}

// TestReadFileTool_Call_AbsolutePathEscape verifies absolute paths outside workDir are blocked.
func TestReadFileTool_Call_AbsolutePathEscape(t *testing.T) {
	workDir := t.TempDir()
	tool := ReadFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	outsideFile := filepath.Join(os.TempDir(), "secret.txt")
	args := map[string]any{"filename": outsideFile}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure for absolute path escape")
	}
	if !result.IsError {
		t.Error("expected IsError = true for absolute path escape")
	}
}

// TestReadFileTool_Call_UnicodeContent verifies reading files with Unicode content.
func TestReadFileTool_Call_UnicodeContent(t *testing.T) {
	workDir := t.TempDir()
	tool := ReadFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	content := "你好世界 🌍\nUnicode test content"
	testFile := filepath.Join(workDir, "unicode.txt")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	args := map[string]any{"filename": "unicode.txt"}
	result := tool.Call(args, ctx)

	if !result.Ok {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if result.Content != content {
		t.Errorf("content = %q, want %q", result.Content, content)
	}
}

// TestReadFileTool_Call_LargeFile verifies reading a large file.
func TestReadFileTool_Call_LargeFile(t *testing.T) {
	workDir := t.TempDir()
	tool := ReadFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	largeContent := make([]byte, 1024*1024) // 1 MB
	for i := range largeContent {
		largeContent[i] = byte('A' + (i % 26))
	}

	testFile := filepath.Join(workDir, "large.txt")
	if err := os.WriteFile(testFile, largeContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	args := map[string]any{"filename": "large.txt"}
	result := tool.Call(args, ctx)

	if !result.Ok {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if len(result.Content) != len(largeContent) {
		t.Errorf("content length = %d, want %d", len(result.Content), len(largeContent))
	}
}

// ========== Registry Integration ==========

// TestReadFileTool_ViaRegistry verifies execution through the Registry.
func TestReadFileTool_ViaRegistry(t *testing.T) {
	workDir := t.TempDir()
	registry := NewRegistry()
	registry.Register(ReadFileTool{})
	ctx := &ToolContext{WorkPath: workDir}

	testFile := filepath.Join(workDir, "registry_read.txt")
	if err := os.WriteFile(testFile, []byte("via registry"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	args := map[string]any{"filename": "registry_read.txt"}
	result := registry.RunTool("read_file", args, ctx)

	if !result.Ok {
		t.Fatalf("expected success via registry, got error: %s", result.Content)
	}
	if result.Content != "via registry" {
		t.Errorf("content = %q, want %q", result.Content, "via registry")
	}
}
