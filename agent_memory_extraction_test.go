package main

// 本文件覆盖 AgentSession.runMemoryExtraction 的同步抽取行为与功能关闭安全性。
// 抽取执行本身的测试在 memory 包（memory/memory_extractor_test.go）。

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"zoomClient/clients"
	"zoomClient/fsm"
	"zoomClient/memory"
	"zoomClient/tools"

	"go.uber.org/zap"
)

// recordingChatClient 并发安全的记录型 ChatClient：
// 记录每次调用入参并返回"Nothing to save."，供抽取输入断言。
type recordingChatClient struct {
	mu     sync.Mutex
	calls  int
	inputs [][]clients.Message
}

func (c *recordingChatClient) Chat(model string, messages []clients.Message, toolList []tools.Tool, options map[string]interface{}) (*clients.ChatResponse, error) {
	c.mu.Lock()
	c.calls++
	snapshot := make([]clients.Message, len(messages))
	copy(snapshot, messages)
	c.inputs = append(c.inputs, snapshot)
	c.mu.Unlock()
	return &clients.ChatResponse{
		Model:   "test",
		Message: clients.Message{Role: "assistant", Content: "Nothing to save."},
	}, nil
}

func (c *recordingChatClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *recordingChatClient) inputAt(i int) []clients.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i < 0 || i >= len(c.inputs) {
		return nil
	}
	return c.inputs[i]
}

// buildMessages 构造 n 条内容唯一的消息（user/assistant 交替）。
// 注意 RunWithFork 会裁剪末尾的 assistant 消息，断言时优先使用偶数下标（user）内容。
func buildMessages(n int) []clients.Message {
	msgs := make([]clients.Message, n)
	for i := range msgs {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs[i] = clients.Message{
			Role:    role,
			Content: fmt.Sprintf("msg-%d. I prefer tabs over spaces, remember this detail.", i),
		}
	}
	return msgs
}

// joinedMessages 把一次 LLM 调用的入参渲染为单一字符串，供子串断言。
func joinedMessages(msgs []clients.Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString(messageContentToString(m.Content))
		sb.WriteString("\n")
	}
	return sb.String()
}

// TestRunMemoryExtractionRunsSynchronously 覆盖同步语义：
// runMemoryExtraction 返回时抽取已完成（客户端已被调用），无需等待后台。
func TestRunMemoryExtractionRunsSynchronously(t *testing.T) {
	dir := t.TempDir()
	client := &recordingChatClient{}

	s := &AgentSession{
		State:           &fsm.State{Messages: buildMessages(2)},
		Client:          client,
		ModelName:       "test-model",
		ToolCtx:         &tools.ToolContext{Logger: zap.NewNop(), SessionID: "sync-test", Ctx: context.Background()},
		MemoryExtractor: memory.NewExtractor(dir, nil),
	}
	s.runMemoryExtraction(true)

	// 同步返回后抽取应已发生
	if client.callCount() == 0 {
		t.Fatal("expected extraction to run synchronously before returning")
	}
	got := joinedMessages(client.inputAt(0))
	if !strings.Contains(got, "msg-0.") {
		t.Errorf("extraction input missing session messages: %s", got)
	}
}

// TestRunMemoryExtractionNilExtractorSafe 覆盖功能关闭（extractor 为 nil）时的安全性。
func TestRunMemoryExtractionNilExtractorSafe(t *testing.T) {
	s := &AgentSession{}
	s.runMemoryExtraction(true)  // 不应 panic
	s.runMemoryExtraction(false) // 不应 panic
}
