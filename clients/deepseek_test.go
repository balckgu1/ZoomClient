package clients

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"zoomClient/fsm"
	"zoomClient/tools"
)

// fakeTool 仅用于单元测试，实现 tools.Tool 接口。
type fakeTool struct {
	name        string
	description string
	parameters  map[string]interface{}
}

func (f fakeTool) Name() string                       { return f.name }
func (f fakeTool) Description() string                { return f.description }
func (f fakeTool) Parameters() map[string]interface{} { return f.parameters }
func (f fakeTool) Call(_ map[string]interface{}, _ *tools.ToolContext) tools.ToolResult {
	return tools.ToolResult{Ok: true, Content: "ok"}
}

// TestBuildDeepSeekTools 验证将通用 Tool 列表转换为 DeepSeek 协议格式时字段映射正确。
func TestBuildDeepSeekTools(t *testing.T) {
	toolList := []tools.Tool{
		fakeTool{
			name:        "get_weather",
			description: "查询天气",
			parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"location": map[string]interface{}{"type": "string"},
				},
			},
		},
	}

	result := BuildDeepSeekTools(toolList)
	if len(result) != 1 {
		t.Fatalf("期望返回 1 个工具，实际为 %d", len(result))
	}
	if result[0].Type != "function" {
		t.Errorf("type 字段应为 function，实际为 %s", result[0].Type)
	}
	if result[0].Function.Name != "get_weather" {
		t.Errorf("函数名称应为 get_weather，实际为 %s", result[0].Function.Name)
	}
	if result[0].Function.Description != "查询天气" {
		t.Errorf("描述字段未正确传递")
	}
	if result[0].Function.Parameters == nil {
		t.Errorf("parameters 字段不应为 nil")
	}
}

// TestBuildDeepSeekToolsEmpty 验证空入参时返回空切片而不是 nil panic。
func TestBuildDeepSeekToolsEmpty(t *testing.T) {
	result := BuildDeepSeekTools(nil)
	if len(result) != 0 {
		t.Errorf("空输入应返回长度为 0 的切片，实际长度 %d", len(result))
	}
}

// TestConvertToDeepSeekMessages 验证消息列表转换：
//  1. 普通消息字段透传；
//  2. assistant 工具调用中的 arguments 被序列化为 JSON 字符串；
//  3. tool 角色消息的 ToolCallID 被正确携带。
func TestConvertToDeepSeekMessages(t *testing.T) {
	messages := []fsm.Message{
		{Role: "system", Content: "你是助手"},
		{Role: "user", Content: "杭州天气如何？"},
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []tools.ToolCall{
				{
					ID: "call_123",
					Function: tools.ToolCallFunction{
						Name:      "get_weather",
						Arguments: map[string]interface{}{"location": "Hangzhou"},
					},
				},
			},
		},
		{
			Role:       "tool",
			Content:    "24℃",
			ToolCallID: "call_123",
		},
	}

	dsMessages := convertToDeepSeekMessages(messages)
	if len(dsMessages) != 4 {
		t.Fatalf("期望 4 条消息，实际为 %d", len(dsMessages))
	}

	// 校验 assistant 消息中的工具调用
	asst := dsMessages[2]
	if len(asst.ToolCalls) != 1 {
		t.Fatalf("assistant 应有 1 个工具调用，实际 %d", len(asst.ToolCalls))
	}
	if asst.ToolCalls[0].ID != "call_123" {
		t.Errorf("工具调用 ID 应为 call_123，实际 %s", asst.ToolCalls[0].ID)
	}
	if asst.ToolCalls[0].Type != "function" {
		t.Errorf("工具调用 Type 应为 function，实际 %s", asst.ToolCalls[0].Type)
	}
	// arguments 必须是 JSON 字符串而非对象
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(asst.ToolCalls[0].Function.Arguments), &parsed); err != nil {
		t.Fatalf("arguments 字段应为可解析的 JSON 字符串，解析失败: %v", err)
	}
	if parsed["location"] != "Hangzhou" {
		t.Errorf("arguments 中 location 字段不正确: %+v", parsed)
	}

	// 校验 tool 消息携带 tool_call_id
	toolMsg := dsMessages[3]
	if toolMsg.ToolCallID != "call_123" {
		t.Errorf("tool 消息的 ToolCallID 应为 call_123，实际 %s", toolMsg.ToolCallID)
	}
}

// TestConvertFromDeepSeekToolCalls 验证响应中的 arguments JSON 字符串能被正确解析为 map。
func TestConvertFromDeepSeekToolCalls(t *testing.T) {
	dsCalls := []DeepSeekToolCall{
		{
			ID:   "call_abc",
			Type: "function",
			Function: DeepSeekToolCallFunction{
				Name:      "get_weather",
				Arguments: `{"location":"Beijing"}`,
			},
		},
	}

	calls := convertFromDeepSeekToolCalls(dsCalls)
	if len(calls) != 1 {
		t.Fatalf("应解析出 1 个工具调用，实际 %d", len(calls))
	}
	if calls[0].ID != "call_abc" {
		t.Errorf("ID 透传失败，实际 %s", calls[0].ID)
	}
	if calls[0].Function.Name != "get_weather" {
		t.Errorf("函数名错误，实际 %s", calls[0].Function.Name)
	}
	if calls[0].Function.Arguments["location"] != "Beijing" {
		t.Errorf("arguments 解析错误，实际 %+v", calls[0].Function.Arguments)
	}
}

// TestConvertFromDeepSeekToolCallsEmptyArgs 验证 arguments 为空字符串时不应 panic 且参数为空 map。
func TestConvertFromDeepSeekToolCallsEmptyArgs(t *testing.T) {
	dsCalls := []DeepSeekToolCall{
		{
			ID:       "call_empty",
			Type:     "function",
			Function: DeepSeekToolCallFunction{Name: "noop", Arguments: ""},
		},
	}
	calls := convertFromDeepSeekToolCalls(dsCalls)
	if len(calls) != 1 {
		t.Fatalf("应解析出 1 个工具调用")
	}
	if calls[0].Function.Arguments == nil {
		t.Errorf("Arguments 不应为 nil，应为可用的空 map")
	}
}

// TestConvertFromDeepSeekToolCallsNil 验证 nil 入参返回 nil。
func TestConvertFromDeepSeekToolCallsNil(t *testing.T) {
	if got := convertFromDeepSeekToolCalls(nil); got != nil {
		t.Errorf("nil 输入应返回 nil，实际 %+v", got)
	}
}

// TestDeepSeekClient_Chat_Success 通过 httptest 模拟 DeepSeek API，验证完整请求—响应链路。
func TestDeepSeekClient_Chat_Success(t *testing.T) {
	// 构造模拟服务器，校验请求并返回固定响应
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 校验请求路径与鉴权头
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("请求路径错误: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization 头错误: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type 头错误: %s", r.Header.Get("Content-Type"))
		}

		// 校验请求体
		body, _ := io.ReadAll(r.Body)
		var req DeepSeekChatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("请求体解析失败: %v", err)
		}
		if req.Model != "deepseek-chat" {
			t.Errorf("模型名错误: %s", req.Model)
		}
		if req.Stream != false {
			t.Errorf("Stream 应为 false")
		}

		// 返回带工具调用的响应
		resp := DeepSeekChatResponse{
			ID:    "resp_1",
			Model: "deepseek-chat",
			Choices: []DeepSeekChoice{
				{
					Index: 0,
					Message: DeepSeekMessage{
						Role:    "assistant",
						Content: "",
						ToolCalls: []DeepSeekToolCall{
							{
								ID:   "call_xyz",
								Type: "function",
								Function: DeepSeekToolCallFunction{
									Name:      "get_weather",
									Arguments: `{"location":"Shanghai"}`,
								},
							},
						},
					},
					FinishReason: "tool_calls",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewDeepSeekClient(server.URL, "test-key")
	messages := []fsm.Message{{Role: "user", Content: "上海天气？"}}
	result, err := client.Chat("deepseek-chat", messages, nil, map[string]interface{}{
		"temperature": 0.5,
	})
	if err != nil {
		t.Fatalf("Chat 调用失败: %v", err)
	}
	if result.Message.Role != "assistant" {
		t.Errorf("响应角色错误: %s", result.Message.Role)
	}
	if len(result.Message.ToolCalls) != 1 {
		t.Fatalf("应解析出 1 个工具调用")
	}
	if result.Message.ToolCalls[0].ID != "call_xyz" {
		t.Errorf("工具调用 ID 错误: %s", result.Message.ToolCalls[0].ID)
	}
	if result.Message.ToolCalls[0].Function.Arguments["location"] != "Shanghai" {
		t.Errorf("arguments 解析错误: %+v", result.Message.ToolCalls[0].Function.Arguments)
	}
}

// TestDeepSeekClient_Chat_HTTPError 验证非 200 响应被识别为错误。
func TestDeepSeekClient_Chat_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer server.Close()

	client := NewDeepSeekClient(server.URL, "bad-key")
	_, err := client.Chat("deepseek-chat", nil, nil, nil)
	if err == nil {
		t.Fatalf("非 200 响应应返回错误")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("错误信息中应包含状态码 401，实际: %v", err)
	}
}

// TestDeepSeekClient_Chat_EmptyChoices 验证响应中 choices 为空时返回错误。
func TestDeepSeekClient_Chat_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","model":"deepseek-chat","choices":[]}`))
	}))
	defer server.Close()

	client := NewDeepSeekClient(server.URL, "test-key")
	_, err := client.Chat("deepseek-chat", nil, nil, nil)
	if err == nil {
		t.Fatalf("choices 为空时应返回错误")
	}
	if !strings.Contains(err.Error(), "choices") {
		t.Errorf("错误信息应提到 choices，实际: %v", err)
	}
}

// TestDeepSeekClient_Chat_InvalidJSON 验证响应 JSON 不合法时返回错误。
func TestDeepSeekClient_Chat_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not a json`))
	}))
	defer server.Close()

	client := NewDeepSeekClient(server.URL, "test-key")
	_, err := client.Chat("deepseek-chat", nil, nil, nil)
	if err == nil {
		t.Fatalf("非法 JSON 响应应返回错误")
	}
}
