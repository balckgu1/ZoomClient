package prompt

import (
	"runtime"
	"strings"
	"testing"
	"zoomClient/skills"
)

// TestBuild_EmptyRegistryAndMemoryDir 测试空 registry + 空 memoryDir 场景。
// 预期：core + memory(save rules only) + dynamic 三段非空，tools 和 CLAUDE.md 为空。
func TestBuild_EmptyRegistryAndMemoryDir(t *testing.T) {
	reg, _ := skills.NewSkillRegistry("")
	builder := NewSystemPromptBuilder(reg, "", "test-model", "./workdir")

	result := builder.Build()

	// core 段：应包含核心身份说明
	if !strings.Contains(result, "helpful assistant") {
		t.Errorf("Build() should contain core prompt, got:\n%s", result)
	}

	// dynamic 段：应包含模型名和工作目录
	if !strings.Contains(result, "Model: test-model") {
		t.Errorf("Build() should contain model name in dynamic section, got:\n%s", result)
	}
	if !strings.Contains(result, "Working directory: ./workdir") {
		t.Errorf("Build() should contain workDir in dynamic section, got:\n%s", result)
	}

	// memory 段：应包含 save rules
	if !strings.Contains(result, "Save Memories:") {
		t.Errorf("Build() should contain memory save rules, got:\n%s", result)
	}

	// tools 和 CLAUDE.md 段应为空，不应出现多余内容
	// 检查没有 "Skills available" 文本（因为 registry 为空）
	if strings.Contains(result, "Skills available") {
		t.Errorf("Build() should NOT contain skills section with empty registry, got:\n%s", result)
	}
}

// TestBuild_NilRegistry 测试 nil registry 场景，确保不会 panic。
func TestBuild_NilRegistry(t *testing.T) {
	builder := NewSystemPromptBuilder(nil, "", "model-x", "/tmp")

	// 不应 panic
	result := builder.Build()

	if result == "" {
		t.Error("Build() should return non-empty string even with nil registry")
	}
	if !strings.Contains(result, "helpful assistant") {
		t.Errorf("Build() should still contain core prompt, got:\n%s", result)
	}
}

// TestBuild_DynamicSection 测试动态段的 4 项信息是否齐全。
func TestBuild_DynamicSection(t *testing.T) {
	builder := NewSystemPromptBuilder(nil, "", "deepseek-v4", "/home/user/project")

	result := builder.Build()

	expectations := []string{
		"## Current Environment",
		"- Date:",
		"- Working directory: /home/user/project",
		"- Model: deepseek-v4",
		"- OS: " + runtime.GOOS,
	}

	for _, expect := range expectations {
		if !strings.Contains(result, expect) {
			t.Errorf("Build() dynamic section should contain %q, got:\n%s", expect, result)
		}
	}
}

// TestBuild_SectionsSeparatedByDoubleNewline 测试各段之间用 \n\n 分隔。
func TestBuild_SectionsSeparatedByDoubleNewline(t *testing.T) {
	builder := NewSystemPromptBuilder(nil, "", "m", "./w")

	result := builder.Build()

	// core 段和 dynamic 段之间应有 \n\n
	if !strings.Contains(result, "\n\n") {
		t.Errorf("Build() sections should be separated by \\n\\n, got:\n%s", result)
	}
}
