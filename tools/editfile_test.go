package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ========== EditFileTool Basic Properties ==========

// TestEditFileTool_Name verifies the tool name is correct.
func TestEditFileTool_Name(t *testing.T) {
	tool := EditFileTool{}
	if got := tool.Name(); got != "edit_file" {
		t.Errorf("tool.Name() = %q, want %q", got, "edit_file")
	}
}

// TestEditFileTool_Description verifies the description is not empty.
func TestEditFileTool_Description(t *testing.T) {
	tool := EditFileTool{}
	if desc := tool.Description(); desc == "" {
		t.Error("tool.Description() should not be empty")
	}
}

// TestEditFileTool_Parameters verifies the parameter schema is correct.
func TestEditFileTool_Parameters(t *testing.T) {
	tool := EditFileTool{}
	params := tool.Parameters()

	if params["type"] != "object" {
		t.Errorf("params type = %q, want %q", params["type"], "object")
	}

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("params.properties missing or wrong type")
	}
	for _, key := range []string{"filename", "content"} {
		if _, exists := props[key]; !exists {
			t.Errorf("params.properties missing required key: %s", key)
		}
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("params.required missing or wrong type")
	}
	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r] = true
	}
	for _, key := range []string{"filename", "content"} {
		if !requiredSet[key] {
			t.Errorf("params.required should contain '%s'", key)
		}
	}
}

// ========== EditFileTool.Call Functional Tests ==========

// TestEditFileTool_Call_Success verifies editing an existing file.
func TestEditFileTool_Call_Success(t *testing.T) {
	workDir := t.TempDir()
	tool := EditFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	testFile := filepath.Join(workDir, "edit.txt")
	if err := os.WriteFile(testFile, []byte("old content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	args := map[string]any{
		"filename": "edit.txt",
		"content":  "new content",
	}
	result := tool.Call(args, ctx)

	if !result.Ok {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if result.IsError {
		t.Error("success case should have IsError = false")
	}

	// Verify file content was updated.
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file after edit: %v", err)
	}
	if string(data) != "new content" {
		t.Errorf("file content = %q, want %q", string(data), "new content")
	}
}

// TestEditFileTool_Call_FileNotExist verifies error when the file does not exist.
func TestEditFileTool_Call_FileNotExist(t *testing.T) {
	workDir := t.TempDir()
	tool := EditFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	args := map[string]any{
		"filename": "nonexistent.txt",
		"content":  "some content",
	}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure for nonexistent file")
	}
	if !result.IsError {
		t.Error("expected IsError = true for nonexistent file")
	}
	if !strings.Contains(result.Content, "does not exist") {
		t.Errorf("error message should mention file does not exist, got: %s", result.Content)
	}
}

// TestEditFileTool_Call_MissingFilename verifies error when filename argument is missing.
func TestEditFileTool_Call_MissingFilename(t *testing.T) {
	workDir := t.TempDir()
	tool := EditFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	args := map[string]any{
		"content": "some content",
	}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure when filename is missing")
	}
	if !result.IsError {
		t.Error("expected IsError = true when filename is missing")
	}
}

// TestEditFileTool_Call_EmptyFilename verifies error when filename is an empty string.
func TestEditFileTool_Call_EmptyFilename(t *testing.T) {
	workDir := t.TempDir()
	tool := EditFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	args := map[string]any{
		"filename": "",
		"content":  "some content",
	}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure when filename is empty")
	}
	if !result.IsError {
		t.Error("expected IsError = true when filename is empty")
	}
}

// TestEditFileTool_Call_FilenameWrongType verifies error when filename is not a string.
func TestEditFileTool_Call_FilenameWrongType(t *testing.T) {
	workDir := t.TempDir()
	tool := EditFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	args := map[string]any{
		"filename": 12345,
		"content":  "some content",
	}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure when filename type is wrong")
	}
	if !result.IsError {
		t.Error("expected IsError = true when filename type is wrong")
	}
}

// TestEditFileTool_Call_MissingContent verifies error when content argument is missing.
func TestEditFileTool_Call_MissingContent(t *testing.T) {
	workDir := t.TempDir()
	tool := EditFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	testFile := filepath.Join(workDir, "missing_content.txt")
	if err := os.WriteFile(testFile, []byte("old"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	args := map[string]any{
		"filename": "missing_content.txt",
	}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure when content is missing")
	}
	if !result.IsError {
		t.Error("expected IsError = true when content is missing")
	}
}

// TestEditFileTool_Call_EmptyContent verifies error when content is an empty string.
func TestEditFileTool_Call_EmptyContent(t *testing.T) {
	workDir := t.TempDir()
	tool := EditFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	testFile := filepath.Join(workDir, "empty_content.txt")
	if err := os.WriteFile(testFile, []byte("old"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	args := map[string]any{
		"filename": "empty_content.txt",
		"content":  "",
	}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure when content is empty")
	}
	if !result.IsError {
		t.Error("expected IsError = true when content is empty")
	}
}

// TestEditFileTool_Call_ContentWrongType verifies error when content is not a string.
func TestEditFileTool_Call_ContentWrongType(t *testing.T) {
	workDir := t.TempDir()
	tool := EditFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	testFile := filepath.Join(workDir, "wrong_type.txt")
	if err := os.WriteFile(testFile, []byte("old"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	args := map[string]any{
		"filename": "wrong_type.txt",
		"content":  42,
	}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure when content type is wrong")
	}
	if !result.IsError {
		t.Error("expected IsError = true when content type is wrong")
	}
}

// TestEditFileTool_Call_PathTraversal verifies path traversal attempts are blocked.
func TestEditFileTool_Call_PathTraversal(t *testing.T) {
	workDir := t.TempDir()
	tool := EditFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	args := map[string]any{
		"filename": "../../etc/passwd",
		"content":  "malicious",
	}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure for path traversal")
	}
	if !result.IsError {
		t.Error("expected IsError = true for path traversal")
	}
}

// TestEditFileTool_Call_AbsolutePathEscape verifies absolute paths outside workDir are blocked.
func TestEditFileTool_Call_AbsolutePathEscape(t *testing.T) {
	workDir := t.TempDir()
	tool := EditFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	outsideFile := filepath.Join(os.TempDir(), "secret.txt")
	args := map[string]any{
		"filename": outsideFile,
		"content":  "malicious",
	}
	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("expected failure for absolute path escape")
	}
	if !result.IsError {
		t.Error("expected IsError = true for absolute path escape")
	}
}

// TestEditFileTool_Call_UnicodeContent verifies editing with Unicode content.
func TestEditFileTool_Call_UnicodeContent(t *testing.T) {
	workDir := t.TempDir()
	tool := EditFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	testFile := filepath.Join(workDir, "unicode_edit.txt")
	if err := os.WriteFile(testFile, []byte("old"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	unicodeContent := "你好世界 🌍\nUnicode edit content"
	args := map[string]any{
		"filename": "unicode_edit.txt",
		"content":  unicodeContent,
	}
	result := tool.Call(args, ctx)

	if !result.Ok {
		t.Fatalf("expected success, got error: %s", result.Content)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file after edit: %v", err)
	}
	if string(data) != unicodeContent {
		t.Errorf("file content = %q, want %q", string(data), unicodeContent)
	}
}

// TestEditFileTool_Call_LargeContent verifies editing with a large content.
func TestEditFileTool_Call_LargeContent(t *testing.T) {
	workDir := t.TempDir()
	tool := EditFileTool{}
	ctx := &ToolContext{WorkPath: workDir}

	testFile := filepath.Join(workDir, "large_edit.txt")
	if err := os.WriteFile(testFile, []byte("old"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	largeContent := make([]byte, 1024*1024) // 1 MB
	for i := range largeContent {
		largeContent[i] = byte('A' + (i % 26))
	}

	args := map[string]any{
		"filename": "large_edit.txt",
		"content":  string(largeContent),
	}
	result := tool.Call(args, ctx)

	if !result.Ok {
		t.Fatalf("expected success, got error: %s", result.Content)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file after edit: %v", err)
	}
	if len(data) != len(largeContent) {
		t.Errorf("file size = %d, want %d", len(data), len(largeContent))
	}
}

// ========== Registry Integration ==========

// TestEditFileTool_ViaRegistry verifies execution through the Registry.
func TestEditFileTool_ViaRegistry(t *testing.T) {
	workDir := t.TempDir()
	registry := NewRegistry()
	registry.Register(EditFileTool{})
	ctx := &ToolContext{WorkPath: workDir}

	testFile := filepath.Join(workDir, "registry_edit.txt")
	if err := os.WriteFile(testFile, []byte("old registry"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	args := map[string]any{
		"filename": "registry_edit.txt",
		"content":  "via registry",
	}
	result := registry.RunTool("edit_file", args, ctx)

	if !result.Ok {
		t.Fatalf("expected success via registry, got error: %s", result.Content)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file after registry edit: %v", err)
	}
	if string(data) != "via registry" {
		t.Errorf("file content = %q, want %q", string(data), "via registry")
	}
}
