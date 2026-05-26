package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"zoomClient/fsm"
	"zoomClient/tools"

	"google.golang.org/genai"
)

// ===================== 客户端定义 =====================

// GeminiClient Gemini 聊天客户端，使用官方 google.golang.org/genai SDK。
type GeminiClient struct {
	APIKey string
}

// NewGeminiClient 创建新的 Gemini 客户端。
func NewGeminiClient(apiKey string) *GeminiClient {
	return &GeminiClient{APIKey: apiKey}
}

// ===================== 转换辅助函数 =====================

// geminiContentStr 将 fsm.Message.Content 安全转为字符串。
func geminiContentStr(c interface{}) string {
	switch v := c.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// paramsToGeminiSchema 将 JSON Schema map 递归转换为 *genai.Schema。
func paramsToGeminiSchema(m map[string]interface{}) *genai.Schema {
	schema := &genai.Schema{}

	if t, ok := m["type"].(string); ok {
		switch t {
		case "string":
			schema.Type = genai.TypeString
		case "number":
			schema.Type = genai.TypeNumber
		case "integer":
			schema.Type = genai.TypeInteger
		case "boolean":
			schema.Type = genai.TypeBoolean
		case "array":
			schema.Type = genai.TypeArray
		case "object":
			schema.Type = genai.TypeObject
		}
	}
	if desc, ok := m["description"].(string); ok {
		schema.Description = desc
	}
	if props, ok := m["properties"].(map[string]interface{}); ok {
		schema.Properties = make(map[string]*genai.Schema)
		for k, v := range props {
			if propMap, ok := v.(map[string]interface{}); ok {
				schema.Properties[k] = paramsToGeminiSchema(propMap)
			}
		}
	}
	switch req := m["required"].(type) {
	case []string:
		schema.Required = req
	case []interface{}:
		for _, r := range req {
			if s, ok := r.(string); ok {
				schema.Required = append(schema.Required, s)
			}
		}
	}
	return schema
}

// buildGeminiTools 将通用工具列表转换为 Gemini SDK 的 []*genai.Tool 格式。
func buildGeminiTools(toolList []tools.Tool) []*genai.Tool {
	fds := make([]*genai.FunctionDeclaration, 0, len(toolList))
	for _, t := range toolList {
		fds = append(fds, &genai.FunctionDeclaration{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  paramsToGeminiSchema(t.Parameters()),
		})
	}
	return []*genai.Tool{{FunctionDeclarations: fds}}
}

// buildIDToNameMap 从消息历史中构建 ToolCallID → 函数名 的映射，
// 用于将 role:"tool" 消息转换为 FunctionResponse 时查找函数名。
func buildIDToNameMap(messages []fsm.Message) map[string]string {
	m := make(map[string]string)
	for _, msg := range messages {
		if msg.Role == "assistant" {
			for _, tc := range msg.ToolCalls {
				m[tc.ID] = tc.Function.Name
			}
		}
	}
	return m
}

// convertToGeminiContents 将内部 fsm.Message 列表转换为 Gemini Contents。
// 同时返回提取出的 system 文本。
//
// 处理规则：
//  1. system    → 提取为 systemText，跳过
//  2. user      → Content{Role:"user", Parts:[{Text:...}]}
//  3. assistant → Content{Role:"model", Parts:[{Text:...}, {FunctionCall:...}]}
//  4. tool      → 收集连续多条，合并为单条 Content{Role:"user", Parts:[{FunctionResponse:...}...]}
func convertToGeminiContents(messages []fsm.Message) ([]*genai.Content, string) {
	var contents []*genai.Content
	var systemText string
	idToName := buildIDToNameMap(messages)

	i := 0
	for i < len(messages) {
		msg := messages[i]
		switch msg.Role {
		case "system":
			systemText = geminiContentStr(msg.Content)
			i++

		case "user":
			contents = append(contents, &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{{Text: geminiContentStr(msg.Content)}},
			})
			i++

		case "assistant":
			var parts []*genai.Part
			if text := geminiContentStr(msg.Content); text != "" {
				parts = append(parts, &genai.Part{Text: text})
			}
			for _, tc := range msg.ToolCalls {
				parts = append(parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						Name: tc.Function.Name,
						Args: tc.Function.Arguments,
					},
				})
			}
			if len(parts) == 0 {
				parts = []*genai.Part{{Text: ""}}
			}
			contents = append(contents, &genai.Content{Role: "model", Parts: parts})
			i++

		case "tool":
			// 收集连续 tool 消息 → 合并为一条 user 消息（含多个 FunctionResponsePart）
			var fnParts []*genai.Part
			for i < len(messages) && messages[i].Role == "tool" {
				name := idToName[messages[i].ToolCallID]
				if name == "" {
					name = "unknown"
				}
				fnParts = append(fnParts, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						Name:     name,
						Response: map[string]any{"result": geminiContentStr(messages[i].Content)},
					},
				})
				i++
			}
			contents = append(contents, &genai.Content{Role: "user", Parts: fnParts})

		default:
			i++
		}
	}
	return contents, systemText
}

// ===================== Chat 方法实现 =====================

// Chat 实现 ChatClient 接口，向 Gemini 发起一次内容生成请求。
//
// 实现要点：
//  1. system 消息提取为 GenerateContentConfig.SystemInstruction；
//  2. assistant 角色映射为 Gemini 的 "model" 角色；
//  3. 连续 tool 消息合并为单条含 FunctionResponsePart 的 user 消息；
//  4. 响应 Parts 中 Text 拼接为内容，FunctionCall 转为 ToolCall。
func (c *GeminiClient) Chat(model string, messages []fsm.Message, toolList []tools.Tool, options map[string]interface{}) (*ChatResponse, error) {
	ctx := context.Background()

	// 1. 创建 SDK 客户端（每次调用创建，保持构造函数零错误的惯例）
	genaiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("Gemini 创建客户端失败: %w", err)
	}

	// 2. 协议转换
	contents, systemText := convertToGeminiContents(messages)
	geminiTools := buildGeminiTools(toolList)

	// 3. 构造 GenerateContentConfig
	cfg := &genai.GenerateContentConfig{
		Tools: geminiTools,
	}
	if systemText != "" {
		cfg.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: systemText}},
		}
	}
	if temp, ok := options["temperature"].(float64); ok {
		t := float32(temp)
		cfg.Temperature = &t
	}

	// 4. 发起请求
	resp, err := genaiClient.Models.GenerateContent(ctx, model, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("Gemini API 请求失败: %w", err)
	}
	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("Gemini 响应中无有效候选结果")
	}

	// 5. 解析响应：遍历第一个候选的 Parts
	candidate := resp.Candidates[0]
	var textParts []string
	var toolCalls []tools.ToolCall

	if candidate.Content != nil {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
			if part.FunctionCall != nil {
				toolCalls = append(toolCalls, tools.ToolCall{
					// Gemini 不返回工具调用 ID，使用函数名+序号构造唯一 ID
					ID: fmt.Sprintf("gemini-%s-%d", part.FunctionCall.Name, len(toolCalls)),
					Function: tools.ToolCallFunction{
						Name:      part.FunctionCall.Name,
						Arguments: part.FunctionCall.Args,
					},
				})
			}
		}
	}

	// 6. 归一化为内部 ChatResponse
	chatResp := &ChatResponse{
		Model: model,
		Done:  true,
		Message: fsm.Message{
			Role:      "assistant",
			Content:   strings.Join(textParts, ""),
			ToolCalls: toolCalls,
		},
	}
	return chatResp, nil
}
