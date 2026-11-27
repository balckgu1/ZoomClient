package tools

// 本文件覆盖 ask_user_question 工具与 StdinUserAsker 的行为。

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// fakeUserAsker 记录收到的请求并返回预设回答，便于断言工具侧行为。
type fakeUserAsker struct {
	gotReq QuestionRequest
	answer string
	err    error
}

func (f *fakeUserAsker) AskQuestion(req QuestionRequest) (string, error) {
	f.gotReq = req
	return f.answer, f.err
}

func TestAskUserQuestion_透传问题并返回答案(t *testing.T) {
	asker := &fakeUserAsker{answer: "  使用 SQLite  "}
	res := AskUserQuestionTool{}.Call(map[string]any{
		"question": "选哪个数据库？",
		"header":   "存储选型",
		"options":  []any{"SQLite", "Postgres"},
	}, &ToolContext{UserAsker: asker})
	if res.IsError || res.Content != "使用 SQLite" {
		t.Fatalf("应返回去空格后的答案，实际：%+v", res)
	}
	if asker.gotReq.Question != "选哪个数据库？" || asker.gotReq.Header != "存储选型" {
		t.Errorf("问题/标题透传错误：%+v", asker.gotReq)
	}
	if len(asker.gotReq.Options) != 2 || asker.gotReq.Options[1] != "Postgres" {
		t.Errorf("选项透传错误：%+v", asker.gotReq.Options)
	}
}

func TestAskUserQuestion_选项数量上限与脏数据过滤(t *testing.T) {
	asker := &fakeUserAsker{answer: "ok"}
	AskUserQuestionTool{}.Call(map[string]any{
		"question": "q",
		"options":  []any{"a", "a", "", "b", "c", "d", "e", "f", "g"},
	}, &ToolContext{UserAsker: asker})
	if len(asker.gotReq.Options) != maxQuestionOptions {
		t.Errorf("选项应截断到 %d 个，实际 %d", maxQuestionOptions, len(asker.gotReq.Options))
	}
	seen := map[string]bool{}
	for _, opt := range asker.gotReq.Options {
		if opt == "" {
			t.Errorf("空选项应被过滤：%+v", asker.gotReq.Options)
		}
		if seen[opt] {
			t.Errorf("重复选项应被去重：%+v", asker.gotReq.Options)
		}
		seen[opt] = true
	}
}

func TestAskUserQuestion_错误路径(t *testing.T) {
	if res := (AskUserQuestionTool{}).Call(map[string]any{"question": ""}, &ToolContext{UserAsker: &fakeUserAsker{}}); !res.IsError {
		t.Error("空问题应报错")
	}
	if res := (AskUserQuestionTool{}).Call(map[string]any{"question": "q"}, &ToolContext{}); !res.IsError || !strings.Contains(res.Content, "not available") {
		t.Errorf("缺少提问通道应报错：%s", res.Content)
	}
	failing := &fakeUserAsker{err: errors.New("user cancelled the question")}
	res := AskUserQuestionTool{}.Call(map[string]any{"question": "q"}, &ToolContext{UserAsker: failing})
	if !res.IsError || !strings.Contains(res.Content, "cancelled") {
		t.Errorf("应透传提问通道错误：%s", res.Content)
	}
	empty := &fakeUserAsker{answer: "   "}
	if res := (AskUserQuestionTool{}).Call(map[string]any{"question": "q"}, &ToolContext{UserAsker: empty}); !res.IsError {
		t.Error("空答案应被视为未回答")
	}
}

func TestStdinUserAsker_数字选择与自由输入(t *testing.T) {
	// 数字选择 → 映射到对应选项
	var out bytes.Buffer
	asker := &StdinUserAsker{Out: &out, In: strings.NewReader("2\n")}
	answer, err := asker.AskQuestion(QuestionRequest{Question: "选哪个？", Options: []string{"A", "B"}})
	if err != nil || answer != "B" {
		t.Fatalf("数字选择应返回 B，实际 %q err=%v", answer, err)
	}
	if !strings.Contains(out.String(), "1) A") || !strings.Contains(out.String(), "选哪个？") {
		t.Errorf("输出应包含问题与选项：%s", out.String())
	}

	// 自由输入 → 原样返回
	free := &StdinUserAsker{Out: &out, In: strings.NewReader("自定义答案\n")}
	if answer, err := free.AskQuestion(QuestionRequest{Question: "q"}); err != nil || answer != "自定义答案" {
		t.Errorf("自由输入应原样返回，实际 %q err=%v", answer, err)
	}

	// 超出范围的数字按文本处理
	overflow := &StdinUserAsker{Out: &out, In: strings.NewReader("9\n")}
	if answer, err := overflow.AskQuestion(QuestionRequest{Question: "q", Options: []string{"A"}}); err != nil || answer != "9" {
		t.Errorf("越界数字应按文本返回，实际 %q err=%v", answer, err)
	}
}

func TestStdinUserAsker_空输入与EOF(t *testing.T) {
	var out bytes.Buffer
	empty := &StdinUserAsker{Out: &out, In: strings.NewReader("\n")}
	if _, err := empty.AskQuestion(QuestionRequest{Question: "q"}); err == nil {
		t.Error("空回答应返回错误")
	}
	eof := &StdinUserAsker{Out: &out, In: strings.NewReader("")}
	if _, err := eof.AskQuestion(QuestionRequest{Question: "q"}); err == nil {
		t.Error("EOF 应返回错误")
	}
}
