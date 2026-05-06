package clients

import (
	"zoomClient/fsm"
	"zoomClient/tools"
)

// ChatClient 抽象不同 LLM 服务的聊天能力。
//
// 各具体实现负责：
//  1. 将通用的 []tools.Tool 转换为对应服务商所需的 tool schema；
//  2. 将通用的 []fsm.Message 转换为对应服务商的 message 协议；
//  3. 调用 HTTP 接口并将响应归一化为 *ChatResponse；
//  4. 将工具调用参数（不同协议中可能为对象或 JSON 字符串）统一解析为 map。
type ChatClient interface {
	Chat(model string, messages []fsm.Message, toolList []tools.Tool, options map[string]interface{}) (*ChatResponse, error)
}
