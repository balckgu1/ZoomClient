package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func newTestGlobCtx(t *testing.T) (*ToolContext, string) {
	workDir := t.TempDir()
	return &ToolContext{
		WorkPath:  workDir,
		Logger:    zap.NewNop(),
		SessionID: "test",
	}, workDir
}

func createFiles(t *testing.T, base string, paths []string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(base, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir for %s: %v", p, err)
		}
		if err := os.WriteFile(full, []byte("x"), 0644); err != nil {
			t.Fatalf("create %s: %v", p, err)
		}
	}
}

// ========== Basic Properties ==========

func TestGlobSearch_Name(t *testing.T) {
	g := &GlobSearch{}
	if g.Name() != "glob_search" {
		t.Errorf("Name() = %q, want %q", g.Name(), "glob_search")
	}
}

func TestGlobSearch_Description(t *testing.T) {
	g := &GlobSearch{}
	if g.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

func TestGlobSearch_Parameters(t *testing.T) {
	g := &GlobSearch{}
	params := g.Parameters()

	if params["type"] != "object" {
		t.Errorf("type = %v, want object", params["type"])
	}
	props := params["properties"].(map[string]any)
	if _, ok := props["pattern"]; !ok {
		t.Error("missing required property: pattern")
	}
	required := params["required"].([]string)
	found := false
	for _, r := range required {
		if r == "pattern" {
			found = true
		}
	}
	if !found {
		t.Error("required should contain 'pattern'")
	}
}

// ========== Call Functional Tests ==========

func TestGlobSearch_Call_AllGoFiles(t *testing.T) {
	ctx, workDir := newTestGlobCtx(t)
	createFiles(t, workDir, []string{
		"main.go", "util.go", "src/app.go", "src/lib/helper.go", "README.md",
	})

	result := (&GlobSearch{}).Call(map[string]any{"pattern": "**/*.go"}, ctx)
	if !result.Ok {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	for _, expected := range []string{"main.go", "util.go", "src/app.go", "src/lib/helper.go"} {
		if !strings.Contains(result.Content, expected) {
			t.Errorf("result should contain %q, got:\n%s", expected, result.Content)
		}
	}
	if strings.Contains(result.Content, "README.md") {
		t.Error("result should not contain README.md")
	}
}

func TestGlobSearch_Call_TopLevelOnly(t *testing.T) {
	ctx, workDir := newTestGlobCtx(t)
	createFiles(t, workDir, []string{
		"main.go", "src/app.go",
	})

	result := (&GlobSearch{}).Call(map[string]any{"pattern": "*.go"}, ctx)
	if !result.Ok {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "main.go") {
		t.Error("should contain main.go")
	}
	if strings.Contains(result.Content, "src/app.go") {
		t.Error("*.go should not match nested files")
	}
}

func TestGlobSearch_Call_SpecificDir(t *testing.T) {
	ctx, workDir := newTestGlobCtx(t)
	createFiles(t, workDir, []string{
		"src/main.go", "src/pkg/util.go", "lib/other.go",
	})

	result := (&GlobSearch{}).Call(map[string]any{"pattern": "src/**/*.go"}, ctx)
	if !result.Ok {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "src/main.go") {
		t.Error("should contain src/main.go")
	}
	if !strings.Contains(result.Content, "src/pkg/util.go") {
		t.Error("should contain src/pkg/util.go")
	}
	if strings.Contains(result.Content, "lib/other.go") {
		t.Error("should not contain lib/other.go")
	}
}

func TestGlobSearch_Call_WithBasePath(t *testing.T) {
	ctx, workDir := newTestGlobCtx(t)
	createFiles(t, workDir, []string{
		"src/a.go", "src/b.go", "src/sub/c.go", "lib/d.go",
	})

	result := (&GlobSearch{}).Call(map[string]any{"pattern": "**/*.go", "path": "src"}, ctx)
	if !result.Ok {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "lib/d.go") {
		t.Error("should not contain lib/d.go when base is src")
	}
	if !strings.Contains(result.Content, "a.go") {
		t.Error("should contain a.go")
	}
}

func TestGlobSearch_Call_QuestionMark(t *testing.T) {
	ctx, workDir := newTestGlobCtx(t)
	createFiles(t, workDir, []string{
		"test_a.py", "test_b.py", "test_ab.py",
	})

	result := (&GlobSearch{}).Call(map[string]any{"pattern": "test_?.py"}, ctx)
	if !result.Ok {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "test_a.py") {
		t.Error("should match test_a.py")
	}
	if !strings.Contains(result.Content, "test_b.py") {
		t.Error("should match test_b.py")
	}
	if strings.Contains(result.Content, "test_ab.py") {
		t.Error("test_?.py should not match test_ab.py")
	}
}

func TestGlobSearch_Call_CharacterClass(t *testing.T) {
	ctx, workDir := newTestGlobCtx(t)
	createFiles(t, workDir, []string{
		"file1.txt", "file2.txt", "file3.txt", "fileA.txt",
	})

	result := (&GlobSearch{}).Call(map[string]any{"pattern": "file[12].txt"}, ctx)
	if !result.Ok {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "file1.txt") {
		t.Error("should match file1.txt")
	}
	if !strings.Contains(result.Content, "file2.txt") {
		t.Error("should match file2.txt")
	}
	if strings.Contains(result.Content, "file3.txt") {
		t.Error("should not match file3.txt")
	}
	if strings.Contains(result.Content, "fileA.txt") {
		t.Error("should not match fileA.txt")
	}
}

func TestGlobSearch_Call_NoMatch(t *testing.T) {
	ctx, workDir := newTestGlobCtx(t)
	createFiles(t, workDir, []string{"hello.go"})

	result := (&GlobSearch{}).Call(map[string]any{"pattern": "**/*.rs"}, ctx)
	if !result.Ok {
		t.Fatalf("no match should still be Ok, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "No files matched") {
		t.Errorf("should indicate no match, got: %s", result.Content)
	}
}

func TestGlobSearch_Call_MaxResults(t *testing.T) {
	ctx, workDir := newTestGlobCtx(t)
	files := make([]string, 50)
	for i := range files {
		files[i] = fmt.Sprintf("data/file_%02d.txt", i)
	}
	createFiles(t, workDir, files)

	result := (&GlobSearch{}).Call(map[string]any{"pattern": "data/*.txt", "max_results": float64(10)}, ctx)
	if !result.Ok {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "truncated") {
		t.Error("should indicate truncation when results exceed max_results")
	}
	lines := strings.Split(strings.TrimSpace(result.Content), "\n")
	fileLines := 0
	for _, l := range lines {
		if strings.HasSuffix(l, ".txt") {
			fileLines++
		}
	}
	if fileLines > 10 {
		t.Errorf("should have at most 10 results, got %d", fileLines)
	}
}

func TestGlobSearch_Call_MissingPattern(t *testing.T) {
	ctx, _ := newTestGlobCtx(t)
	result := (&GlobSearch{}).Call(map[string]any{}, ctx)
	if result.Ok || !result.IsError {
		t.Error("missing pattern should return error")
	}
}

func TestGlobSearch_Call_EmptyPattern(t *testing.T) {
	ctx, _ := newTestGlobCtx(t)
	result := (&GlobSearch{}).Call(map[string]any{"pattern": ""}, ctx)
	if result.Ok || !result.IsError {
		t.Error("empty pattern should return error")
	}
}

func TestGlobSearch_Call_SkipsHiddenDirs(t *testing.T) {
	ctx, workDir := newTestGlobCtx(t)
	createFiles(t, workDir, []string{
		"visible.go", ".hidden/secret.go",
	})

	result := (&GlobSearch{}).Call(map[string]any{"pattern": "**/*.go"}, ctx)
	if !result.Ok {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "secret.go") {
		t.Error("should skip hidden directories")
	}
	if !strings.Contains(result.Content, "visible.go") {
		t.Error("should find visible.go")
	}
}

func TestGlobSearch_Call_PathEscape(t *testing.T) {
	ctx, _ := newTestGlobCtx(t)
	result := (&GlobSearch{}).Call(map[string]any{
		"pattern": "**/*.go",
		"path":    "../../etc",
	}, ctx)
	if result.Ok {
		t.Error("path escape should be blocked")
	}
	if !result.IsError {
		t.Error("path escape should return IsError=true")
	}
}

func TestGlobSearch_Call_DeepNesting(t *testing.T) {
	ctx, workDir := newTestGlobCtx(t)
	createFiles(t, workDir, []string{
		"a/b/c/d/e/deep.go",
		"a/b/shallow.go",
		"top.go",
	})

	result := (&GlobSearch{}).Call(map[string]any{"pattern": "**/*.go"}, ctx)
	if !result.Ok {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	for _, expected := range []string{"a/b/c/d/e/deep.go", "a/b/shallow.go", "top.go"} {
		if !strings.Contains(result.Content, expected) {
			t.Errorf("should contain %q, got:\n%s", expected, result.Content)
		}
	}
}

// ========== Registry Integration ==========

func TestGlobSearch_ViaRegistry(t *testing.T) {
	ctx, workDir := newTestGlobCtx(t)
	createFiles(t, workDir, []string{"app.go"})

	reg := NewRegistry()
	reg.Register(&GlobSearch{})

	result := reg.RunTool("glob_search", map[string]any{"pattern": "*.go"}, ctx)
	if !result.Ok {
		t.Fatalf("expected success via registry, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "app.go") {
		t.Errorf("should find app.go, got: %s", result.Content)
	}
}
