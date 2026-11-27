package tools

// 本文件覆盖 grep_search 的内容匹配、输出模式、过滤规则与路径安全边界。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupGrepWorkdir 构造一个带多类型文件的临时工作区。
func setupGrepWorkdir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"main.go":               "package main\n\nfunc main() {\n\tprintln(\"Hello World\")\n\tprintln(\"hello again\")\n}\n",
		"src/util.go":           "package src\n\n// TODO: fix this later\nvar Version = \"1.0.0\"\n",
		"src/app.ts":            "const msg = 'Hello TypeScript';\nexport default msg;\n",
		"docs/readme.md":        "# Hello\nSome text with hello inside.\n",
		"node_modules/pkg/x.js": "hello in dependency, should be skipped\n",
		"binary.dat":            "hello\x00binary content should be skipped",
	}
	for name, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}
	}
	return dir
}

func TestGrepSearch_内容模式带行号且默认忽略大小写(t *testing.T) {
	dir := setupGrepWorkdir(t)
	tool := GrepSearch{}
	res := tool.Call(map[string]any{"pattern": "hello"}, &ToolContext{WorkPath: dir})
	if res.IsError || !res.Ok {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "main.go:4:") || !strings.Contains(res.Content, "main.go:5:") {
		t.Errorf("应命中 main.go 的两行，实际输出：\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "docs/readme.md:1:") {
		t.Errorf("应命中 docs/readme.md，实际输出：\n%s", res.Content)
	}
	if strings.Contains(res.Content, "node_modules") {
		t.Errorf("node_modules 应被跳过，实际输出：\n%s", res.Content)
	}
	if strings.Contains(res.Content, "binary.dat") {
		t.Errorf("二进制文件应被跳过，实际输出：\n%s", res.Content)
	}
}

func TestGrepSearch_大小写敏感(t *testing.T) {
	dir := setupGrepWorkdir(t)
	tool := GrepSearch{}
	res := tool.Call(map[string]any{"pattern": "Hello World", "case_sensitive": true}, &ToolContext{WorkPath: dir})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "main.go:4:") {
		t.Errorf("应命中首字母大写的行，实际输出：\n%s", res.Content)
	}
	// 关闭大小写敏感后 hello again 也应命中
	res2 := tool.Call(map[string]any{"pattern": "hello", "case_sensitive": false}, &ToolContext{WorkPath: dir})
	if !strings.Contains(res2.Content, "main.go:5:") {
		t.Errorf("大小写不敏感时应命中 hello again，实际输出：\n%s", res2.Content)
	}
}

func TestGrepSearch_正则与字面量模式(t *testing.T) {
	dir := setupGrepWorkdir(t)
	tool := GrepSearch{}
	// 正则：1.x.x 版本号
	res := tool.Call(map[string]any{"pattern": `"1\.\d+\.\d+"`}, &ToolContext{WorkPath: dir})
	if res.IsError || !strings.Contains(res.Content, "src/util.go:4:") {
		t.Errorf("正则模式应命中版本号，实际输出：\n%s", res.Content)
	}
	// 字面量：点号不应被当作正则通配符
	lit := tool.Call(map[string]any{"pattern": "println(\"Hello World\")", "fixed_strings": true}, &ToolContext{WorkPath: dir})
	if lit.IsError || !strings.Contains(lit.Content, "main.go:4:") {
		t.Errorf("字面量模式应命中完整调用行，实际输出：\n%s", lit.Content)
	}
}

func TestGrepSearch_文件列表与计数模式(t *testing.T) {
	dir := setupGrepWorkdir(t)
	tool := GrepSearch{}

	files := tool.Call(map[string]any{"pattern": "hello", "output_mode": "files_with_matches"}, &ToolContext{WorkPath: dir})
	if files.IsError {
		t.Fatalf("unexpected error: %s", files.Content)
	}
	if !strings.Contains(files.Content, "main.go") || !strings.Contains(files.Content, "docs/readme.md") {
		t.Errorf("文件列表缺少预期文件：\n%s", files.Content)
	}
	if strings.Contains(files.Content, ":4:") {
		t.Errorf("文件列表模式不应输出行内容：\n%s", files.Content)
	}

	counts := tool.Call(map[string]any{"pattern": "hello", "output_mode": "count"}, &ToolContext{WorkPath: dir})
	if counts.IsError {
		t.Fatalf("unexpected error: %s", counts.Content)
	}
	if !strings.Contains(counts.Content, "main.go: 2") {
		t.Errorf("计数模式应报告 main.go 命中 2 次：\n%s", counts.Content)
	}
}

func TestGrepSearch_glob过滤与单文件路径(t *testing.T) {
	dir := setupGrepWorkdir(t)
	tool := GrepSearch{}

	// 只搜 .ts：main.go 不应出现
	res := tool.Call(map[string]any{"pattern": "hello", "glob": "*.ts"}, &ToolContext{WorkPath: dir})
	if res.IsError || !strings.Contains(res.Content, "src/app.ts:1:") {
		t.Fatalf("glob 过滤应命中 src/app.ts，实际输出：\n%s", res.Content)
	}
	if strings.Contains(res.Content, "main.go") {
		t.Errorf("glob=*.ts 不应命中 main.go：\n%s", res.Content)
	}

	// 直接指定单个文件
	single := tool.Call(map[string]any{"pattern": "TODO", "path": "src/util.go"}, &ToolContext{WorkPath: dir})
	if single.IsError || !strings.Contains(single.Content, "src/util.go:3:") {
		t.Errorf("单文件搜索应命中 TODO 行，实际输出：\n%s", single.Content)
	}
}

func TestGrepSearch_head_limit截断提示(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for i := 0; i < 10; i++ {
		b.WriteString("match line\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "many.txt"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	res := GrepSearch{}.Call(map[string]any{"pattern": "match", "head_limit": float64(3)}, &ToolContext{WorkPath: dir})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if got := strings.Count(res.Content, "many.txt:"); got != 3 {
		t.Errorf("head_limit=3 时应只输出 3 行，实际 %d 行：\n%s", got, res.Content)
	}
	if !strings.Contains(res.Content, "[truncated") {
		t.Errorf("截断时应给出提示：\n%s", res.Content)
	}
}

func TestGrepSearch_无匹配与参数错误(t *testing.T) {
	dir := setupGrepWorkdir(t)
	tool := GrepSearch{}

	none := tool.Call(map[string]any{"pattern": "zzz_not_exist_zzz"}, &ToolContext{WorkPath: dir})
	if none.IsError || !strings.Contains(none.Content, "No matches found") {
		t.Errorf("无匹配时应返回 No matches found，实际：%s", none.Content)
	}

	missing := tool.Call(map[string]any{}, &ToolContext{WorkPath: dir})
	if !missing.IsError {
		t.Error("缺少 pattern 应返回错误")
	}

	badRegex := tool.Call(map[string]any{"pattern": "[unclosed"}, &ToolContext{WorkPath: dir})
	if !badRegex.IsError {
		t.Error("非法正则应返回错误")
	}

	badMode := tool.Call(map[string]any{"pattern": "hello", "output_mode": "invalid"}, &ToolContext{WorkPath: dir})
	if !badMode.IsError {
		t.Error("非法 output_mode 应返回错误")
	}
}

func TestGrepSearch_拒绝工作区外路径(t *testing.T) {
	dir := setupGrepWorkdir(t)
	res := GrepSearch{}.Call(map[string]any{"pattern": "hello", "path": "../../etc"}, &ToolContext{WorkPath: dir})
	if !res.IsError || !strings.Contains(res.Content, "path escape") {
		t.Errorf("工作区外路径应被拒绝，实际：%s", res.Content)
	}
}

func TestGrepSearch_上下文取消后停止遍历(t *testing.T) {
	dir := setupGrepWorkdir(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := GrepSearch{}.Call(map[string]any{"pattern": "hello"}, &ToolContext{WorkPath: dir, Ctx: ctx})
	// 取消后不应 panic，允许返回空结果
	if !res.Ok && !res.IsError {
		t.Errorf("取消场景应返回确定结果，实际：%+v", res)
	}
}

func TestGrepSearch_跳过依赖与构建目录(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"node_modules", "vendor", "dist", ".git", "src"} {
		full := filepath.Join(dir, sub)
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		content := "needle in " + sub + "\n"
		if err := os.WriteFile(filepath.Join(full, "x.go"), []byte(content), 0o644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}
	}
	res := GrepSearch{}.Call(map[string]any{"pattern": "needle"}, &ToolContext{WorkPath: dir})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "src/x.go") {
		t.Errorf("src 目录应被搜索：\n%s", res.Content)
	}
	for _, skipped := range []string{"node_modules", "vendor", "dist", ".git"} {
		if strings.Contains(res.Content, skipped+"/x.go") {
			t.Errorf("%s 目录应被跳过：\n%s", skipped, res.Content)
		}
	}
}
