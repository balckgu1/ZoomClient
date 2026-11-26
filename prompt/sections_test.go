package prompt

import (
	"strings"
	"testing"
)

// TestBuild_BehaviorSections 验证交互风格、安全、上下文连续性三段均已注入，且包含关键约束。
func TestBuild_BehaviorSections(t *testing.T) {
	builder := NewSystemPromptBuilder(nil, "", "test-model", "./workdir")

	result := builder.Build()

	expectations := map[string][]string{
		"\n\nCommunication\n": {
			"Before the first tool call",
			"give a one-sentence update",
			"Do not narrate internal deliberation",
		},
		"\n\nSafety\n": {
			"explicit user confirmation",
			"--no-verify",
			"A denied tool call is the user's decision",
			"before overwriting it",
		},
		"\n\nContext\n": {
			CompactionMarker,
			"authoritative record",
			"re-read the files",
		},
	}
	for header, phrases := range expectations {
		if !strings.Contains(result, header) {
			t.Fatalf("Build() 缺少 %q 段:\n%s", header, result)
		}
		for _, p := range phrases {
			if !strings.Contains(result, p) {
				t.Errorf("%q 段应包含 %q", header, p)
			}
		}
	}
}

// TestBuild_MemoryUsageNote 验证 memory 段包含“快照可能过期、先校验”的使用约束，
// 且该约束排在保存规则之前（先讲怎么用，再讲怎么存）。
func TestBuild_MemoryUsageNote(t *testing.T) {
	result := NewSystemPromptBuilder(nil, "", "m", "./w").Build()

	for _, p := range []string{
		"Using memories:",
		"snapshot from the moment it was written",
		"verify it still exists",
		"trust the current state",
	} {
		if !strings.Contains(result, p) {
			t.Errorf("memory 段应包含 %q，实际:\n%s", p, result)
		}
	}

	usageIdx := strings.Index(result, "\n\nUsing memories:\n")
	saveIdx := strings.Index(result, "\n\nSave Memories:\n")
	if usageIdx < 0 || saveIdx < 0 {
		t.Fatalf("memory 使用约束与保存规则都应作为独立段存在，usage=%d save=%d", usageIdx, saveIdx)
	}
	if usageIdx > saveIdx {
		t.Error("memory 使用约束应排在保存规则之前")
	}
}

// TestBuild_BehaviorSectionOrder 验证段落顺序：身份 -> 交互风格 -> 工具协议 -> 安全 -> memory -> 上下文连续性 -> 环境。
func TestBuild_BehaviorSectionOrder(t *testing.T) {
	result := NewSystemPromptBuilder(nil, "", "m", "./w").Build()

	order := []string{
		"AI agent powered by ZoomClient",
		"\n\nCommunication\n",
		"\n\nTool Usage\n",
		"\n\nSafety\n",
		"\n\nUsing memories:\n",
		"\n\nSave Memories:\n",
		"\n\nContext\n",
		"\n\nCurrent Environment\n",
	}

	prev := -1
	for _, marker := range order {
		idx := strings.Index(result, marker)
		if idx < 0 {
			t.Fatalf("Build() 缺少 %q:\n%s", marker, result)
		}
		if idx < prev {
			t.Errorf("%q 应出现在上一个标记之后，实际位置 %d < %d", marker, idx, prev)
		}
		prev = idx
	}
}

// TestBuild_BehaviorSectionsNoEdgeNewline 静态段首尾不应自带换行，拼接由 Build 统一处理。
func TestBuild_BehaviorSectionsNoEdgeNewline(t *testing.T) {
	sections := map[string]string{
		"communication": communicationSection,
		"safety":        safetySection,
		"compaction":    compactionSection,
	}
	for name, section := range sections {
		if section == "" {
			t.Errorf("%s 段不应为空", name)
			continue
		}
		if strings.HasPrefix(section, "\n") || strings.HasSuffix(section, "\n") {
			t.Errorf("%s 段首尾不应有换行", name)
		}
	}
}

// TestCompactionMarker 锁定压缩标记文本，compact 包依赖该常量生成摘要消息。
func TestCompactionMarker(t *testing.T) {
	want := "This conversation was compacted for continuity."
	if CompactionMarker != want {
		t.Errorf("CompactionMarker 应保持为 %q，实际 %q", want, CompactionMarker)
	}
	if !strings.Contains(compactionSection, CompactionMarker) {
		t.Error("compactionSection 应原样引用 CompactionMarker，模型才能识别压缩消息")
	}
}
