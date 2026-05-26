package clients

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"zoomClient/fsm"
	"zoomClient/tools"
)

// mockTool 实现 tools.Tool 接口，用于测试
type mockTool struct {
	name        string
	description string
	params      map[string]interface{}
}

func (m mockTool) Name() string        { return m.name }
func (m mockTool) Description() string { return m.description }
func (m mockTool) Parameters() map[string]interface{} {
	return m.params
}
func (m mockTool) Call(args map[string]interface{}, ctx *tools.ToolContext) tools.ToolResult {
	return tools.ToolResult{Ok: true, Content: "ok"}
}

func TestBuildOpenAITools(t *testing.T) {
	toolList := []tools.Tool{
		mockTool{
			name:        "read_file",
			description: "Read a file",
			params: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string"},
				},
				"required": []string{"path"},
			},
		},
	}

	result := BuildOpenAITools(toolList)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Type != "function" {
		t.Errorf("expected type 'function', got %s", result[0].Type)
	}
	if result[0].Function.Name != "read_file" {
		t.Errorf("expected name 'read_file', got %s", result[0].Function.Name)
	}
	if result[0].Function.Description != "Read a file" {
		t.Errorf("expected description 'Read a file', got %s", result[0].Function.Description)
	}
	if result[0].Function.Parameters == nil {
		t.Error("expected parameters to be non-nil")
	}
}

func TestConvertToOpenAIMessages(t *testing.T) {
	messages := []fsm.Message{
		{Role: "system", Content: "You are a helpful assistant"},
		{Role: "user", Content: "Hello"},
		{
			Role:    "assistant",
			Content: "Let me help",
			ToolCalls: []tools.ToolCall{
				{
					ID: "call_123",
					Function: tools.ToolCallFunction{
						Name:      "read_file",
						Arguments: map[string]interface{}{"path": "/tmp/test.txt"},
					},
				},
			},
		},
		{Role: "tool", Content: "file content", ToolCallID: "call_123"},
	}

	result := convertToOpenAIMessages(messages)
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result))
	}

	// system
	if result[0].Role != "system" || result[0].Content != "You are a helpful assistant" {
		t.Errorf("system message mismatch: %+v", result[0])
	}

	// user
	if result[1].Role != "user" || result[1].Content != "Hello" {
		t.Errorf("user message mismatch: %+v", result[1])
	}

	// assistant with tool calls
	if result[2].Role != "assistant" {
		t.Errorf("expected role assistant, got %s", result[2].Role)
	}
	if len(result[2].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result[2].ToolCalls))
	}
	if result[2].ToolCalls[0].ID != "call_123" {
		t.Errorf("expected tool call id 'call_123', got %s", result[2].ToolCalls[0].ID)
	}
	if result[2].ToolCalls[0].Function.Name != "read_file" {
		t.Errorf("expected function name 'read_file', got %s", result[2].ToolCalls[0].Function.Name)
	}
	// arguments should be JSON string
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(result[2].ToolCalls[0].Function.Arguments), &args); err != nil {
		t.Errorf("arguments should be valid JSON: %v", err)
	}
	if args["path"] != "/tmp/test.txt" {
		t.Errorf("expected path '/tmp/test.txt', got %v", args["path"])
	}

	// tool
	if result[3].Role != "tool" || result[3].ToolCallID != "call_123" {
		t.Errorf("tool message mismatch: %+v", result[3])
	}
}

func TestConvertToOpenAIMessages_WithReasoningContent(t *testing.T) {
	messages := []fsm.Message{
		{
			Role:             "assistant",
			Content:          "The answer is 42",
			ReasoningContent: "Let me think...",
		},
	}

	result := convertToOpenAIMessages(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].ReasoningContent != "Let me think..." {
		t.Errorf("expected reasoning content 'Let me think...', got %s", result[0].ReasoningContent)
	}
}

func TestConvertFromOpenAIToolCalls(t *testing.T) {
	openaiToolCalls := []OpenAIToolCall{
		{
			ID:   "call_456",
			Type: "function",
			Function: OpenAIToolCallFunction{
				Name:      "write_file",
				Arguments: `{"path":"/tmp/out.txt","content":"hello"}`,
			},
		},
	}

	result := convertFromOpenAIToolCalls(openaiToolCalls)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result))
	}
	if result[0].ID != "call_456" {
		t.Errorf("expected id 'call_456', got %s", result[0].ID)
	}
	if result[0].Function.Name != "write_file" {
		t.Errorf("expected name 'write_file', got %s", result[0].Function.Name)
	}
	if result[0].Function.Arguments["path"] != "/tmp/out.txt" {
		t.Errorf("expected path '/tmp/out.txt', got %v", result[0].Function.Arguments["path"])
	}
	if result[0].Function.Arguments["content"] != "hello" {
		t.Errorf("expected content 'hello', got %v", result[0].Function.Arguments["content"])
	}
}

func TestConvertFromOpenAIToolCalls_EmptyArguments(t *testing.T) {
	openaiToolCalls := []OpenAIToolCall{
		{
			ID:   "call_789",
			Type: "function",
			Function: OpenAIToolCallFunction{
				Name:      "git_status",
				Arguments: "",
			},
		},
	}

	result := convertFromOpenAIToolCalls(openaiToolCalls)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result))
	}
	if result[0].Function.Arguments == nil {
		t.Error("expected empty map for empty arguments, got nil")
	}
}

func TestOpenAIClient_Chat_Success(t *testing.T) {
	mockResponse := OpenAIChatResponse{
		ID:    "resp_123",
		Model: "gpt-4o",
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Message: OpenAIMessage{
					Role:    "assistant",
					Content: "Hello, world!",
				},
				FinishReason: "stop",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected path /chat/completions, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization header 'Bearer test-key', got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %s", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "test-key")
	messages := []fsm.Message{
		{Role: "user", Content: "Say hello"},
	}

	resp, err := client.Chat("gpt-4o", messages, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %s", resp.Model)
	}
	if resp.Message.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", resp.Message.Role)
	}
	if resp.Message.Content != "Hello, world!" {
		t.Errorf("expected content 'Hello, world!', got %v", resp.Message.Content)
	}
	if !resp.Done {
		t.Error("expected Done to be true")
	}
}

func TestOpenAIClient_Chat_WithToolCalls(t *testing.T) {
	mockResponse := OpenAIChatResponse{
		ID:    "resp_456",
		Model: "gpt-4o",
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Message: OpenAIMessage{
					Role: "assistant",
					ToolCalls: []OpenAIToolCall{
						{
							ID:   "call_abc",
							Type: "function",
							Function: OpenAIToolCallFunction{
								Name:      "read_file",
								Arguments: `{"path":"/tmp/test.txt"}`,
							},
						},
					},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "test-key")
	toolList := []tools.Tool{
		mockTool{name: "read_file", description: "Read file", params: map[string]interface{}{"type": "object"}},
	}
	messages := []fsm.Message{{Role: "user", Content: "Read a file"}}

	resp, err := client.Chat("gpt-4o", messages, toolList, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.Message.ToolCalls))
	}
	if resp.Message.ToolCalls[0].Function.Name != "read_file" {
		t.Errorf("expected function name 'read_file', got %s", resp.Message.ToolCalls[0].Function.Name)
	}
	if resp.Message.ToolCalls[0].Function.Arguments["path"] != "/tmp/test.txt" {
		t.Errorf("expected path '/tmp/test.txt', got %v", resp.Message.ToolCalls[0].Function.Arguments["path"])
	}
}

func TestOpenAIClient_Chat_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "bad-key")
	messages := []fsm.Message{{Role: "user", Content: "Hello"}}

	_, err := client.Chat("gpt-4o", messages, nil, nil)
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
	if !contains(err.Error(), "401") {
		t.Errorf("expected error to contain '401', got: %v", err)
	}
}

func TestOpenAIClient_Chat_EmptyChoices(t *testing.T) {
	mockResponse := OpenAIChatResponse{
		ID:      "resp_789",
		Model:   "gpt-4o",
		Choices: []OpenAIChoice{},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "test-key")
	messages := []fsm.Message{{Role: "user", Content: "Hello"}}

	_, err := client.Chat("gpt-4o", messages, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
	if !contains(err.Error(), "无有效 choices") {
		t.Errorf("expected error about empty choices, got: %v", err)
	}
}

func TestOpenAIClient_Chat_NilContentFallback(t *testing.T) {
	mockResponse := OpenAIChatResponse{
		ID:    "resp_000",
		Model: "gpt-4o",
		Choices: []OpenAIChoice{
			{
				Message: OpenAIMessage{
					Role:    "assistant",
					Content: nil,
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "test-key")
	messages := []fsm.Message{{Role: "user", Content: "Hello"}}

	resp, err := client.Chat("gpt-4o", messages, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.Content != "" {
		t.Errorf("expected empty string for nil content, got %v", resp.Message.Content)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
