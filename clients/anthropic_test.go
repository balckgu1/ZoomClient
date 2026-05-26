package clients

import (
	"testing"

	"zoomClient/fsm"
	"zoomClient/tools"
)

func TestAnthropicContentStr(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"map", map[string]interface{}{"key": "value"}, `{"key":"value"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := anthropicContentStr(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestConvertToAnthropicMessages_SystemOnly(t *testing.T) {
	messages := []fsm.Message{
		{Role: "system", Content: "You are a helpful assistant"},
	}

	result, systemText := convertToAnthropicMessages(messages)
	if len(result) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result))
	}
	if systemText != "You are a helpful assistant" {
		t.Errorf("expected system text 'You are a helpful assistant', got %q", systemText)
	}
}

func TestConvertToAnthropicMessages_UserAndAssistant(t *testing.T) {
	messages := []fsm.Message{
		{Role: "system", Content: "You are a helpful assistant"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}

	result, systemText := convertToAnthropicMessages(messages)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if systemText != "You are a helpful assistant" {
		t.Errorf("expected system text, got %q", systemText)
	}

	// user message
	if result[0].Role != "user" {
		t.Errorf("expected first message role 'user', got %s", result[0].Role)
	}

	// assistant message
	if result[1].Role != "assistant" {
		t.Errorf("expected second message role 'assistant', got %s", result[1].Role)
	}
}

func TestConvertToAnthropicMessages_AssistantWithToolCalls(t *testing.T) {
	messages := []fsm.Message{
		{Role: "user", Content: "Read files"},
		{
			Role:    "assistant",
			Content: "I'll read the files for you",
			ToolCalls: []tools.ToolCall{
				{
					ID: "toolu_01",
					Function: tools.ToolCallFunction{
						Name:      "read_file",
						Arguments: map[string]interface{}{"path": "/tmp/a.txt"},
					},
				},
			},
		},
	}

	result, _ := convertToAnthropicMessages(messages)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	// assistant message should have tool_use block
	assistantMsg := result[1]
	if assistantMsg.Role != "assistant" {
		t.Fatalf("expected assistant role, got %s", assistantMsg.Role)
	}
	if len(assistantMsg.Content) != 2 { // text block + tool_use block
		t.Fatalf("expected 2 content blocks, got %d", len(assistantMsg.Content))
	}
}

func TestConvertToAnthropicMessages_ToolResults(t *testing.T) {
	messages := []fsm.Message{
		{Role: "user", Content: "Read files"},
		{
			Role: "assistant",
			ToolCalls: []tools.ToolCall{
				{ID: "toolu_01", Function: tools.ToolCallFunction{Name: "read_file"}},
			},
		},
		{Role: "tool", Content: "content of a.txt", ToolCallID: "toolu_01"},
	}

	result, _ := convertToAnthropicMessages(messages)
	// user + assistant(with tool call) + tool result(user message) = 3 messages
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	// tool results should be merged into a user message
	toolResultMsg := result[2]
	if toolResultMsg.Role != "user" {
		t.Errorf("expected tool results merged into user message, got role %s", toolResultMsg.Role)
	}
}

func TestConvertToAnthropicMessages_MultipleToolResults(t *testing.T) {
	messages := []fsm.Message{
		{
			Role: "assistant",
			ToolCalls: []tools.ToolCall{
				{ID: "toolu_01", Function: tools.ToolCallFunction{Name: "read_file"}},
				{ID: "toolu_02", Function: tools.ToolCallFunction{Name: "read_file"}},
			},
		},
		{Role: "tool", Content: "content a", ToolCallID: "toolu_01"},
		{Role: "tool", Content: "content b", ToolCallID: "toolu_02"},
	}

	result, _ := convertToAnthropicMessages(messages)
	// assistant(with tool calls) + merged tool results(user message) = 2 messages
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	// both tool results should be in a single user message
	if result[1].Role != "user" {
		t.Errorf("expected user role, got %s", result[1].Role)
	}
	if len(result[1].Content) != 2 {
		t.Errorf("expected 2 tool result blocks, got %d", len(result[1].Content))
	}
}

func TestConvertToAnthropicMessages_EmptyAssistant(t *testing.T) {
	messages := []fsm.Message{
		{Role: "assistant", Content: ""},
	}

	result, _ := convertToAnthropicMessages(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	// empty assistant should still have at least one text block
	if len(result[0].Content) != 1 {
		t.Errorf("expected 1 content block for empty assistant, got %d", len(result[0].Content))
	}
}

func TestBuildAnthropicTools(t *testing.T) {
	toolList := []tools.Tool{
		mockTool{
			name:        "read_file",
			description: "Read a file",
			params: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "file path",
					},
				},
				"required": []string{"path"},
			},
		},
	}

	result := buildAnthropicTools(toolList)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].OfTool == nil {
		t.Fatal("expected OfTool to be non-nil")
	}
	if result[0].OfTool.Name != "read_file" {
		t.Errorf("expected name 'read_file', got %s", result[0].OfTool.Name)
	}
	// Description is set via anthropic.String() helper in buildAnthropicTools
	// The param.Opt[string] type handles optional values; we just verify the tool name is correct
	// and assume the SDK handles description properly
}

func TestNewAnthropicClient(t *testing.T) {
	client := NewAnthropicClient("test-api-key")
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	// anthropic.Client is a struct (not a pointer), so we just verify the wrapper is created
}
