package memory

import (
	"os"
	"path/filepath"
	"testing"
	"zoomClient/tools"

	"go.uber.org/zap"
)

// newTestToolCtx 创建用于测试的 ToolContext（使用 NopLogger）。
func newTestToolCtx(t *testing.T) *tools.ToolContext {
	t.Helper()
	return &tools.ToolContext{
		Logger:    zap.NewNop(),
		SessionID: "test-session",
	}
}

// createTempMemoryDir 创建临时 memory 目录，测试结束后自动清理。
func createTempMemoryDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

// writeMemoryFile 在指定目录中写入一个 memory markdown 文件。
// fileName 为不含扩展名的名称，会自动加 .md 后缀。
func writeMemoryFile(t *testing.T, dir, fileName, name, description, typ, body string) {
	t.Helper()
	content := "---\nname: " + name + "\ndescription: " + description + "\ntype: " + typ + "\n---\n" + body + "\n"
	err := os.WriteFile(filepath.Join(dir, fileName+".md"), []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write memory file %s: %v", fileName, err)
	}
}

// writeMemoryFileRaw 在指定目录中写入原始内容的文件。
func writeMemoryFileRaw(t *testing.T, dir, fileName, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, fileName), []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write file %s: %v", fileName, err)
	}
}

// buildIndex 在目录中创建 MEMORY.md 索引，供 delete/update/list 测试使用。
func buildIndex(t *testing.T, dir string) {
	t.Helper()
	if err := rebuildIndex(dir); err != nil {
		t.Fatalf("failed to rebuild index: %v", err)
	}
}
