package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"zoomClient/fsm"
	"zoomClient/tools"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// ===================== 客户端定义 =====================

// AnthropicClient Anthropic 聊天客户端，使用官方 anthropic-sdk-go。
type AnthropicClient struct {
	client anthropic.Client
}

// NewAnthropicClient 创建新的 Anthropic 客户端。
func NewAnthropicClient(apiKey string) *AnthropicClient {
	return &AnthropicClient{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}
}

// ===================== 转换辅助函数 =====================

// buildAnthropicTools 将通用工具列表转换为 Anthropic SDK 的 ToolUnionParam 格式。
func buildAnthropicTools(toolList []tools.Tool) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, 0, len(toolList))
	for _, t := range toolList {
		params := t.Parameters()
		schema := anthropic.ToolInputSchemaParam{
			Properties: params["properties"],
		}
		extras := map[string]any{"type": "object"}
		if req := params["required"]; req != nil {
			extras["required"] = req
		}
		schema.SetExtraFields(extras)

		tp := anthropic.ToolParam{
			Name:        t.Name(),
			Description: anthropic.String(t.Description()),
			InputSchema: schema,
		}
		result = append(result, anthropic.ToolUnionParam{OfTool: &tp})
	}
	return result
}

// anthropicContentStr 将 fsm.Message.Content 安全转为字符串。
func anthropicContentStr(c interface{}) string {
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

// convertToAnthropicMessages 将内部 fsm.Message 列表转换为 Anthropic 协议消息列表。
// 同时返回提取出的 system 文本（Anthropic 要求 system 作为顶层参数）。
//
// 处理规则：
//  1. system  → 提取为 systemText，跳过不加入数组
//  2. user    → NewUserMessage(NewTextBlock(...))
//  3. assistant + ToolCalls → AssistantMessageParam，含 ToolUseBlock + 可选 TextBlock
//  4. tool    → 收集连续多条，合并为单条 NewUserMessage(NewToolResultBlock(...)...)
func convertToAnthropicMessages(messages []fsm.Message) ([]anthropic.MessageParam, string) {
	var result []anthropic.MessageParam
	var systemText string

	i := 0
	for i < len(messages) {
		msg := messages[i]
		switch msg.Role {
		case "system":
			systemText = anthropicContentStr(msg.Content)
			i++

		case "user":
			result = append(result, anthropic.NewUserMessage(
				anthropic.NewTextBlock(anthropicContentStr(msg.Content)),
			))
			i++

		case "assistant":
			var blocks []anthropic.ContentBlockParamUnion
			if text := anthropicContentStr(msg.Content); text != "" {
				blocks = append(blocks, anthropic.NewTextBlock(text))
			}
			for _, tc := range msg.ToolCalls {
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: tc.Function.Arguments,
					},
				})
			}
			if len(blocks) == 0 {
				blocks = append(blocks, anthropic.NewTextBlock(""))
			}
			result = append(result, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleAssistant,
				Content: blocks,
			})
			i++

		case "tool":
			// 收集连续 tool 消息 → 合并为一条 user 消息（含多个 ToolResultBlock）
			var toolResultBlocks []anthropic.ContentBlockParamUnion
			for i < len(messages) && messages[i].Role == "tool" {
				toolResultBlocks = append(toolResultBlocks,
					anthropic.NewToolResultBlock(
						messages[i].ToolCallID,
						anthropicContentStr(messages[i].Content),
						false,
					),
				)
				i++
			}
			result = append(result, anthropic.NewUserMessage(toolResultBlocks...))

		default:
			i++
		}
	}
	return result, systemText
}

// ===================== Chat 方法实现 =====================

// Chat 实现 ChatClient 接口，向 Anthropic 发起一次消息补全请求。
//
// 实现要点：
//  1. system 消息提取为顶层 System 参数；
//  2. tool 结果消息合并为 user 消息的 ToolResultBlock；
//  3. 响应 ContentBlock 数组中 TextBlock 拼接为文本，ToolUseBlock 转为 ToolCall。
func (c *AnthropicClient) Chat(model string, messages []fsm.Message, toolList []tools.Tool, options map[string]interface{}) (*ChatResponse, error) {
	ctx := context.Background()

	// 1. 协议转换
	anthropicMessages, systemText := convertToAnthropicMessages(messages)
	anthropicTools := buildAnthropicTools(toolList)

	// 2. 构造请求参数
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 8096,
		Messages:  anthropicMessages,
		Tools:     anthropicTools,
	}
	if systemText != "" {
		params.System = []anthropic.TextBlockParam{{Text: systemText}}
	}

	// 3. 发起请求
	msg, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("Anthropic API 请求失败: %w", err)
	}

	// 4. 解析响应：遍历 ContentBlock 数组
	var textParts []string
	var toolCalls []tools.ToolCall

	for _, block := range msg.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			if b.Text != "" {
				textParts = append(textParts, b.Text)
			}
		case anthropic.ToolUseBlock:
			argsMap := map[string]any{}
			if inputJSON, merr := json.Marshal(b.Input); merr == nil {
				_ = json.Unmarshal(inputJSON, &argsMap)
			}
			toolCalls = append(toolCalls, tools.ToolCall{
				ID: b.ID,
				Function: tools.ToolCallFunction{
					Name:      b.Name,
					Arguments: argsMap,
				},
			})
		}
	}

	// 5. 归一化为内部 ChatResponse
	chatResp := &ChatResponse{
		Model: string(msg.Model),
		Done:  true,
		Message: fsm.Message{
			Role:      "assistant",
			Content:   strings.Join(textParts, ""),
			ToolCalls: toolCalls,
		},
	}
	return chatResp, nil
}
