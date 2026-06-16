package clients

import (
	"zoomClient/fsm"
	"zoomClient/tools"
)

// ChatClient 抽象不同 LLM 服务的聊天能力
//
// 具体实现负责：
//   - 将通用的 []tools.Tool 转换为对应 LLM所需的 tool schema；
//   - 将通用的 []fsm.Message 转换为对应 LLM 的 message 协议；
//   - 调用 LLM API 并将响应转换为 *ChatResponse；
//   - 将工具调用参数统一解析为 map
type ChatClient interface {
	Chat(model string, messages []fsm.Message, toolList []tools.Tool, options map[string]interface{}) (*ChatResponse, error)
}
