package compact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zoomClient/clients"
	"zoomClient/fsm"
	"zoomClient/utils"
)

// CompactConfig Context compact configuration.
type CompactConfig struct {
	PersistThreshold      int    // 第 1 层：单条工具结果超过该字节数则落盘
	PreviewBytes          int    // 第 1 层：落盘后保留的预览字节数
	KeepRecentToolResults int    // 第 2 层：保留最近 N 条工具结果的完整内容
	ContextLimit          int    // 第 3 层：估算的总字节阈值，超过则触发整体摘要
	PersistDir            string // 第 1 层：落盘目录
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
//
// 在压缩之后，主循环和摘要本身仍然能交代清楚"上一次压缩做了什么 / 最近碰过哪些文件"。
type CompactState struct {
	HasCompacted bool     // 这一会话之前是否已经做过完整压缩
	LastSummary  string   // 最近一次完整压缩生成的摘要
	RecentFiles  []string // 最近碰过的文件，便于压缩后继续追踪
}

// CompactManager 把三层压缩能力收敛在一起，便于 agentLoop 持有并按需调用。
type CompactManager struct {
	Config               *CompactConfig
	State                *CompactState
	client               clients.ChatClient // 用于第 3 层调模型生成摘要
	model                string             // 摘要使用的模型名
	pendingManualCompact bool
	// pendingManualCompact 表示：本轮内的某个工具调用（或外部）请求了一次完整压缩。
	// 真正的压缩动作由 agentLoop 在"工具结果都已 append 完"之后再执行，
	// 这样才不会破坏 OpenAI 协议中 assistant(tool_calls) 与 tool 消息的配对关系。
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

// ===================== 第 1 层：大工具结果落盘 + 预览 =====================

// PersistLargeOutput 将工具结果写入磁盘，返回占位文本
//   - 输出体积 <= 阈值：原样返回，不做任何改动
//   - 输出体积  > 阈值：把全文写到磁盘，返回一段带预览的占位文本，
func (m *CompactManager) PersistLargeOutput(toolUseID string, output string) string {
	if len(output) <= m.Config.PersistThreshold {
		return output
	}

	storedPath, err := m.saveToDisk(toolUseID, output)
	if err != nil {
		// 落盘失败时，宁可回退到原文，也不要让主循环崩掉。
		return output
	}

	preview := output
	if len(preview) > m.Config.PreviewBytes {
		preview = preview[:m.Config.PreviewBytes]
	}

	return fmt.Sprintf(
		"<persisted-output>\nFull output saved to: %s\nPreview:\n%s\n</persisted-output>",
		storedPath, preview,
	)
}

// saveToDisk 把单条tool output写入 PersistDir
func (m *CompactManager) saveToDisk(toolUseID, output string) (string, error) {
	if err := os.MkdirAll(m.Config.PersistDir, 0o755); err != nil {
		return "", err
	}
	// 工具 ID 可能为空（Ollama 协议下没有 ID），用占位补齐
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

// ===================== 第 2 层：旧工具结果做微压缩 =====================

// microCompactPlaceholder 替换旧 tool 消息内容用的占位文本。
const microCompactPlaceholder = "[Earlier tool result omitted for brevity]"

// MicroCompact 把"较早"的 tool 消息替换为占位，只保留最近 KeepRecentToolResults 条的完整内容
func (m *CompactManager) MicroCompact(messages []fsm.Message) []fsm.Message {
	keep := m.Config.KeepRecentToolResults

	// 收集所有 tool 消息的下标
	toolIdxs := make([]int, 0)
	for i, msg := range messages {
		if msg.Role == "tool" {
			toolIdxs = append(toolIdxs, i)
		}
	}
	if len(toolIdxs) <= keep {
		return messages
	}

	// 把前面 (len - keep) 条替换成占位
	cutoff := len(toolIdxs) - keep
	for _, idx := range toolIdxs[:cutoff] {
		if s, ok := messages[idx].Content.(string); ok && s == microCompactPlaceholder {
			continue // 已是占位，跳过
		}
		messages[idx].Content = microCompactPlaceholder
	}
	return messages
}

// ===================== 第 3 层：整体历史过长时做完整压缩 =====================

// EstimateSize 估算一份消息历史在上下文里占用的字节数。
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
		for _, tc := range msg.ToolCalls {
			total += len(tc.Function.Name)
			b, _ := json.Marshal(tc.Function.Arguments)
			total += len(b)
		}
	}
	return total
}

// ShouldAutoCompact 主循环每轮结束时调用，决定是否要触发第 3 层完整压缩。
// 同时考虑"自动阈值触发"和"工具/用户手动请求"。
func (m *CompactManager) ShouldAutoCompact(messages []fsm.Message) bool {
	if m.pendingManualCompact {
		return true
	}
	return m.EstimateSize(messages) > m.Config.ContextLimit
}

// CompactHistory 第 3 层：调模型生成一份"连续性摘要"，用 system + 摘要消息替换原始长历史。
func (m *CompactManager) CompactHistory(messages []fsm.Message) ([]fsm.Message, error) {
	// 消费手动压缩标记：无论成功与否，都不让它二次触发
	m.pendingManualCompact = false

	summary, err := m.summarize(messages)
	if err != nil {
		return messages, err
	}

	m.State.HasCompacted = true
	m.State.LastSummary = summary

	// 保留原始 system 消息（包含工具说明、skill 列表等上下文）
	newMessages := make([]fsm.Message, 0, 2)
	if len(messages) > 0 && messages[0].Role == "system" {
		newMessages = append(newMessages, messages[0])
	}
	newMessages = append(newMessages, fsm.Message{
		Role:    "user",
		Content: "This conversation was compacted for continuity.\n\n" + summary,
	})
	return newMessages, nil
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

// ===================== 手动压缩入口（供 compact 工具调用） =====================

// RequestManualCompact 标记"本轮结束时执行一次完整压缩"。
func (m *CompactManager) RequestManualCompact() {
	m.pendingManualCompact = true
}
