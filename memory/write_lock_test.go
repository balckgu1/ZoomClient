package memory

// 本文件验证 memory 目录的进程内写锁（lock.go 的 writeMu）：
// 并发 save_memory 不丢失索引条目、不产生半损坏文件。

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestConcurrentSaveMemoryIndexIntegrity 并发保存不同 name 的记忆，
// 断言 MEMORY.md 索引包含全部条目、每个记忆文件可解析且 frontmatter 完整。
// 索引更新是"读-改-写"序列，若无 writeMu 串行化，并发写入会互相覆盖、丢失条目。
func TestConcurrentSaveMemoryIndexIntegrity(t *testing.T) {
	dir := createTempMemoryDir(t)
	ctx := newTestToolCtx(t)
	tool := NewSaveMemoryTool(dir)

	const workers = 16
	names := make([]string, workers)
	for i := range names {
		names[i] = fmt.Sprintf("mem-%02d", i)
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []string
	)
	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			res := tool.Call(map[string]interface{}{
				"name":        name,
				"description": fmt.Sprintf("concurrent save test %d", i),
				"type":        "user",
				"content":     fmt.Sprintf("body of %s", name),
			}, ctx)
			if !res.Ok {
				mu.Lock()
				errs = append(errs, fmt.Sprintf("%s: %s", name, res.Content))
				mu.Unlock()
			}
		}(i, name)
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("concurrent saves failed: %v", errs)
	}

	// 索引必须包含全部条目（无锁时读-改-写会丢失更新）
	data, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	entries := parseIndex(string(data))
	if len(entries) != workers {
		t.Fatalf("index should contain %d entries, got %d:\n%s", workers, len(entries), data)
	}
	indexed := make(map[string]bool, len(entries))
	for _, e := range entries {
		indexed[e.Name] = true
	}
	for _, name := range names {
		if !indexed[name] {
			t.Errorf("index missing entry %q", name)
		}
	}

	// 每个记忆文件必须存在且 frontmatter 完整可解析
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name+".md"))
		if err != nil {
			t.Errorf("memory file %s missing: %v", name, err)
			continue
		}
		doc := ParseFrontMatter(string(raw))
		if doc.FrontMatter.Name != name {
			t.Errorf("file %s frontmatter name = %q, want %q", name, doc.FrontMatter.Name, name)
		}
		if doc.FrontMatter.Type != "user" {
			t.Errorf("file %s frontmatter type = %q, want %q", name, doc.FrontMatter.Type, "user")
		}
	}
}
