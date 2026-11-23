package tools

import (
	"os"
	"path/filepath"
	"testing"
)

// ========== ReadFileTool 基本属性 ==========

// TestReadFileTool_Name 验证工具名称是否正确。
func TestReadFileTool_Name(t *testing.T) {
	tool := ReadFileTool{}
	if got := tool.Name(); got != "read_file" {
		t.Errorf("tool.Name() = %q, want %q", got, "read_file")
	}
}

// TestReadFileTool_Description 验证描述不为空。
func TestReadFileTool_Description(t *testing.T) {
	tool := ReadFileTool{}
	if desc := tool.Description(); desc == "" {
		t.Error("tool.Description() should not be empty")
	}
}

// TestReadFileTool_Parameters 验证参数模式是否正确。
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

// ========== ReadFileTool.Call 功能测试 ==========

// TestReadFileTool_Call_Success 验证成功读取文件。
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

// TestReadFileTool_Call_FileNotFound 验证文件不存在时的错误。
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

// TestReadFileTool_Call_MissingFilename 验证缺少 filename 参数时的错误。
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

// TestReadFileTool_Call_EmptyFilename 验证 filename 为空字符串时的错误。
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

// TestReadFileTool_Call_FilenameWrongType 验证 filename 不是字符串时的错误。
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

// TestReadFileTool_Call_PathTraversal 验证路径遍历尝试被阻止。
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

// TestReadFileTool_Call_AbsolutePathEscape 验证 workDir 之外的绝对路径被阻止。
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

// TestReadFileTool_Call_UnicodeContent 验证读取包含 Unicode 内容的文件。
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

// TestReadFileTool_Call_LargeFile 验证读取大文件。
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

// ========== Registry 集成 ==========

// TestReadFileTool_ViaRegistry 验证通过 Registry 执行。
func TestReadFileTool_ViaRegistry(t *testing.T) {
	workDir := t.TempDir()
	registry := NewToolRegister()
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
