package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	testcases := []struct {
		name     string
		raw      string
		wantMeta SkillManifest
		wantBody string
		wantErr  bool
	}{
		{
			name: "normal frontmatter",
			raw:  "---\nname: code-review\ndescription: Review checklist\ncompatibility: python 3.10+\nauthor: test\nversion: v1.0\n---\n# Body\nhello\n",
			wantMeta: SkillManifest{
				Name: "code-review", Description: "Review checklist",
				Compatibility: "python 3.10+", Version: "v1.0",
				Author: "test",
			},
			wantBody: "# Body\nhello\n",
			wantErr:  false,
		},
		{
			name: "multi-line description",
			raw:  "---\nname: code-review\ndescription: |\n  Line one\n  Line two: with colon\n  \"Quoted line\"\nauthor: test\nversion: v1.0\n---\nBody here",
			wantMeta: SkillManifest{
				Name:        "code-review",
				Description: "Line one\nLine two: with colon\n\"Quoted line\"\n",
				Author:      "test", Version: "v1.0",
			},
			wantBody: "Body here",
			wantErr:  false,
		},
		{
			name:     "no frontmatter",
			raw:      "# Just a markdown file\nNo frontmatter here.",
			wantMeta: SkillManifest{},
			wantBody: "# Just a markdown file\nNo frontmatter here.",
			wantErr:  false,
		},
		{
			name:     "unclosed frontmatter delimiter",
			raw:      "---\nname: broken\n# missing closing ---\nSome content",
			wantMeta: SkillManifest{},
			wantBody: "---\nname: broken\n# missing closing ---\nSome content",
			wantErr:  false, // 无有效闭合时视为无 frontmatter
		},
		{
			name:    "invalid yaml syntax",
			raw:     "---\nname: [unclosed bracket\n---\nbody",
			wantErr: true,
		},
		{
			name: "windows CRLF line endings",
			raw:  "---\r\nname: crlf-test\r\ndescription: Windows style\r\nauthor: win\r\nversion: \"2.0\"\r\n---\r\nCRLF Body\r\n",
			wantMeta: SkillManifest{
				Name: "crlf-test", Description: "Windows style",
				Author: "win", Version: "2.0",
			},
			wantBody: "CRLF Body\n",
			wantErr:  false,
		},
		{
			name: "quoted values with special characters",
			raw:  "---\nname: \"tool: special\"\ndescription: 'it''s a \"complex\" value'\nauthor: dev\nversion: v1\n---\nBody",
			wantMeta: SkillManifest{
				Name: "tool: special", Description: "it's a \"complex\" value",
				Author: "dev", Version: "v1",
			},
			wantBody: "Body",
			wantErr:  false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			meta, body, err := parseFrontmatter(tc.raw)

			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if !reflect.DeepEqual(meta, tc.wantMeta) {
				t.Errorf("meta mismatch:\n got:  %#v\n want: %#v", meta, tc.wantMeta)
			}
			if body != tc.wantBody {
				t.Errorf("body mismatch:\n got:  %q\n want: %q", body, tc.wantBody)
			}
		})
	}
}

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		candidates []string
		want       string
	}{
		// 精确匹配
		{
			name:       "exact match",
			target:     "commit",
			candidates: []string{"commit", "review", "deploy"},
			want:       "commit",
		},
		{
			name:       "exact match case insensitive",
			target:     "Commit",
			candidates: []string{"commit", "review", "deploy"},
			want:       "commit",
		},
		// 前缀匹配
		{
			name:       "prefix match",
			target:     "com",
			candidates: []string{"commit", "review", "deploy"},
			want:       "commit",
		},
		{
			name:       "prefix match case insensitive",
			target:     "REV",
			candidates: []string{"commit", "review", "deploy"},
			want:       "review",
		},
		// 模糊匹配
		{
			name:       "fuzzy match one edit",
			target:     "comit",
			candidates: []string{"commit", "review", "deploy"},
			want:       "commit",
		},
		{
			name:       "fuzzy match two edits",
			target:     "commt",
			candidates: []string{"commit", "review", "deploy"},
			want:       "commit",
		},
		{
			name:       "fuzzy match three edits",
			target:     "comi",
			candidates: []string{"commit", "review", "deploy"},
			want:       "commit",
		},
		// 超出最大编辑距离，无匹配
		{
			name:       "no match beyond max distance",
			target:     "xyz",
			candidates: []string{"commit", "review", "deploy"},
			want:       "",
		},
		// 候选列表为空
		{
			name:       "empty candidates",
			target:     "commit",
			candidates: []string{},
			want:       "",
		},
		// 候选列表为 nil
		{
			name:       "nil candidates",
			target:     "commit",
			candidates: nil,
			want:       "",
		},
		// 多个候选，选出编辑距离最小的
		{
			name:       "pick best fuzzy candidate",
			target:     "reviw",
			candidates: []string{"commit", "review", "deploy"},
			want:       "review",
		},
		// 精确匹配优先于模糊匹配
		{
			name:       "exact match beats fuzzy",
			target:     "deploy",
			candidates: []string{"deplo", "deploy", "deploys"},
			want:       "deploy",
		},
		// 前缀匹配优先于编辑距离匹配
		{
			name:       "prefix match beats fuzzy",
			target:     "dep",
			candidates: []string{"deploy", "deep", "dep"},
			want:       "deploy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuzzyMatch(tt.target, tt.candidates)
			if got != tt.want {
				t.Errorf("fuzzyMatch(%q, %v) = %q, want %q",
					tt.target, tt.candidates, got, tt.want)
			}
		})
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"identical strings", "abc", "abc", 0},
		{"both empty", "", "", 0},
		{"a empty", "", "abc", 3},
		{"b empty", "abc", "", 3},
		{"single insert", "abc", "abcd", 1},
		{"single delete", "abcd", "abc", 1},
		{"single substitute", "abc", "axc", 1},
		{"completely different", "abc", "xyz", 3},
		{"swap optimization a longer", "abcdef", "ab", 4},
		{"real world commit vs comit", "commit", "comit", 1},
		{"real world review vs reviw", "review", "reviw", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := levenshteinDistance(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("levenshteinDistance(%q, %q) = %d, want %d",
					tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestRegistryLoadAndDescribe(t *testing.T) {
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
	reg, err := NewSkillRegistry(dir)
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
	// 验证 basedir 属性指向 SKILL.md 所在目录
	if !strings.Contains(body, `basedir="`+skillDir+`"`) {
		t.Fatalf("expected basedir=%q in output, got: %q", skillDir, body)
	}

	if _, err := reg.LoadFullText("missing"); err == nil {
		t.Fatal("expected error for missing skill")
	}
}

func TestRegistryMissingDir(t *testing.T) {
	reg, err := NewSkillRegistry(filepath.Join(t.TempDir(), "not-exist"))
	if err != nil {
		t.Fatal(err)
	}
	if reg.Count() != 0 || reg.DescribeAvailable() != "" {
		t.Fatal("missing dir should yield empty registry")
	}
}
