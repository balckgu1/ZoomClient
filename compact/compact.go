package compact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"zoomClient/clients"
	"zoomClient/fsm"
	"zoomClient/utils"
)

// microCompactPlaceholder 替换旧 ToolResult 用的占位文本
const microCompactPlaceholder = "[Earlier tool result omitted for brevity]"

// CompactConfig Context compact configuration.
type CompactConfig struct {
	PersistThreshold      int    // 单条ToolResult超过该字节数则落盘
	PreviewBytes          int    // 落盘后保留的预览字节数
	KeepRecentToolResults int    // 保留最近 N 条工具结果的完整内容
	ContextLimit          int    // 估算的总字节阈值，超过则触发整体摘要
	PersistDir            string // 落盘目录
}

// DefaultConfig Returns a set of default values for the config.
func DefaultConfig(cfg utils.Config) *CompactConfig {
	return &CompactConfig{
		PersistThreshold:      cfg.Compact.PersistThreshold,
		PreviewBytes:          cfg.Compact.PreviewBytes,
		KeepRecentToolResults: cfg.Compact.KeepRecentToolResults,
		ContextLimit:          cfg.Compact.ContextLimit,
		PersistDir:            cfg.Compact.PersistDir,
	}
}

// CompactState 显式维护的压缩状态
type CompactState struct {
	HasCompacted bool     // 之前是否已经做过完整压缩
	LastSummary  string   // 最近一次完整压缩生成的摘要
	RecentFiles  []string // 最近碰过的文件，便于压缩后继续追踪
}

// CompactManager 把三层压缩能力收敛在一起，便于 agentLoop 持有并按需调用。
type CompactManager struct {
	Config               *CompactConfig
	State                *CompactState
	client               clients.ChatClient // 用于第 3 层调模型生成摘要
	model                string             // 摘要使用的模型名
	pendingManualCompact bool               // 本轮内的某个工具调用（或外部）请求了一次完整压缩
}

// NewManager 创建 Manager，需要传入用于生成摘要的 ChatClient 与模型名。
func NewCompactManager(cfg *CompactConfig, client clients.ChatClient, model string) *CompactManager {
	return &CompactManager{
		Config: cfg,
		State:  &CompactState{},
		client: client,
		model:  model,
	}
}

// UpdateModel 热更新 CompactManager 使用的客户端和模型名。
func (m *CompactManager) UpdateModel(client clients.ChatClient, model string) {
	m.client = client
	m.model = model
}

// PersistLargeOutput 若ToolResult过大，将工具结果写入磁盘，返回占位文本
func (m *CompactManager) PersistLargeOutput(toolUseID string, output string) string {
	// 如果ToolResult <= 阈值，原样返回
	if len(output) <= m.Config.PersistThreshold {
		return output
	}

	storedPath, err := m.saveToDisk(toolUseID, output)
	if err != nil {
		// 写入磁盘失败时，返回原文
		return output
	}

	preview := truncateUTF8(output, m.Config.PreviewBytes)

	return fmt.Sprintf(
		"<persisted-output>\nFull output saved to: %s\nPreview:\n%s\n</persisted-output>",
		storedPath, preview,
	)
}

// truncateUTF8 安全地把 s 截断到不超过 max 字节，且不切断多字节字符
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	end := max
	// end 此时 < len(s)，preview[end] 安全；向前回退到 rune 边界
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end]
}

// saveToDisk 把单条tool output写入 PersistDir 持久化存储
func (m *CompactManager) saveToDisk(toolUseID, output string) (string, error) {
	if err := os.MkdirAll(m.Config.PersistDir, 0o755); err != nil {
		return "", err
	}
	// 工具 ID 可能为空（Ollama 后端没有 ID），用占位补齐
	if toolUseID == "" {
		toolUseID = "tool"
	}
	fname := fmt.Sprintf("%s-%d.txt", toolUseID, time.Now().UnixNano())
	full := filepath.Join(m.Config.PersistDir, fname)
	if err := os.WriteFile(full, []byte(output), 0o644); err != nil {
		return "", err
	}
	return full, nil
}

// MicroCompact 把较早的 ToolResult替换为占位，只保留最近 KeepRecentToolResults 条的完整内容
func (m *CompactManager) MicroCompact(messages []fsm.Message) []fsm.Message {
	keep := m.Config.KeepRecentToolResults

	// 收集message中所有 role=tool 消息的下标
	toolIdxs := make([]int, 0, keep)
	for i, msg := range messages {
		if msg.Role == "tool" {
			toolIdxs = append(toolIdxs, i)
		}
	}

	// 如果当前的 tool数量小于等于 KeepRecentToolResults，返回原样message
	if len(toolIdxs) <= keep {
		return messages
	}

	// 复制一份，避免修改原始 slice
	result := make([]fsm.Message, len(messages))
	copy(result, messages)

	// 把前面 (len - keep) 条替换成占位
	cutoff := len(toolIdxs) - keep
	for _, idx := range toolIdxs[:cutoff] {
		if s, ok := result[idx].Content.(string); ok && s == microCompactPlaceholder {
			// 已是占位，跳过
			continue
		}
		result[idx].Content = microCompactPlaceholder
	}
	return result
}

// EstimateSize 估算一份消息历史在上下文里占用的字节数
func (m *CompactManager) EstimateSize(messages []fsm.Message) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Role)
		switch v := msg.Content.(type) {
		case string:
			total += len(v)
		default:
			b, _ := json.Marshal(v)
			total += len(b)
		}
		total += len(msg.ReasoningContent)
		total += len(msg.ToolCallID)
		for _, tc := range msg.ToolCalls {
			total += len(tc.Function.Name)
			b, _ := json.Marshal(tc.Function.Arguments)
			total += len(b)
		}
	}
	return total
}

// ShouldAutoCompact 由AgentLoop每轮结束时调用，决定是否要触发完整压缩
func (m *CompactManager) ShouldAutoCompact(messages []fsm.Message) bool {
	// 如果标记了 pendingManualCompact，直接返回true
	if m.pendingManualCompact {
		return true
	}
	return m.EstimateSize(messages) > m.Config.ContextLimit
}

// CompactHistory 调模型生成一份摘要，用 system + 摘要消息替换原始长历史。
// 若原始历史尾部存在带 tool_calls 的 assistant 消息及其配对的 tool 结果，
// 会将它们一并保留在摘要之后，确保 OpenAI 协议的配对关系不被破坏。
func (m *CompactManager) CompactHistory(messages []fsm.Message) ([]fsm.Message, error) {
	summary, err := m.summarize(messages)
	if err != nil {
		return messages, err
	}

	// 消费主动压缩标记
	m.pendingManualCompact = false

	m.State.HasCompacted = true
	m.State.LastSummary = summary

	// 保留原始 system prompt
	newMessages := make([]fsm.Message, 0, 4)
	if len(messages) > 0 && messages[0].Role == "system" {
		newMessages = append(newMessages, messages[0])
	}
	newMessages = append(newMessages, fsm.Message{
		Role:    "user",
		Content: "This conversation was compacted for continuity.\n\n" + summary,
	})

	// 保留尾部未完成的 assistant(tool_calls) 及其配对的 ToolResult
	boundary := findPendingToolCallBoundary(messages)
	if boundary >= 0 {
		newMessages = append(newMessages, messages[boundary:]...)
	}

	return newMessages, nil
}

// findPendingToolCallBoundary 从后往前找到最后一个 len(tool_calls)>0 的 assistant 消息下标
func findPendingToolCallBoundary(messages []fsm.Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && len(messages[i].ToolCalls) > 0 {
			return i
		}
	}
	return -1
}

// summarize 调一次模型生成摘要。
func (m *CompactManager) summarize(messages []fsm.Message) (string, error) {
	prompt := `Please read the dialogue history below and output a 'Continuous Compression Summary'. The following key points must be kept (none of which are missing):
1. Current task objective
2. Completed key actions
3. The file path that has been modified or viewed with emphasis
4. Key decisions and constraints
5. What should be done next
Requirement: Only output the main body of the abstract, without any explanation, no marking down of the title, and no small talk.`

	history := renderForSummary(messages)
	summaryMsgs := []fsm.Message{
		{Role: "system", Content: prompt},
		{Role: "user", Content: history},
	}

	resp, err := m.client.Chat(m.model, summaryMsgs, nil, map[string]interface{}{
		"temperature": 0.3,
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}
	if s, ok := resp.Message.Content.(string); ok && s != "" {
		return s, nil
	}
	return "", fmt.Errorf("summary response is empty or type is incorrect")
}

// renderForSummary 把 messages 渲染成一段纯文本，喂给摘要请求
func renderForSummary(messages []fsm.Message) string {
	var sb strings.Builder
	for _, msg := range messages {
		if msg.Role == "system" {
			continue
		}
		sb.WriteString("[")
		sb.WriteString(msg.Role)
		sb.WriteString("] ")
		switch v := msg.Content.(type) {
		case string:
			sb.WriteString(v)
		default:
			b, _ := json.Marshal(v)
			sb.Write(b)
		}
		if msg.ReasoningContent != "" {
			sb.WriteString("  [reasoning] ")
			sb.WriteString(msg.ReasoningContent)
			sb.WriteString("\n")
		}
		for _, tc := range msg.ToolCalls {
			sb.WriteString("\n  -> tool_call: ")
			sb.WriteString(tc.Function.Name)
			args, _ := json.Marshal(tc.Function.Arguments)
			sb.WriteString(" args=")
			sb.Write(args)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// RequestManualCompact 标记"需执行一次完整压缩"。
func (m *CompactManager) RequestManualCompact() {
	m.pendingManualCompact = true
}
