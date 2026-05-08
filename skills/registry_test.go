package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	raw := "---\nname: code-review\ndescription: Review checklist\nversion: 1.0\nlicense: MIT\n---\n# Body\nhello\n"
	meta, body := parseFrontmatter(raw)
	if meta["name"] != "code-review" || meta["description"] != "Review checklist" {
		t.Fatalf("unexpected meta: %#v", meta)
	}
	if !strings.HasPrefix(body, "# Body") {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	raw := "# Just body\ncontent"
	meta, body := parseFrontmatter(raw)
	if len(meta) != 0 {
		t.Fatalf("expected empty meta, got %#v", meta)
	}
	if body != raw {
		t.Fatalf("body should equal raw when no frontmatter")
	}
}

func TestRegistry_LoadAndDescribe(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "code-review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: code-review\ndescription: Review checklist\n---\nCheck for nil returns.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Logf("\ndir: %s\n", dir)
	t.Logf("\nskillDir: %s\n", skillDir)
	reg, err := NewRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("\nSkillRegistry: %+v\n", *reg)
	if reg.Count() != 1 {
		t.Fatalf("expected 1 skill, got %d", reg.Count())
	}
	if desc := reg.DescribeAvailable(); !strings.Contains(desc, "code-review") {
		t.Fatalf("directory text should contain skill name, got: %q", desc)
	}

	body, err := reg.LoadFullText("code-review")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Check for nil returns.") || !strings.Contains(body, `name="code-review"`) {
		t.Fatalf("unexpected body wrap: %q", body)
	}

	if _, err := reg.LoadFullText("missing"); err == nil {
		t.Fatal("expected error for missing skill")
	}
}

func TestRegistry_MissingDir(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "not-exist"))
	if err != nil {
		t.Fatal(err)
	}
	if reg.Count() != 0 || reg.DescribeAvailable() != "" {
		t.Fatal("missing dir should yield empty registry")
	}
}
