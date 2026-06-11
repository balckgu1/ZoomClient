package memory

import (
	"strings"
	"testing"
)

// -----------------------------------------------------------------------
// TestParseFrontMatter
// -----------------------------------------------------------------------

func TestParseFrontMatter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantDesc string
		wantType string
		wantBody string
	}{
		{
			name:     "basic frontmatter",
			input:    "---\nname: test\ndescription: a test memory\ntype: user\n---\nThis is the body.\n",
			wantName: "test",
			wantDesc: "a test memory",
			wantType: "user",
			wantBody: "This is the body.",
		},
		{
			name:     "quoted description with colon",
			input:    "---\nname: db_config\ndescription: \"DB连接: host:port格式\"\ntype: project\n---\nConnection string\n",
			wantName: "db_config",
			wantDesc: "DB连接: host:port格式",
			wantType: "project",
			wantBody: "Connection string",
		},
		{
			name:     "quoted name with special chars",
			input:    "---\nname: \"my#memory\"\ndescription: simple\ntype: feedback\n---\nContent here\n",
			wantName: "my#memory",
			wantDesc: "simple",
			wantType: "feedback",
			wantBody: "Content here",
		},
		{
			name:     "no frontmatter",
			input:    "Just plain text without frontmatter.\n",
			wantName: "",
			wantDesc: "",
			wantType: "",
			wantBody: "",
		},
		{
			name:     "empty body",
			input:    "---\nname: empty\ndescription: no body\ntype: reference\n---\n",
			wantName: "empty",
			wantDesc: "no body",
			wantType: "reference",
			wantBody: "",
		},
		{
			name:     "multiline body",
			input:    "---\nname: multi\ndescription: multi-line body\ntype: user\n---\nLine 1\nLine 2\nLine 3\n",
			wantName: "multi",
			wantDesc: "multi-line body",
			wantType: "user",
			wantBody: "Line 1\nLine 2\nLine 3",
		},
		{
			name:     "description with escaped quotes",
			input:    "---\nname: test\ndescription: \"He said \\\"hello\\\" to me\"\ntype: user\n---\nbody\n",
			wantName: "test",
			wantDesc: `He said "hello" to me`,
			wantType: "user",
			wantBody: "body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := ParseFrontMatter(tt.input)
			if doc.FrontMatter.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", doc.FrontMatter.Name, tt.wantName)
			}
			if doc.FrontMatter.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", doc.FrontMatter.Description, tt.wantDesc)
			}
			if doc.FrontMatter.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", doc.FrontMatter.Type, tt.wantType)
			}
			if doc.Body != tt.wantBody {
				t.Errorf("Body = %q, want %q", doc.Body, tt.wantBody)
			}
		})
	}
}

// -----------------------------------------------------------------------
// TestYamlQuote
// -----------------------------------------------------------------------

func TestYamlQuote(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain string", "hello world", "hello world"},
		{"contains colon", "host:port", `"host:port"`},
		{"contains hash", "C# language", `"C# language"`},
		{"contains double quote", `say "hi"`, `"say \"hi\""`},
		{"contains newline", "line1\nline2", `"line1\nline2"`},
		{"leading space", " starts with space", `" starts with space"`},
		{"trailing space", "ends with space ", `"ends with space "`},
		{"no special chars", "simple_value", "simple_value"},
		{"contains backslash", `path\to\file`, `path\to\file`},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := yamlQuote(tt.input)
			if got != tt.want {
				t.Errorf("yamlQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// -----------------------------------------------------------------------
// TestYamlUnquote
// -----------------------------------------------------------------------

func TestYamlUnquote(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"unquoted plain", "hello", "hello"},
		{"quoted simple", `"hello"`, "hello"},
		{"quoted with escaped quote", `"say \"hi\""`, `say "hi"`},
		{"quoted with escaped newline", `"line1\nline2"`, "line1\nline2"},
		{"quoted with escaped backslash", `"path\\to\\file"`, `path\to\file`},
		{"single quote only", `"`, `"`},
		{"empty quoted", `""`, ""},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := yamlUnquote(tt.input)
			if got != tt.want {
				t.Errorf("yamlUnquote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// -----------------------------------------------------------------------
// TestYamlQuoteUnquote_RoundTrip
// -----------------------------------------------------------------------

func TestYamlQuoteUnquote_RoundTrip(t *testing.T) {
	values := []string{
		"simple",
		"host:port",
		`He said "hello"`,
		"line1\nline2",
		"has # hash",
		"C++ & Go",
	}
	for _, v := range values {
		t.Run(v, func(t *testing.T) {
			quoted := yamlQuote(v)
			got := yamlUnquote(quoted)
			if got != v {
				t.Errorf("roundtrip failed: %q -> %q -> %q", v, quoted, got)
			}
		})
	}
}

// -----------------------------------------------------------------------
// TestLoadMemorySection
// -----------------------------------------------------------------------

func TestLoadMemorySection(t *testing.T) {
	t.Run("empty dir", func(t *testing.T) {
		result := LoadMemorySection("")
		if result != "" {
			t.Errorf("expected empty result for empty dir, got %q", result)
		}
	})

	t.Run("nonexistent dir", func(t *testing.T) {
		result := LoadMemorySection("/nonexistent/path")
		if result != "" {
			t.Errorf("expected empty result for nonexistent dir, got %q", result)
		}
	})

	t.Run("dir with memory files", func(t *testing.T) {
		dir := createTempMemoryDir(t)
		writeMemoryFile(t, dir, "prefer_tabs", "prefer_tabs", "Use tabs for indentation", "user", "Always use tabs")
		writeMemoryFile(t, dir, "no_secrets", "no_secrets", "Do not commit secrets", "feedback", "Never commit API keys")
		buildIndex(t, dir)

		result := LoadMemorySection(dir)
		if !strings.Contains(result, "## Memories from previous sessions") {
			t.Error("expected section header")
		}
		if !strings.Contains(result, "search_memory") {
			t.Error("expected search_memory tool hint")
		}
		if !strings.Contains(result, "prefer_tabs") {
			t.Error("expected prefer_tabs in output")
		}
		if !strings.Contains(result, "no_secrets") {
			t.Error("expected no_secrets in output")
		}
		// body 不应出现在输出中（仅索引摘要）
		if strings.Contains(result, "Always use tabs") {
			t.Error("body content should NOT be in output, only index summary")
		}
		if strings.Contains(result, "Never commit API keys") {
			t.Error("body content should NOT be in output, only index summary")
		}
	})

	t.Run("priority ordering", func(t *testing.T) {
		dir := createTempMemoryDir(t)
		writeMemoryFile(t, dir, "user_pref", "user_pref", "A preference", "user", "body")
		writeMemoryFile(t, dir, "correction", "correction", "A correction", "feedback", "body")
		writeMemoryFile(t, dir, "ref_link", "ref_link", "A link", "reference", "body")
		buildIndex(t, dir)

		result := LoadMemorySection(dir)
		// feedback 应该排在 user 前面
		feedbackIdx := strings.Index(result, "correction")
		userIdx := strings.Index(result, "user_pref")
		refIdx := strings.Index(result, "ref_link")
		if feedbackIdx > userIdx {
			t.Error("expected feedback entries before user entries")
		}
		if userIdx > refIdx {
			t.Error("expected user entries before reference entries")
		}
	})

	t.Run("skips entries not in index", func(t *testing.T) {
		dir := createTempMemoryDir(t)
		writeMemoryFile(t, dir, "test", "test", "desc", "user", "test content")
		writeMemoryFile(t, dir, "real_memory", "real_memory", "A real memory", "user", "Content")
		// 索引仅包含 test
		writeMemoryFileRaw(t, dir, "MEMORY.md", "# Memory Index\n- test: desc [user]\n")

		result := LoadMemorySection(dir)
		if !strings.Contains(result, "test") {
			t.Error("expected test in output")
		}
		if strings.Contains(result, "real_memory") {
			t.Error("real_memory not in index, should not appear")
		}
	})
}
