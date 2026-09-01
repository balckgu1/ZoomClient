package ui

import (
	"bufio"
	"strings"
	"testing"
)

// 构造一个非交互（管道/缓冲区）Renderer，用于回退路径测试。
func fallbackRenderer(input string) (*Renderer, *strings.Builder) {
	out := &strings.Builder{}
	r := &Renderer{Out: out, In: strings.NewReader(input)}
	r.reader = bufio.NewReader(r.In)
	return r, out
}

// 回退路径应能逐行读取并去掉首尾空白。
func TestPromptUserFallbackReadsLine(t *testing.T) {
	r, _ := fallbackRenderer("  hello world  \n")
	line, ok := r.PromptUser()
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if line != "hello world" {
		t.Fatalf("expected %q, got %q", "hello world", line)
	}
}

// 回退路径在 EOF 时应返回 ok=false。
func TestPromptUserFallbackEOF(t *testing.T) {
	r, _ := fallbackRenderer("") // 空读 -> 立即 EOF
	_, ok := r.PromptUser()
	if ok {
		t.Fatalf("expected ok=false on EOF")
	}
}

// promptStr 在有模型名时附带显示模型。
func TestPromptStr(t *testing.T) {
	out := &strings.Builder{}
	r := &Renderer{Out: out, In: strings.NewReader("")}
	r.reader = bufio.NewReader(r.In)

	r.model = ""
	if !strings.Contains(r.promptStr(), "You>") {
		t.Fatalf("expected base prompt, got %q", r.promptStr())
	}
	r.model = "qwen3"
	if !strings.Contains(r.promptStr(), "qwen3") {
		t.Fatalf("expected model in prompt, got %q", r.promptStr())
	}
}

// 横幅应包含命令提示与模型信息。
func TestPrintSessionStartContainsHints(t *testing.T) {
	out := &strings.Builder{}
	r := &Renderer{Out: out, In: strings.NewReader("")}
	r.reader = bufio.NewReader(r.In)

	r.PrintSessionStart("qwen3", "/tmp/logs")
	got := out.String()
	for _, want := range []string{"ZoomClient", "qwen3", "/clear", "/help", "Tab"} {
		if !strings.Contains(got, want) {
			t.Fatalf("banner missing %q; got:\n%s", want, got)
		}
	}
}