package memory

// 本文件覆盖同步增量记忆抽取器 Extractor 的行为：
// 增量游标、skip 前移、失败重试、announce 相位事件、抽取落盘。

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"zoomClient/clients"
	"zoomClient/tools"

	"go.uber.org/zap"
)

// chatStep 表示脚本化客户端依次返回的每一步结果。
type chatStep struct {
	resp *clients.ChatResponse
	err  error
}

// scriptedChatClient 脚本化 ChatClient：按步骤返回预设响应并记录每次调用入参。
// 步骤耗尽后返回无害的 "Nothing to save." 响应。
type scriptedChatClient struct {
	mu     sync.Mutex
	calls  int
	inputs [][]clients.Message
	steps  []chatStep
}

func newScriptedChatClient(steps ...chatStep) *scriptedChatClient {
	return &scriptedChatClient{steps: steps}
}

func (c *scriptedChatClient) Chat(model string, messages []clients.Message, toolList []tools.Tool, options map[string]interface{}) (*clients.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	snapshot := make([]clients.Message, len(messages))
	copy(snapshot, messages)
	c.inputs = append(c.inputs, snapshot)
	var step chatStep
	if c.calls-1 < len(c.steps) {
		step = c.steps[c.calls-1]
	} else {
		step = chatStep{resp: nothingToSaveResponse()}
	}
	return step.resp, step.err
}

func (c *scriptedChatClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *scriptedChatClient) inputAt(i int) []clients.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i < 0 || i >= len(c.inputs) {
		return nil
	}
	return c.inputs[i]
}

// nothingToSaveResponse 返回抽取子代理"无新增记忆"的结束响应。
func nothingToSaveResponse() *clients.ChatResponse {
	return &clients.ChatResponse{
		Model:   "test",
		Message: clients.Message{Role: "assistant", Content: "Nothing to save."},
	}
}

// recordingEmitter 是最小化的 emitter.Emitter 测试替身：
// 记录 EmitEmotion 与 EmitInfo 调用，其余事件 no-op。
type recordingEmitter struct {
	mu       sync.Mutex
	emotions []string
	infos    []string
}

func (e *recordingEmitter) emotionStates() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, len(e.emotions))
	copy(out, e.emotions)
	return out
}

func (e *recordingEmitter) countEmotion(state string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, s := range e.emotions {
		if s == state {
			n++
		}
	}
	return n
}

func (e *recordingEmitter) EmitSessionStart(model, logPath string)                {}
func (e *recordingEmitter) EmitSessionEnd(totalTurns int)                         {}
func (e *recordingEmitter) EmitTurnSeparator()                                    {}
func (e *recordingEmitter) EmitAssistant(text string)                             {}
func (e *recordingEmitter) EmitReasoning(text string)                             {}
func (e *recordingEmitter) EmitDone()                                             {}
func (e *recordingEmitter) EmitToolCall(name, argsPreview string)                 {}
func (e *recordingEmitter) EmitToolResult(name, content string, isError bool)     {}
func (e *recordingEmitter) EmitSubAgent(promptPreview string)                     {}
func (e *recordingEmitter) EmitHookBlocked(toolName, reason string)               {}
func (e *recordingEmitter) EmitTodoPanel(rendered string, items []tools.PlanItem) {}
func (e *recordingEmitter) EmitCompact(beforeBytes, afterBytes int)               {}
func (e *recordingEmitter) EmitError(scope, msg string)                           {}
func (e *recordingEmitter) EmitSystem(event string, data map[string]string)       {}

func (e *recordingEmitter) EmitInfo(msg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.infos = append(e.infos, msg)
}

func (e *recordingEmitter) EmitEmotion(state string, meta map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.emotions = append(e.emotions, state)
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

// extractRequest 构造交给 Extractor.Run 的抽取请求。
func extractRequest(client clients.ChatClient, msgs []clients.Message) ExtractionRequest {
	return ExtractionRequest{
		Messages: msgs,
		Client:   client,
		Model:    "test-model",
		ToolCtx:  tools.ToolContext{Logger: zap.NewNop(), SessionID: "extractor-test", Ctx: context.Background()},
	}
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

// TestMemoryCursorWindow 覆盖增量游标的有效窗口与各类失效兜底。
func TestMemoryCursorWindow(t *testing.T) {
	base := []clients.Message{
		{Role: "user", Content: "m0"},
		{Role: "assistant", Content: "m1"},
		{Role: "user", Content: "m2"},
		{Role: "assistant", Content: "m3"},
	}

	var c memoryCursor
	if got := c.window(base); len(got) != 4 {
		t.Fatalf("initial window: expected full 4, got %d", len(got))
	}

	c.advance(base[:2])
	got := c.window(base)
	if len(got) != 2 || got[0].Content != "m2" {
		t.Fatalf("incremental window: expected [m2 m3], got %+v", got)
	}

	if got := c.window(base[:1]); len(got) != 1 {
		t.Fatalf("shrunken messages: expected fallback full 1, got %d", len(got))
	}

	modified := []clients.Message{base[0], {Role: "assistant", Content: "changed"}, base[2], base[3]}
	if got := c.window(modified); len(got) != 4 {
		t.Fatalf("fingerprint mismatch: expected fallback full 4, got %d", len(got))
	}

	if got := c.window(nil); len(got) != 0 {
		t.Fatalf("empty messages: expected 0, got %d", len(got))
	}
}

// TestExtractorSkipAdvancesCursor 覆盖窗口化 skip：窗口内已手写记忆时不调 LLM，
// 且游标前移（该窗口视为已处理，不重试）。
func TestExtractorSkipAdvancesCursor(t *testing.T) {
	dir := t.TempDir()
	client := newScriptedChatClient()
	e := NewExtractor(dir, nil)

	snap := []clients.Message{
		{Role: "user", Content: "Remember that I prefer tabs over spaces in all Go code I write."},
		{Role: "assistant", ToolCalls: []tools.ToolCall{{Name: "save_memory"}}},
	}
	e.Run(extractRequest(client, snap), true)

	if got := client.callCount(); got != 0 {
		t.Fatalf("expected skip without LLM call, got %d calls", got)
	}
	if e.cursor.index != len(snap) {
		t.Fatalf("expected cursor advanced to %d, got %d", len(snap), e.cursor.index)
	}
}

// TestExtractorSkipNoAnnounce 覆盖 skip 时不推送 memory 相位事件（避免状态闪现）。
func TestExtractorSkipNoAnnounce(t *testing.T) {
	dir := t.TempDir()
	client := newScriptedChatClient()
	em := &recordingEmitter{}
	e := NewExtractor(dir, em)

	// 用户散文 < 阈值 → skip
	e.Run(extractRequest(client, []clients.Message{{Role: "user", Content: "ok"}}), true)

	if client.callCount() != 0 {
		t.Fatalf("expected skip without LLM call")
	}
	if em.countEmotion("memory") != 0 {
		t.Fatalf("skip should not announce memory phase, got %v", em.emotionStates())
	}
}

// TestExtractorAnnounceEmitsMemoryPhase 覆盖 announce 语义：
// 真正抽取前推送一次 memory 相位事件；announce=false 时不推送。
func TestExtractorAnnounceEmitsMemoryPhase(t *testing.T) {
	dir := t.TempDir()
	client := newScriptedChatClient()
	em := &recordingEmitter{}
	e := NewExtractor(dir, em)

	e.Run(extractRequest(client, buildMessages(2)), true)
	if em.countEmotion("memory") != 1 {
		t.Fatalf("announce=true should emit exactly one memory phase, got %v", em.emotionStates())
	}

	em2 := &recordingEmitter{}
	e2 := NewExtractor(dir, em2)
	e2.Run(extractRequest(client, buildMessages(2)), false)
	if em2.countEmotion("memory") != 0 {
		t.Fatalf("announce=false should not emit memory phase, got %v", em2.emotionStates())
	}
}

// TestExtractorIncrementalWindow 覆盖增量：首次抽取覆盖前 4 条后，
// 第二次仅抽取新增窗口（msg-4/msg-6），不重复已覆盖的 msg-0。
func TestExtractorIncrementalWindow(t *testing.T) {
	dir := t.TempDir()
	client := newScriptedChatClient()
	e := NewExtractor(dir, nil)

	e.Run(extractRequest(client, buildMessages(4)), false)
	e.Run(extractRequest(client, buildMessages(8)), false)

	if got := client.callCount(); got != 2 {
		t.Fatalf("expected 2 extraction calls, got %d", got)
	}
	second := joinedMessages(client.inputAt(1))
	if strings.Contains(second, "msg-0.") {
		t.Errorf("second window should not re-extract already covered messages:\n%s", second)
	}
	if !strings.Contains(second, "msg-4.") || !strings.Contains(second, "msg-6.") {
		t.Errorf("second window should cover the new increment:\n%s", second)
	}
	if e.cursor.index != 8 {
		t.Fatalf("expected cursor advanced to 8, got %d", e.cursor.index)
	}
}

// TestExtractorRetriesWindowAfterFailure 覆盖失败不前移游标：
// 第一次抽取失败后窗口保持，下次 Run 整窗重试（含旧消息），成功后游标推进。
func TestExtractorRetriesWindowAfterFailure(t *testing.T) {
	dir := t.TempDir()
	client := newScriptedChatClient(chatStep{err: errors.New("boom")})
	e := NewExtractor(dir, nil)

	e.Run(extractRequest(client, buildMessages(2)), false)
	if e.cursor.index != 0 {
		t.Fatalf("failure should not advance cursor, got index %d", e.cursor.index)
	}

	snap2 := buildMessages(4)
	e.Run(extractRequest(client, snap2), false)

	second := joinedMessages(client.inputAt(1))
	if !strings.Contains(second, "msg-0.") {
		t.Errorf("retry should re-cover the previously failed window:\n%s", second)
	}
	if !strings.Contains(second, "msg-2.") {
		t.Errorf("retry should also cover new messages:\n%s", second)
	}
	if e.cursor.index != len(snap2) {
		t.Fatalf("expected cursor advanced to %d after success, got %d", len(snap2), e.cursor.index)
	}
}

// TestExtractorExtractsAndPersists 覆盖完整链路：抽取成功后记忆落盘、游标推进。
func TestExtractorExtractsAndPersists(t *testing.T) {
	dir := t.TempDir()
	saveCall := &clients.ChatResponse{
		Model: "test",
		Message: clients.Message{
			Role: "assistant",
			ToolCalls: []tools.ToolCall{
				{
					ID:   "call_1",
					Name: "save_memory",
					Arguments: map[string]interface{}{
						"name":        "prefer_tabs",
						"description": "User prefers tabs for indentation",
						"type":        "user",
						"content":     "The user explicitly prefers tabs over spaces.",
					},
				},
			},
		},
	}
	final := &clients.ChatResponse{
		Model:   "test",
		Message: clients.Message{Role: "assistant", Content: "Saved 1 memory about indentation preference."},
	}
	client := newScriptedChatClient(chatStep{resp: saveCall}, chatStep{resp: final})
	e := NewExtractor(dir, nil)

	snap := []clients.Message{
		{Role: "user", Content: "I always prefer tabs over spaces when writing Go code, please remember that."},
	}
	e.Run(extractRequest(client, snap), false)

	if _, err := os.Stat(filepath.Join(dir, "prefer_tabs.md")); err != nil {
		t.Errorf("expected prefer_tabs.md to be created: %v", err)
	}
	if e.cursor.index != len(snap) {
		t.Fatalf("expected cursor advanced to %d, got %d", len(snap), e.cursor.index)
	}
}
