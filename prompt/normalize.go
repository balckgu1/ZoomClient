package prompt

import "zoomClient/clients"

// normalizeMessages 统一消息格式：
//  1. 去除首条 system 消息（system prompt 由 Builder 独立生成）
//  2. 滤掉空内容消息（但保留有 ToolCalls 的 assistant 消息）
func (p *MessagePipeline) normalizeMessages(raw []clients.Message) []clients.Message {
	var result []clients.Message
	for i, msg := range raw {
		// 跳过首条 system 消息
		if i == 0 && msg.Role == "system" {
			continue
		}
		// 跳过空内容消息，但 assistant 的 tool_calls 消息 content 可能为空需保留
		if msg.Content == nil || msg.Content == "" {
			if len(msg.ToolCalls) == 0 {
				continue
			}
		}
		result = append(result, msg)
	}
	return result
}
