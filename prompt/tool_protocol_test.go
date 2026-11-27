package prompt

import (
	"strings"
	"testing"
)

// TestBuild_ToolProtocolSection 验证工具协议段存在且包含关键约束：
// 文件工具优先于 shell、路径限制在工作区内、整文件覆盖前先读、命令有硬超时、失败后不得绕过或原样重试。
func TestBuild_ToolProtocolSection(t *testing.T) {
	builder := NewSystemPromptBuilder(nil, "", "test-model", "./workdir")

	result := builder.Build()

	if !strings.Contains(result, "\n\nTool Usage\n") {
		t.Fatalf("Build() 应包含工具协议段，实际:\n%s", result)
	}

	expectations := []string{
		"read_file",                       // 文本文件走 read_file
		"glob_search",                     // 找文件走 glob_search
		"list_directory",                  // 列目录走 list_directory
		"path escape",                     // 明确工作区外路径会被拒绝
		"hard timeout",                    // 命令有硬超时
		"Read a file before modifying",    // 整文件覆盖前先读
		"do not repeat an identical call", // 被拒绝/失败后不要原样重试
	}
	for _, expect := range expectations {
		if !strings.Contains(result, expect) {
			t.Errorf("工具协议段应包含 %q，实际:\n%s", expect, result)
		}
	}

	// 该段为常量段，不依赖 registry/memory，任何构造下都应存在
	empty := NewSystemPromptBuilder(nil, "", "m", "./w").buildToolProtocol()
	if empty == "" {
		t.Error("buildToolProtocol() 不应返回空字符串")
	}
	if strings.HasPrefix(empty, "\n") || strings.HasSuffix(empty, "\n") {
		t.Error("工具协议段首尾不应有换行，拼接由 Build 统一处理")
	}
}

// TestBuild_ToolProtocolOrder 验证段落顺序：core -> tool protocol -> memory -> dynamic。
// 静态段在前、动态日期在后，保证前缀缓存稳定。
func TestBuild_ToolProtocolOrder(t *testing.T) {
	builder := NewSystemPromptBuilder(nil, "", "test-model", "./workdir")

	result := builder.Build()

	coreIdx := strings.Index(result, "AI agent powered by ZoomClient")
	toolIdx := strings.Index(result, "\n\nTool Usage\n")
	memoryIdx := strings.Index(result, "Save Memories:")
	dynamicIdx := strings.Index(result, "\n\nCurrent Environment\n")

	if coreIdx < 0 || toolIdx < 0 || memoryIdx < 0 || dynamicIdx < 0 {
		t.Fatalf("段落缺失，core=%d tool=%d memory=%d dynamic=%d\n%s",
			coreIdx, toolIdx, memoryIdx, dynamicIdx, result)
	}
	if !(coreIdx < toolIdx && toolIdx < memoryIdx && memoryIdx < dynamicIdx) {
		t.Errorf("段落顺序应为 core < tool protocol < memory < dynamic，实际 %d/%d/%d/%d",
			coreIdx, toolIdx, memoryIdx, dynamicIdx)
	}
}

// TestBuild_NoDuplicateFileRules 回归保护：读写顺序规则只出现在工具协议段，
// core 段不应再重复（此前 core 曾引用不存在的 fs-observation-policy 与 write/edit 工具名）。
func TestBuild_NoDuplicateFileRules(t *testing.T) {
	builder := NewSystemPromptBuilder(nil, "", "test-model", "./workdir")

	result := builder.Build()

	if strings.Contains(result, "fs-observation-policy") {
		t.Error("prompt 不应引用项目中不存在的 fs-observation-policy")
	}
	if got := strings.Count(result, "Read a file before modifying"); got != 1 {
		t.Errorf("读后改规则应只出现 1 次，实际 %d 次", got)
	}
}
