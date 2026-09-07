package prompt

import (
	"os"
	"path/filepath"
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

// TestSkillsSection_And_SkillCount 验证 skills 段与数量统计：
// nil/空 registry 返回空段与 0；含 skill 的 registry 返回目录段与数量，
// 且 Build()/Pipeline 透传结果包含该段原文（占用统计按字节拆分依赖这一点）。
func TestSkillsSection_And_SkillCount(t *testing.T) {
	// nil registry：不应 panic，返回空段与 0
	nilBuilder := NewSystemPromptBuilder(nil, "", "m", "./w")
	if got := nilBuilder.SkillsSection(); got != "" {
		t.Errorf("nil registry 的 SkillsSection() 应返回空字符串，实际 %q", got)
	}
	if got := nilBuilder.SkillCount(); got != 0 {
		t.Errorf("nil registry 的 SkillCount() 应返回 0，实际 %d", got)
	}

	// 空 registry：同样为空
	emptyReg, _ := skills.NewSkillRegistry("")
	emptyBuilder := NewSystemPromptBuilder(emptyReg, "", "m", "./w")
	if got := emptyBuilder.SkillsSection(); got != "" {
		t.Errorf("空 registry 的 SkillsSection() 应返回空字符串，实际 %q", got)
	}
	if got := emptyBuilder.SkillCount(); got != 0 {
		t.Errorf("空 registry 的 SkillCount() 应返回 0，实际 %d", got)
	}

	// 含 1 个 skill 的 registry
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "code-review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: code-review\ndescription: Review checklist\n---\nCheck for nil returns.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := skills.NewSkillRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	builder := NewSystemPromptBuilder(reg, "", "m", "./w")

	section := builder.SkillsSection()
	if !strings.Contains(section, "Skills available") || !strings.Contains(section, "code-review") {
		t.Errorf("SkillsSection() 应包含 skills 目录，实际 %q", section)
	}
	if builder.SkillCount() != 1 {
		t.Errorf("SkillCount() 应返回 1，实际 %d", builder.SkillCount())
	}
	// Build() 必须原样包含 skills 段，占用统计才能按字节拆分
	if !strings.Contains(builder.Build(), section) {
		t.Error("Build() 应包含 SkillsSection() 的完整原文")
	}

	// Pipeline 透传应与 builder 一致
	pipeline := NewPipeline(builder)
	if pipeline.SkillsSection() != section {
		t.Error("Pipeline.SkillsSection() 应与 builder 一致")
	}
	if pipeline.SkillCount() != 1 {
		t.Errorf("Pipeline.SkillCount() 应返回 1，实际 %d", pipeline.SkillCount())
	}
	if !strings.Contains(pipeline.BuildSystemPrompt(), section) {
		t.Error("Pipeline.BuildSystemPrompt() 应包含 skills 段")
	}

	// builder 为 nil 的 pipeline 应返回零值而非 panic
	nilPipeline := NewPipeline(nil)
	if nilPipeline.SkillsSection() != "" || nilPipeline.SkillCount() != 0 || nilPipeline.BuildSystemPrompt() != "" {
		t.Error("builder 为 nil 的 pipeline 应返回零值")
	}
}
