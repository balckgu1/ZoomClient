package prompt

import (
	"strings"
	"testing"
	"zoomClient/fsm"
	"zoomClient/skills"
	"zoomClient/tools"
)

func newTestPipeline() *MessagePipeline {
	reg, _ := skills.NewRegistry("")
	builder := NewSystemPromptBuilder(reg, "", "test-model", "./workdir")
	return NewPipeline(builder)
}

// TestAssemblePayload_BasicFlow 基本组装流程
func TestAssemblePayload_BasicFlow(t *testing.T) {
	p := newTestPipeline()
	msgs := []fsm.Message{
		{Role: "user", Content: "hello"},
	}

	payload := p.AssemblePayload(msgs)

	if payload.SystemPrompt == "" {
		t.Error("SystemPrompt should not be empty")
	}
	if len(payload.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(payload.Messages))
	}
	if payload.Messages[0].Content != "hello" {
		t.Errorf("expected 'hello', got %v", payload.Messages[0].Content)
	}
}

// TestAssemblePayload_RemovesLeadingSystem 验证 normalize 去除首条 system
func TestAssemblePayload_RemovesLeadingSystem(t *testing.T) {
	p := newTestPipeline()
	msgs := []fsm.Message{
		{Role: "system", Content: "old system prompt"},
		{Role: "user", Content: "hi"},
	}

	payload := p.AssemblePayload(msgs)

	for _, m := range payload.Messages {
		if m.Role == "system" && m.Content == "old system prompt" {
			t.Error("old system message should have been removed by normalize")
		}
	}
	found := false
	for _, m := range payload.Messages {
		if m.Role == "user" && m.Content == "hi" {
			found = true
		}
	}
	if !found {
		t.Error("user message 'hi' should be preserved")
	}
}

// TestAddReminder_OneShotCleanup 验证 ClearOneShotReminders 只删 OneShot
func TestAddReminder_OneShotCleanup(t *testing.T) {
	p := newTestPipeline()
	p.AddReminder(Reminder{Content: "one-shot", Source: "hook", OneShot: true})
	p.AddReminder(Reminder{Content: "persistent", Source: "todo", OneShot: false})

	if len(p.reminders) != 2 {
		t.Fatalf("expected 2 reminders, got %d", len(p.reminders))
	}

	p.ClearOneShotReminders()

	if len(p.reminders) != 1 {
		t.Fatalf("expected 1 reminder after cleanup, got %d", len(p.reminders))
	}
	if p.reminders[0].Content != "persistent" {
		t.Errorf("expected 'persistent' to survive, got %q", p.reminders[0].Content)
	}
}

// TestAssemblePayload_WithReminders 验证 reminder 以 system role 追加
func TestAssemblePayload_WithReminders(t *testing.T) {
	p := newTestPipeline()
	p.AddReminder(Reminder{Content: "update your plan", Source: "todo", OneShot: true})

	msgs := []fsm.Message{
		{Role: "user", Content: "do something"},
	}

	payload := p.AssemblePayload(msgs)

	last := payload.Messages[len(payload.Messages)-1]
	if last.Role != "system" {
		t.Errorf("reminder should have role 'system', got %q", last.Role)
	}
	if !strings.Contains(last.Content.(string), "update your plan") {
		t.Errorf("reminder content mismatch, got %v", last.Content)
	}
}

// TestAssemblePayload_EmptyMessages 空消息列表不 panic
func TestAssemblePayload_EmptyMessages(t *testing.T) {
	p := newTestPipeline()
	payload := p.AssemblePayload([]fsm.Message{})

	if payload.SystemPrompt == "" {
		t.Error("SystemPrompt should still be built even with empty messages")
	}
	if len(payload.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(payload.Messages))
	}
}

// TestNormalize_PreservesToolCallsWithEmptyContent assistant 有 ToolCalls 但 content 为空时应保留
func TestNormalize_PreservesToolCallsWithEmptyContent(t *testing.T) {
	p := newTestPipeline()
	msgs := []fsm.Message{
		{Role: "assistant", Content: "", ToolCalls: []tools.ToolCall{{ID: "1", Function: tools.ToolCallFunction{Name: "test"}}}},
	}

	payload := p.AssemblePayload(msgs)

	if len(payload.Messages) != 1 {
		t.Errorf("assistant with ToolCalls should be preserved, got %d messages", len(payload.Messages))
	}
}
