package tools

import (
	"os"
	"path/filepath"
	"testing"
)

// ========== isSafePath 路径安全检查测试 ==========

// TestIsSafePath_RelativePath 测试相对路径解析是否正确落入工作区
func TestIsSafePath_RelativePath(t *testing.T) {
	workDir := t.TempDir()

	got, err := isSafePath(workDir, "hello.txt")
	if err != nil {
		t.Fatalf("Testing Error, isSafePath() return: %v", err)
	}

	want := filepath.Join(workDir, "hello.txt")
	if got != want {
		t.Errorf("Testing Error, expect: %q, actual: %q", want, got)
	}
}

// TestIsSafePath_SubdirectoryPath 测试子目录下的相对路径
func TestIsSafePath_SubdirectoryPath(t *testing.T) {
	workDir := t.TempDir()

	got, err := isSafePath(workDir, filepath.Join("sub", "dir", "file.txt"))
	if err != nil {
		t.Fatalf("Testing Error, isSafePath() return: %v", err)
	}

	want := filepath.Join(workDir, "sub", "dir", "file.txt")
	if got != want {
		t.Errorf("Testing Error, expect: %q, actual: %q", want, got)
	}
}

// TestIsSafePath_PathTraversal 测试路径穿越攻击（../../）应被拒绝
func TestIsSafePath_PathTraversal(t *testing.T) {
	workDir := t.TempDir()

	_, err := isSafePath(workDir, "../../etc/passwd")
	if err == nil {
		t.Fatal("TestIsSafePath_PathTraversal FAIL")
	}
}

// TestIsSafePath_AbsolutePathOutsideWork 测试绝对路径逃逸到工作区外应被拒绝
func TestIsSafePath_AbsolutePathOutsideWork(t *testing.T) {
	workDir := t.TempDir()

	// 构造一个工作区外的绝对路径
	outsidePath := filepath.Join(os.TempDir(), "outside_escape", "evil.txt")

	_, err := isSafePath(workDir, outsidePath)
	if err == nil {
		t.Fatal("TestIsSafePath_AbsolutePathOutsideWork FAIL")
	}
}

// TestIsSafePath_AbsolutePathInsideWork 测试工作区内的绝对路径应被允许
func TestIsSafePath_AbsolutePathInsideWork(t *testing.T) {
	workDir := t.TempDir()
	insidePath := filepath.Join(workDir, "safe.txt")

	got, err := isSafePath(workDir, insidePath)
	if err != nil {
		t.Fatalf("Testing Error, isSafePath() return: %v", err)
	}
	if got != insidePath {
		t.Errorf("Testing Error, expect: %q, actual: %q", insidePath, got)
	}
}

// TestIsSafePath_PrefixSpoof 测试前缀欺骗（如 /work2 不应匹配 /work）
func TestIsSafePath_PrefixSpoof(t *testing.T) {
	workDir := t.TempDir()
	// 构造一个与工作目录名称前缀相同但多出后缀的路径
	spoofPath := workDir + "2" + string(filepath.Separator) + "bad.txt"

	_, err := isSafePath(workDir, spoofPath)
	if err == nil {
		t.Fatal("TestIsSafePath_PrefixSpoof FAIL")
	}
}

// ========== WriteFileTool 基础属性测试 ==========

// TestWriteFileTool_Name 验证工具名称正确
func TestWriteFileTool_Name(t *testing.T) {
	tool := WriteFileTool{}
	toolname := tool.Name()
	if toolname != "write_file" {
		t.Errorf("Tool name error, expect: %q, actual: %q", "write_file", toolname)
	}
}

// TestWriteFileTool_Description 验证描述不为空
func TestWriteFileTool_Description(t *testing.T) {
	tool := WriteFileTool{}
	if desc := tool.Description(); desc == "" {
		t.Error("Tool description is empty")
	}
}

// TestWriteFileTool_Parameters 验证参数定义包含必填字段
func TestWriteFileTool_Parameters(t *testing.T) {
	tool := WriteFileTool{}
	params := tool.Parameters()
	paramrequired := []string{"filename", "content"}

	// Check if the top-level type is "object"
	if params["type"] != "object" {
		t.Errorf("Testing Error, expect: %q, actual: %q", "object", params["type"])
	}

	// Check if the properties include `filename` and `content` fields
	for _, value := range paramrequired {
		_, exists := params["properties"].(map[string]any)[value]
		if !exists {
			t.Errorf("Testing Error, expect: %q, actual: %q", value, "missing")
		}
	}

	// Check if the required fields include `filename` and `content`
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatalf("Testing Error, `required` field is missing or not an array")
	}
	for _, value := range paramrequired {
		flag := false
		for _, req := range required {
			if req == value {
				flag = true
				break
			}
		}
		if !flag {
			t.Errorf("Testing Error, expect: %q, actual: %q", paramrequired, required)
		}
	}
}

// ========== WriteFileTool.Call 功能测试 ==========

// newTestContext 创建测试用的 ToolContext，workPath 指向临时目录
func newTestContext(workPath string) *ToolContext {
	return &ToolContext{WorkPath: workPath}
}

// TestWriteFileTool_Call_Success 测试正常写入文件
func TestWriteFileTool_Call_Success(t *testing.T) {
	workDir := t.TempDir()
	tool := WriteFileTool{}
	ctx := newTestContext(workDir)

	args := map[string]any{
		"filename": "test.txt",
		"content":  "Hello, World!",
	}

	result := tool.Call(args, ctx)

	// 验证返回结果
	if !result.Ok {
		t.Fatalf("Testing Error, actual result: %s", result.Content)
	}
	if result.IsError {
		t.Error("Testing Error, successful call should have IsError as false")
	}

	// 验证文件确实被创建，内容正确
	targetPath := filepath.Join(workDir, "test.txt")
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Reading file failed: %v", err)
	}
	if string(data) != args["content"].(string) {
		t.Errorf("Testing Error, file content mismatch, expected %q, actual %q", "Hello, World!", string(data))
	}
}

// TestWriteFileTool_Call_MissingFilename 测试缺少 filename 参数
func TestWriteFileTool_Call_MissingFilename(t *testing.T) {
	workDir := t.TempDir()
	tool := WriteFileTool{}
	ctx := newTestContext(workDir)

	args := map[string]any{
		"content": "some content",
	}

	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("Testing Error, missing filename should return failure")
	}
	if !result.IsError {
		t.Error("Testing Error, missing filename should have IsError as true")
	}
}

// TestWriteFileTool_Call_EmptyFilename 测试 filename 为空字符串
func TestWriteFileTool_Call_EmptyFilename(t *testing.T) {
	workDir := t.TempDir()
	tool := WriteFileTool{}
	ctx := newTestContext(workDir)

	args := map[string]any{
		"filename": "",
		"content":  "some content",
	}

	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("Testing Error, empty filename should return failure")
	}
}

// TestWriteFileTool_Call_FilenameWrongType 测试 filename 类型不是 string
func TestWriteFileTool_Call_FilenameWrongType(t *testing.T) {
	workDir := t.TempDir()
	tool := WriteFileTool{}
	ctx := newTestContext(workDir)

	args := map[string]any{
		"filename": 12345,
		"content":  "some content",
	}

	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("Testing Error, filename type error should return failure")
	}
	if !result.IsError {
		t.Error("Testing Error, filename type error should have IsError as true")
	}
}

// TestWriteFileTool_Call_MissingContent 测试缺少 content 参数
func TestWriteFileTool_Call_MissingContent(t *testing.T) {
	workDir := t.TempDir()
	tool := WriteFileTool{}
	ctx := newTestContext(workDir)

	args := map[string]any{
		"filename": "test.txt",
	}

	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("Testing Error, missing content should return failure")
	}
	if !result.IsError {
		t.Error("Testing Error, missing content should have IsError as true")
	}
}

// TestWriteFileTool_Call_EmptyContent 测试 content 为空字符串
func TestWriteFileTool_Call_EmptyContent(t *testing.T) {
	workDir := t.TempDir()
	tool := WriteFileTool{}
	ctx := newTestContext(workDir)

	args := map[string]any{
		"filename": "test.txt",
		"content":  "",
	}

	result := tool.Call(args, ctx)

	if !result.Ok {
		t.Error("Testing Error, empty content should be allowed")
	}
}

// TestWriteFileTool_Call_ContentWrongType 测试 content 类型不是 string
func TestWriteFileTool_Call_ContentWrongType(t *testing.T) {
	workDir := t.TempDir()
	tool := WriteFileTool{}
	ctx := newTestContext(workDir)

	args := map[string]any{
		"filename": "test.txt",
		"content":  42,
	}

	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("Testing Error, content type error should return failure")
	}
	if !result.IsError {
		t.Error("Testing Error, content type error should have IsError as true")
	}
}

// TestWriteFileTool_Call_PathTraversal 测试路径穿越应被拦截
func TestWriteFileTool_Call_PathTraversal(t *testing.T) {
	workDir := t.TempDir()
	tool := WriteFileTool{}
	ctx := newTestContext(workDir)

	args := map[string]any{
		"filename": "../../evil.txt",
		"content":  "malicious content",
	}

	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("Testing Error, path traversal should return failure")
	}
	if !result.IsError {
		t.Error("Testing Error, path traversal should have IsError as true")
	}
}

// TestWriteFileTool_Call_AbsolutePathEscape 测试绝对路径逃逸应被拦截
func TestWriteFileTool_Call_AbsolutePathEscape(t *testing.T) {
	workDir := t.TempDir()
	tool := WriteFileTool{}
	ctx := newTestContext(workDir)

	args := map[string]any{
		"filename": filepath.Join(os.TempDir(), "escape_test_evil.txt"),
		"content":  "malicious content",
	}

	result := tool.Call(args, ctx)

	if result.Ok {
		t.Error("Testing Error, absolute path escape should return failure")
	}
	if !result.IsError {
		t.Error("Testing Error, absolute path escape should have IsError as true")
	}
}

// TestWriteFileTool_Call_OverwriteExistingFile 测试覆盖已有文件
func TestWriteFileTool_Call_OverwriteExistingFile(t *testing.T) {
	workDir := t.TempDir()
	tool := WriteFileTool{}
	ctx := newTestContext(workDir)

	targetPath := filepath.Join(workDir, "overwrite.txt")

	// 先写入初始内容
	if err := os.WriteFile(targetPath, []byte("old content"), 0644); err != nil {
		t.Fatalf("Testing Error: %v", err)
	}

	// 通过工具覆盖写入
	args := map[string]any{
		"filename": "overwrite.txt",
		"content":  "new content",
	}

	result := tool.Call(args, ctx)

	if !result.Ok {
		t.Fatalf("Testing Error, tool actully return: %s", result.Content)
	}

	// 验证内容已被覆盖
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Testing Error: %v", err)
	}
	if string(data) != args["content"].(string) {
		t.Errorf("Testing Error, expect: %q，actual: %q", args["content"].(string), string(data))
	}
}

// TestWriteFileTool_Call_UnicodeContent 测试写入包含 Unicode 字符（中文等）的内容
func TestWriteFileTool_Call_UnicodeContent(t *testing.T) {
	workDir := t.TempDir()
	tool := WriteFileTool{}
	ctx := newTestContext(workDir)

	unicodeContent := "你好，世界！🌍\n这是一段中文测试内容。\n Hello, world! This is a Chinese test case."
	args := map[string]any{
		"filename": "unicode.txt",
		"content":  unicodeContent,
	}

	result := tool.Call(args, ctx)

	if !result.Ok {
		t.Fatalf("Testing Error, tool actully return: %s", result.Content)
	}

	data, err := os.ReadFile(filepath.Join(workDir, "unicode.txt"))
	if err != nil {
		t.Fatalf("Testing Error: %v", err)
	}
	if string(data) != unicodeContent {
		t.Errorf("Testing Error, Unicode content is mismatch, expect: %q, actul: %q", unicodeContent, string(data))
	}
}

// ========== 通过 Registry 集成测试 ==========

// TestWriteFileTool_ViaRegistry 测试通过 Registry 注册并执行 WriteFileTool
func TestWriteFileTool_ViaRegistry(t *testing.T) {
	workDir := t.TempDir()
	registry := NewRegistry()
	registry.Register(WriteFileTool{})
	ctx := newTestContext(workDir)

	args := map[string]any{
		"filename": "registry_test.txt",
		"content":  "via registry",
	}

	result := registry.RunTool("write_file", args, ctx)

	if !result.Ok {
		t.Fatalf("Testing Error, tool actully return: %s", result.Content)
	}

	data, err := os.ReadFile(filepath.Join(workDir, "registry_test.txt"))
	if err != nil {
		t.Fatalf("Testing Error: %v", err)
	}
	if string(data) != "via registry" {
		t.Errorf("Testing Error, expect: %q，actual: %q", "via registry", string(data))
	}
}
