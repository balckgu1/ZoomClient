package clients

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"zoomClient/fsm"
	"zoomClient/tools"
)

func TestBuildOllamaTools(t *testing.T) {
	toolList := []tools.Tool{
		mockTool{
			name:        "run_bash",
			description: "Run shell command",
			params: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{"type": "string"},
				},
				"required": []string{"command"},
			},
		},
		mockTool{
			name:        "write_file",
			description: "Write to file",
			params: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path":    map[string]interface{}{"type": "string"},
					"content": map[string]interface{}{"type": "string"},
				},
				"required": []string{"path", "content"},
			},
		},
	}

	result := BuildOllamaTools(toolList)
	if len(result) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result))
	}

	if result[0].Type != "function" {
		t.Errorf("expected type 'function', got %s", result[0].Type)
	}
	if result[0].Function.Name != "run_bash" {
		t.Errorf("expected name 'run_bash', got %s", result[0].Function.Name)
	}
	if result[0].Function.Description != "Run shell command" {
		t.Errorf("expected description 'Run shell command', got %s", result[0].Function.Description)
	}
	if result[0].Function.Parameters == nil {
		t.Error("expected parameters to be non-nil")
	}

	if result[1].Function.Name != "write_file" {
		t.Errorf("expected name 'write_file', got %s", result[1].Function.Name)
	}
}

func TestOllamaClient_Chat_Success(t *testing.T) {
	mockResponse := ChatResponse{
		Model:   "qwen3:8b",
		Message: fsm.Message{Role: "assistant", Content: "Hello from Ollama!"},
		Done:    true,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected path /api/chat, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %s", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL)
	messages := []fsm.Message{
		{Role: "user", Content: "Say hello"},
	}

	resp, err := client.Chat("qwen3:8b", messages, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "qwen3:8b" {
		t.Errorf("expected model 'qwen3:8b', got %s", resp.Model)
	}
	if resp.Message.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", resp.Message.Role)
	}
	if resp.Message.Content != "Hello from Ollama!" {
		t.Errorf("expected content 'Hello from Ollama!', got %v", resp.Message.Content)
	}
	if !resp.Done {
		t.Error("expected Done to be true")
	}
}

func TestOllamaClient_Chat_NDJSON(t *testing.T) {
	// Ollama may return NDJSON even with stream:false
	lines := []ChatResponse{
		{Model: "qwen3:8b", Message: fsm.Message{Role: "assistant", Content: "Hello"}},
		{Model: "qwen3:8b", Message: fsm.Message{Role: "assistant", Content: " world"}},
		{Model: "qwen3:8b", Message: fsm.Message{Role: "assistant", Content: "!"}, Done: true},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		for _, line := range lines {
			data, _ := json.Marshal(line)
			w.Write(data)
			w.Write([]byte("\n"))
		}
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL)
	messages := []fsm.Message{{Role: "user", Content: "Say hello"}}

	resp, err := client.Chat("qwen3:8b", messages, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.Content != "Hello world!" {
		t.Errorf("expected content 'Hello world!', got %v", resp.Message.Content)
	}
}

func TestOllamaClient_Chat_NDJSON_WithToolCalls(t *testing.T) {
	lines := []ChatResponse{
		{
			Model: "qwen3:8b",
			Message: fsm.Message{
				Role: "assistant",
				ToolCalls: []tools.ToolCall{
					{ID: "call_1", Function: tools.ToolCallFunction{Name: "read_file", Arguments: map[string]interface{}{"path": "/tmp/a.txt"}}},
				},
			},
		},
		{
			Model: "qwen3:8b",
			Message: fsm.Message{
				Role: "assistant",
				ToolCalls: []tools.ToolCall{
					{ID: "call_2", Function: tools.ToolCallFunction{Name: "read_file", Arguments: map[string]interface{}{"path": "/tmp/b.txt"}}},
				},
			},
			Done: true,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		for _, line := range lines {
			data, _ := json.Marshal(line)
			w.Write(data)
			w.Write([]byte("\n"))
		}
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL)
	messages := []fsm.Message{{Role: "user", Content: "Read files"}}

	resp, err := client.Chat("qwen3:8b", messages, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Message.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(resp.Message.ToolCalls))
	}
	if resp.Message.ToolCalls[0].Function.Name != "read_file" {
		t.Errorf("expected first tool call 'read_file', got %s", resp.Message.ToolCalls[0].Function.Name)
	}
	if resp.Message.ToolCalls[1].Function.Name != "read_file" {
		t.Errorf("expected second tool call 'read_file', got %s", resp.Message.ToolCalls[1].Function.Name)
	}
}

func TestOllamaClient_Chat_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "model not found"}`))
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL)
	messages := []fsm.Message{{Role: "user", Content: "Hello"}}

	_, err := client.Chat("qwen3:8b", messages, nil, nil)
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
	if !contains(err.Error(), "500") {
		t.Errorf("expected error to contain '500', got: %v", err)
	}
}

func TestOllamaClient_Chat_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("this is not json\n"))
		// write a valid line after invalid ones
		resp := ChatResponse{
			Model:   "qwen3:8b",
			Message: fsm.Message{Role: "assistant", Content: "valid"},
			Done:    true,
		}
		data, _ := json.Marshal(resp)
		w.Write(data)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL)
	messages := []fsm.Message{{Role: "user", Content: "Hello"}}

	resp, err := client.Chat("qwen3:8b", messages, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.Content != "valid" {
		t.Errorf("expected content 'valid', got %v", resp.Message.Content)
	}
}

func TestOllamaClient_Chat_WithTools(t *testing.T) {
	mockResponse := ChatResponse{
		Model: "qwen3:8b",
		Message: fsm.Message{
			Role:    "assistant",
			Content: "I'll help you",
		},
		Done: true,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify request body contains tools
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if len(req.Tools) != 1 {
			t.Errorf("expected 1 tool in request, got %d", len(req.Tools))
		}
		if req.Tools[0].Function.Name != "read_file" {
			t.Errorf("expected tool name 'read_file', got %s", req.Tools[0].Function.Name)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL)
	toolList := []tools.Tool{
		mockTool{name: "read_file", description: "Read file", params: map[string]interface{}{"type": "object"}},
	}
	messages := []fsm.Message{{Role: "user", Content: "Read a file"}}

	resp, err := client.Chat("qwen3:8b", messages, toolList, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.Content != "I'll help you" {
		t.Errorf("expected content 'I'll help you', got %v", resp.Message.Content)
	}
}
