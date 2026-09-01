package main

// 本文件测试 chatWithHooks 的 hook 干预与重试逻辑：
// 使用 mock 客户端按步骤返回预设响应/错误，验证
// PreChat 拦截、LLMError 重试、PostChat 空回复重试、预算耗尽与提醒注入。

import (
	"errors"
	"strings"
	"testing"

	"zoomClient/clients"
	"zoomClient/fsm"
	"zoomClient/hook"
	"zoomClient/logger"
	"zoomClient/prompt"
	"zoomClient/tools"
)

// TestMain 初始化全局日志，避免 chatWithHooks 内部 logger.Log 为 nil。
func TestMain(m *testing.M) {
	logger.Init()
	defer logger.Sync()
	m.Run()
}

// chatStep 表示 mock 客户端依次返回的每一步结果。
type chatStep struct {
	resp *clients.ChatResponse
	err  error
}

// mockChatClient 按步骤依次返回预设响应/错误，用于测试重试逻辑。
type mockChatClient struct {
	steps []chatStep
	calls int
}

// Chat 记录调用次数并按步骤顺序返回结果；步骤耗尽后返回错误。
func (m *mockChatClient) Chat(model string, messages []fsm.Message, toolList []tools.Tool, options map[string]interface{}) (*clients.ChatResponse, error) {
	i := m.calls
	m.calls++
	if i >= len(m.steps) {
		return nil, errors.New("mockChatClient steps exhausted")
	}
	return m.steps[i].resp, m.steps[i].err
}

// newTestPipeline 构造测试用 pipeline（builder 使用空依赖，仅用于承载与组装 reminder）。
func newTestPipeline() *prompt.MessagePipeline {
	return prompt.NewPipeline(prompt.NewSystemPromptBuilder(nil, "", "test-model", ""))
}

// validResponse 返回一个非空文本回复。
func validResponse() *clients.ChatResponse {
	return &clients.ChatResponse{Model: "test", Message: fsm.Message{Role: "assistant", Content: "hello"}}
}

// emptyResponse 返回一个空回复（无文本、无工具调用）。
func emptyResponse() *clients.ChatResponse {
	return &clients.ChatResponse{Model: "test", Message: fsm.Message{Role: "assistant", Content: ""}}
}

// TestChatWithHooks_NoHandlers_ReturnsFirstResponse 未注册 handler 时直接透传首次调用结果。
func TestChatWithHooks_NoHandlers_ReturnsFirstResponse(t *testing.T) {
	client := &mockChatClient{steps: []chatStep{{resp: validResponse()}}}
	runner := hook.NewRunner()

	resp, err := chatWithHooks(runner, client, "test-model", nil, nil, nil, 0, newTestPipeline())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.Message.Content != "hello" {
		t.Errorf("expected response content 'hello', got %+v", resp)
	}
	if client.calls != 1 {
		t.Errorf("expected 1 client call, got %d", client.calls)
	}
}

// TestChatWithHooks_LLMErrorRetry_Recovers LLM 首次失败且预算内时自动重试成功。
func TestChatWithHooks_LLMErrorRetry_Recovers(t *testing.T) {
	client := &mockChatClient{steps: []chatStep{
		{err: errors.New("network error")},
		{resp: validResponse()},
	}}
	runner := hook.NewRunner()
	runner.HookRegister(hook.EventLLMError, hook.OnLLMErrorRetry)

	resp, err := chatWithHooks(runner, client, "test-model", nil, nil, nil, 3, newTestPipeline())
	if err != nil {
		t.Fatalf("expected retry to recover, got error: %v", err)
	}
	if resp == nil || resp.Message.Content != "hello" {
		t.Errorf("expected 'hello' response, got %+v", resp)
	}
	if client.calls != 2 {
		t.Errorf("expected 2 client calls (1 fail + 1 retry), got %d", client.calls)
	}
}

// TestChatWithHooks_LLMErrorRetry_Exhausted 重试预算耗尽后返回错误。
func TestChatWithHooks_LLMErrorRetry_Exhausted(t *testing.T) {
	client := &mockChatClient{steps: []chatStep{
		{err: errors.New("boom")},
		{err: errors.New("boom")},
		{err: errors.New("boom")},
	}}
	runner := hook.NewRunner()
	runner.HookRegister(hook.EventLLMError, hook.OnLLMErrorRetry)

	_, err := chatWithHooks(runner, client, "test-model", nil, nil, nil, 2, newTestPipeline())
	if err == nil {
		t.Fatal("expected error after retry budget exhausted")
	}
	// 首次调用 + 2 次重试 = 3 次调用
	if client.calls != 3 {
		t.Errorf("expected 3 client calls, got %d", client.calls)
	}
}

// TestChatWithHooks_EmptyResponseRetry_Recovers PostChat 校验空回复后自动重试成功。
func TestChatWithHooks_EmptyResponseRetry_Recovers(t *testing.T) {
	client := &mockChatClient{steps: []chatStep{
		{resp: emptyResponse()},
		{resp: validResponse()},
	}}
	runner := hook.NewRunner()
	runner.HookRegister(hook.EventPostChat, hook.PostChatValidate)

	resp, err := chatWithHooks(runner, client, "test-model", nil, nil, nil, 3, newTestPipeline())
	if err != nil {
		t.Fatalf("expected retry to recover, got error: %v", err)
	}
	if resp == nil || resp.Message.Content != "hello" {
		t.Errorf("expected 'hello' response, got %+v", resp)
	}
	if client.calls != 2 {
		t.Errorf("expected 2 client calls, got %d", client.calls)
	}
}

// TestChatWithHooks_PreChatBlock_PreventsCall PreChat 返回 Block 时不应调用客户端。
func TestChatWithHooks_PreChatBlock_PreventsCall(t *testing.T) {
	client := &mockChatClient{steps: []chatStep{{resp: validResponse()}}}
	runner := hook.NewRunner()
	runner.HookRegister(hook.EventPreChat, func(p map[string]any) hook.HookResult {
		return hook.HookResult{ExitCode: hook.ExitBlock, Message: "blocked by policy"}
	})

	_, err := chatWithHooks(runner, client, "test-model", nil, nil, nil, 3, newTestPipeline())
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected block error, got %v", err)
	}
	if client.calls != 0 {
		t.Errorf("expected 0 client calls, got %d", client.calls)
	}
}

// TestChatWithHooks_PostChatInject_AddsReminder PostChat 注入的消息应在下一轮提示词组装时出现。
func TestChatWithHooks_PostChatInject_AddsReminder(t *testing.T) {
	client := &mockChatClient{steps: []chatStep{{resp: validResponse()}}}
	runner := hook.NewRunner()
	runner.HookRegister(hook.EventPostChat, func(p map[string]any) hook.HookResult {
		return hook.HookResult{ExitCode: hook.ExitInject, Message: "INJECTED-REMINDER"}
	})
	pipeline := newTestPipeline()

	if _, err := chatWithHooks(runner, client, "test-model", nil, nil, nil, 3, pipeline); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assembled := pipeline.AssemblePayload([]fsm.Message{})
	found := false
	for _, m := range assembled.Messages {
		if m.Content == "INJECTED-REMINDER" {
			found = true
		}
	}
	if !found {
		t.Error("expected injected reminder in assembled payload")
	}
}

// TestEstimateTokens_SimpleContent 验证 token 估算启发式：总字符数/4 向上取整。
func TestEstimateTokens_SimpleContent(t *testing.T) {
	msgs := []fsm.Message{
		{Role: "user", Content: "hello"},    // 5 字符
		{Role: "user", Content: "01234567"}, // 8 字符
	}
	got := estimateTokens(msgs)
	want := (5+8)/4 + 1 // 4
	if got != want {
		t.Errorf("expected %d tokens, got %d", want, got)
	}
}

// TestEstimateTokens_Empty 空消息序列估算为 0。
func TestEstimateTokens_Empty(t *testing.T) {
	if got := estimateTokens(nil); got != 0 {
		t.Errorf("expected 0 tokens for nil messages, got %d", got)
	}
}
