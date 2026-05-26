package clients

import (
	"testing"

	"zoomClient/fsm"
	"zoomClient/tools"

	"google.golang.org/genai"
)

func TestGeminiContentStr(t *testing.T) {
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
			result := geminiContentStr(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestParamsToGeminiSchema(t *testing.T) {
	params := map[string]interface{}{
		"type":        "object",
		"description": "test schema",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "the name",
			},
			"age": map[string]interface{}{
				"type":        "integer",
				"description": "the age",
			},
			"active": map[string]interface{}{
				"type": "boolean",
			},
			"score": map[string]interface{}{
				"type": "number",
			},
			"tags": map[string]interface{}{
				"type": "array",
			},
		},
		"required": []string{"name", "age"},
	}

	schema := paramsToGeminiSchema(params)
	if schema == nil {
		t.Fatal("expected non-nil schema")
	}
	if schema.Type != genai.TypeObject {
		t.Errorf("expected type object, got %v", schema.Type)
	}
	if schema.Description != "test schema" {
		t.Errorf("expected description 'test schema', got %s", schema.Description)
	}
	if len(schema.Properties) != 5 {
		t.Errorf("expected 5 properties, got %d", len(schema.Properties))
	}
	if schema.Properties["name"].Type != genai.TypeString {
		t.Errorf("expected name type string, got %v", schema.Properties["name"].Type)
	}
	if schema.Properties["age"].Type != genai.TypeInteger {
		t.Errorf("expected age type integer, got %v", schema.Properties["age"].Type)
	}
	if schema.Properties["active"].Type != genai.TypeBoolean {
		t.Errorf("expected active type boolean, got %v", schema.Properties["active"].Type)
	}
	if schema.Properties["score"].Type != genai.TypeNumber {
		t.Errorf("expected score type number, got %v", schema.Properties["score"].Type)
	}
	if schema.Properties["tags"].Type != genai.TypeArray {
		t.Errorf("expected tags type array, got %v", schema.Properties["tags"].Type)
	}
	if len(schema.Required) != 2 {
		t.Errorf("expected 2 required fields, got %d", len(schema.Required))
	}
}

func TestParamsToGeminiSchema_RequiredAsInterfaceSlice(t *testing.T) {
	params := map[string]interface{}{
		"type":     "object",
		"required": []interface{}{"name", "age"},
	}

	schema := paramsToGeminiSchema(params)
	if len(schema.Required) != 2 {
		t.Errorf("expected 2 required fields from interface slice, got %d", len(schema.Required))
	}
}

func TestBuildIDToNameMap(t *testing.T) {
	messages := []fsm.Message{
		{
			Role: "assistant",
			ToolCalls: []tools.ToolCall{
				{ID: "call_1", Function: tools.ToolCallFunction{Name: "read_file"}},
				{ID: "call_2", Function: tools.ToolCallFunction{Name: "write_file"}},
			},
		},
	}

	m := buildIDToNameMap(messages)
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m))
	}
	if m["call_1"] != "read_file" {
		t.Errorf("expected call_1 -> read_file, got %s", m["call_1"])
	}
	if m["call_2"] != "write_file" {
		t.Errorf("expected call_2 -> write_file, got %s", m["call_2"])
	}
}

func TestBuildGeminiTools(t *testing.T) {
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

	result := buildGeminiTools(toolList)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool group, got %d", len(result))
	}
	if len(result[0].FunctionDeclarations) != 1 {
		t.Fatalf("expected 1 function declaration, got %d", len(result[0].FunctionDeclarations))
	}
	fd := result[0].FunctionDeclarations[0]
	if fd.Name != "read_file" {
		t.Errorf("expected name 'read_file', got %s", fd.Name)
	}
	if fd.Description != "Read a file" {
		t.Errorf("expected description 'Read a file', got %s", fd.Description)
	}
	if fd.Parameters == nil {
		t.Error("expected parameters to be non-nil")
	}
}

func TestConvertToGeminiContents_SystemOnly(t *testing.T) {
	messages := []fsm.Message{
		{Role: "system", Content: "You are helpful"},
	}

	contents, systemText := convertToGeminiContents(messages)
	if len(contents) != 0 {
		t.Errorf("expected 0 contents, got %d", len(contents))
	}
	if systemText != "You are helpful" {
		t.Errorf("expected system text 'You are helpful', got %q", systemText)
	}
}

func TestConvertToGeminiContents_UserAndAssistant(t *testing.T) {
	messages := []fsm.Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi!"},
	}

	contents, systemText := convertToGeminiContents(messages)
	if len(contents) != 2 {
		t.Fatalf("expected 2 contents, got %d", len(contents))
	}
	if systemText != "You are helpful" {
		t.Errorf("expected system text, got %q", systemText)
	}

	// user
	if contents[0].Role != "user" {
		t.Errorf("expected first role 'user', got %s", contents[0].Role)
	}

	// assistant -> model
	if contents[1].Role != "model" {
		t.Errorf("expected second role 'model', got %s", contents[1].Role)
	}
}

func TestConvertToGeminiContents_AssistantWithToolCalls(t *testing.T) {
	messages := []fsm.Message{
		{Role: "user", Content: "Read files"},
		{
			Role:    "assistant",
			Content: "I'll read them",
			ToolCalls: []tools.ToolCall{
				{
					ID: "call_1",
					Function: tools.ToolCallFunction{
						Name:      "read_file",
						Arguments: map[string]interface{}{"path": "/tmp/a.txt"},
					},
				},
			},
		},
	}

	contents, _ := convertToGeminiContents(messages)
	if len(contents) != 2 {
		t.Fatalf("expected 2 contents, got %d", len(contents))
	}

	// assistant with tool call -> model with FunctionCall part
	modelContent := contents[1]
	if modelContent.Role != "model" {
		t.Fatalf("expected model role, got %s", modelContent.Role)
	}
	if len(modelContent.Parts) != 2 { // text + function_call
		t.Fatalf("expected 2 parts, got %d", len(modelContent.Parts))
	}
}

func TestConvertToGeminiContents_ToolResults(t *testing.T) {
	messages := []fsm.Message{
		{
			Role: "assistant",
			ToolCalls: []tools.ToolCall{
				{ID: "call_1", Function: tools.ToolCallFunction{Name: "read_file"}},
			},
		},
		{Role: "tool", Content: "file content", ToolCallID: "call_1"},
	}

	contents, _ := convertToGeminiContents(messages)
	if len(contents) != 2 {
		t.Fatalf("expected 2 contents, got %d", len(contents))
	}

	// tool results -> user with FunctionResponse parts
	toolContent := contents[1]
	if toolContent.Role != "user" {
		t.Errorf("expected user role for tool results, got %s", toolContent.Role)
	}
	if len(toolContent.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(toolContent.Parts))
	}
	if toolContent.Parts[0].FunctionResponse == nil {
		t.Error("expected FunctionResponse part")
	}
}

func TestConvertToGeminiContents_MultipleToolResults(t *testing.T) {
	messages := []fsm.Message{
		{
			Role: "assistant",
			ToolCalls: []tools.ToolCall{
				{ID: "call_1", Function: tools.ToolCallFunction{Name: "read_file"}},
				{ID: "call_2", Function: tools.ToolCallFunction{Name: "write_file"}},
			},
		},
		{Role: "tool", Content: "content a", ToolCallID: "call_1"},
		{Role: "tool", Content: "content b", ToolCallID: "call_2"},
	}

	contents, _ := convertToGeminiContents(messages)
	if len(contents) != 2 {
		t.Fatalf("expected 2 contents, got %d", len(contents))
	}

	// multiple tool results merged into single user message
	toolContent := contents[1]
	if len(toolContent.Parts) != 2 {
		t.Errorf("expected 2 function response parts, got %d", len(toolContent.Parts))
	}
}

func TestConvertToGeminiContents_UnknownToolName(t *testing.T) {
	messages := []fsm.Message{
		{Role: "tool", Content: "orphan result", ToolCallID: "unknown_id"},
	}

	contents, _ := convertToGeminiContents(messages)
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}

	// unknown tool name should fallback to "unknown"
	if contents[0].Parts[0].FunctionResponse.Name != "unknown" {
		t.Errorf("expected fallback name 'unknown', got %s", contents[0].Parts[0].FunctionResponse.Name)
	}
}

func TestNewGeminiClient(t *testing.T) {
	client := NewGeminiClient("test-api-key")
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.APIKey != "test-api-key" {
		t.Errorf("expected API key 'test-api-key', got %s", client.APIKey)
	}
}
