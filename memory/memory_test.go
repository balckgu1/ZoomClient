package memory

import (
	"strings"
	"testing"
)

func TestBuildExtractionRegistry_ToolSet(t *testing.T) {
	reg := BuildExtractionRegistry("/tmp/whatever")
	names := reg.GetAllNames()
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}

	want := []string{"read_file", "glob_search", "list_directory", "save_memory", "update_memory", "delete_memory"}
	for _, w := range want {
		if !set[w] {
			t.Errorf("extraction registry missing tool %q; got %v", w, names)
		}
	}

	notWant := []string{"run_bash", "write_file", "edit_file", "sub_task", "search_memory"}
	for _, nw := range notWant {
		if set[nw] {
			t.Errorf("extraction registry should NOT contain %q", nw)
		}
	}

	if len(names) != len(want) {
		t.Errorf("expected exactly %d tools, got %d: %v", len(want), len(names), names)
	}
}

func TestBuildManifest(t *testing.T) {
	t.Run("empty memoryDir", func(t *testing.T) {
		if got := BuildManifest(""); got != emptyManifestPlaceholder {
			t.Errorf("expected placeholder, got %q", got)
		}
	})

	t.Run("missing MEMORY.md", func(t *testing.T) {
		dir := createTempMemoryDir(t)
		if got := BuildManifest(dir); got != emptyManifestPlaceholder {
			t.Errorf("expected placeholder, got %q", got)
		}
	})

	t.Run("with index content", func(t *testing.T) {
		dir := createTempMemoryDir(t)
		writeMemoryFileRaw(t, dir, "MEMORY.md", "# Memory Index\n- foo: bar [user]\n")
		got := BuildManifest(dir)
		if !strings.Contains(got, "foo: bar") {
			t.Errorf("expected manifest to contain index entry, got %q", got)
		}
	})

	t.Run("oversized truncated", func(t *testing.T) {
		dir := createTempMemoryDir(t)
		writeMemoryFileRaw(t, dir, "MEMORY.md", strings.Repeat("x", manifestMaxBytes+100))
		got := BuildManifest(dir)
		if !strings.Contains(got, "[manifest truncated]") {
			t.Errorf("expected truncation marker, got len=%d", len(got))
		}
		if len(got) > manifestMaxBytes+64 {
			t.Errorf("expected truncated manifest near cap, got len=%d", len(got))
		}
	})
}

func TestBuildExtractionForkPrompt(t *testing.T) {
	p := BuildExtractionForkPrompt("- foo: bar [user]")
	if !strings.Contains(p, "- foo: bar [user]") {
		t.Error("prompt should embed manifest")
	}
	if !strings.Contains(p, "Nothing to save") {
		t.Error("prompt should instruct Nothing to save")
	}
}

func TestExtractionSystemPrompt(t *testing.T) {
	for _, kw := range []string{"Nothing to save", "frontmatter", "user", "feedback", "project", "reference"} {
		if !strings.Contains(ExtractionSystemPrompt, kw) {
			t.Errorf("system prompt missing %q", kw)
		}
	}
}
