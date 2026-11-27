// ctxinsight/recorder_test.go
//
// Recorder 折叠器单元测试：分类折叠、注入对账、纪元去重、微压缩/完整压缩、
// 整轮裁剪、文本截断、持久化往返与并发读写安全。
package ctxinsight

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"zoomClient/clients"
	"zoomClient/tools"
)

// fakeTool 测试用工具桩，实现 tools.Tool 接口
type fakeTool struct{ name string }

func (f fakeTool) Name() string               { return f.name }
func (f fakeTool) Description() string        { return "test tool " + f.name }
func (f fakeTool) Parameters() map[string]any { return map[string]any{"type": "object"} }
func (f fakeTool) Call(map[string]any, *tools.ToolContext) tools.ToolResult {
	return tools.ToolResult{Ok: true}
}

// testResponse 构造一次带真实用量的 LLM 响应
func testResponse(content string) *clients.ChatResponse {
	return &clients.ChatResponse{
		Message: clients.Message{Role: "assistant", Content: content},
		Usage:   clients.TokenUsage{PromptTokens: 100, CompletionTokens: 10, PromptCacheHitTokens: 40},
	}
}

// stepInput 构造一次请求输入：system prompt + 用户消息 + 可选 reminder
func stepInput(reminder string) StepInput {
	payload := []clients.Message{{Role: "user", Content: "hi"}}
	if reminder != "" {
		payload = append(payload, clients.Message{Role: "system", Content: reminder})
	}
	return StepInput{
		SystemPrompt:    "you are a helpful agent",
		SkillsSection:   "skills catalog",
		ToolList:        []tools.Tool{fakeTool{"read_file"}},
		PayloadMessages: payload,
		Response:        testResponse("ok"),
		ActiveMs:        120,
	}
}

// TestRecordStepComposition 验证请求记录的分类折叠与 provider 真实用量透传
func TestRecordStepComposition(t *testing.T) {
	r := New(100000, "test-model")
	r.RecordUserMessage("hi")
	r.RecordStep(stepInput("remember this"))

	head := r.Head()
	if head.Counts.Turns != 1 || head.Counts.Steps != 1 || head.Counts.HumanInputs != 1 {
		t.Fatalf("counts = %+v, want turns/steps/humanInputs = 1/1/1", head.Counts)
	}
	if head.Current.User <= 0 || head.Current.Assistant <= 0 || head.Current.Inject <= 0 {
		t.Fatalf("composition missing categories: %+v", head.Current)
	}
	if head.Current.System != len("you are a helpful agent")-len("skills catalog") {
		t.Fatalf("system bytes = %d, want skills section split out", head.Current.System)
	}
	if head.Current.Skills != len("skills catalog") {
		t.Fatalf("skills bytes = %d", head.Current.Skills)
	}
	wantTotal := head.Current.System + head.Current.Skills + head.Current.Tools +
		head.Current.User + head.Current.Inject + head.Current.Assistant + head.Current.Tool
	if head.Current.Total != wantTotal {
		t.Fatalf("total = %d, want %d", head.Current.Total, wantTotal)
	}
	if head.Last == nil || head.Last.PromptTokens != 100 || head.Last.OutputTokens != 10 || head.Last.CacheHitTokens != 40 {
		t.Fatalf("last request usage not carried: %+v", head.Last)
	}
	if head.Last.ActiveMs != 120 {
		t.Fatalf("activeMs = %d, want 120", head.Last.ActiveMs)
	}
	// 请求记录不含本次回复：assistant 字节应为 0（首步无历史 assistant）
	if head.Last.Assistant != 0 {
		t.Fatalf("first request should carry no assistant history, got %d", head.Last.Assistant)
	}
}

// TestInjectReconcile 验证注入对账：持久 reminder 去重、OneShot 下一步自动归档
func TestInjectReconcile(t *testing.T) {
	r := New(100000, "m")
	r.RecordUserMessage("hi")
	r.RecordStep(stepInput("persistent reminder"))
	r.RecordStep(stepInput("persistent reminder")) // 相同内容不应重复计入
	head := r.Head()
	if head.Counts.Injects != 1 {
		t.Fatalf("injects = %d, want 1 (dedup)", head.Counts.Injects)
	}
	// OneShot：下一步 payload 不再携带 → 归档
	r.RecordStep(stepInput(""))
	detail := r.Detail()
	liveInject := 0
	for _, n := range detail.Nodes {
		if n.Cat == CatInject {
			liveInject++
		}
	}
	if liveInject != 0 {
		t.Fatalf("live inject = %d, want 0 after oneshot consumed", liveInject)
	}
	archived := 0
	for _, n := range detail.Archive {
		if n.Cat == CatInject && n.Gone > 0 {
			archived++
		}
	}
	if archived != 1 {
		t.Fatalf("archived inject = %d, want 1", archived)
	}
	if r.Head().Current.Inject != 0 {
		t.Fatalf("inject sums should drop to 0, got %d", r.Head().Current.Inject)
	}
}

// TestHeaderEpochs 验证请求头纪元：内容不变不追加，变化追加且请求正确引用
func TestHeaderEpochs(t *testing.T) {
	r := New(100000, "m")
	r.RecordUserMessage("hi")
	r.RecordStep(stepInput(""))
	r.RecordStep(stepInput(""))
	if n := len(r.Detail().Headers); n != 1 {
		t.Fatalf("headers = %d, want 1 (unchanged content)", n)
	}
	in := stepInput("")
	in.SkillsSection = "skills catalog + new skill"
	in.SystemPrompt = "you are a helpful agent" + in.SkillsSection
	r.RecordStep(in)
	detail := r.Detail()
	if len(detail.Headers) != 2 {
		t.Fatalf("headers = %d, want 2 after skills change", len(detail.Headers))
	}
	last := detail.Requests[len(detail.Requests)-1]
	if last.HeaderSeq != detail.Headers[1].Seq {
		t.Fatalf("request headerSeq = %d, want latest epoch %d", last.HeaderSeq, detail.Headers[1].Seq)
	}
	if detail.Headers[1].SkillsBytes != len(in.SkillsSection) {
		t.Fatalf("epoch skills bytes = %d", detail.Headers[1].SkillsBytes)
	}
}

// TestToolResultsAndMicroCompact 验证工具节点记录与微压缩的归档/占位替换
func TestToolResultsAndMicroCompact(t *testing.T) {
	r := New(100000, "m")
	r.RecordUserMessage("hi")
	r.RecordStep(stepInput(""))
	calls := []tools.ToolCall{{ID: "c1", Name: "read_file"}, {ID: "c2", Name: "run_bash"}}
	r.RecordToolResults(calls, []string{strings.Repeat("x", 200), "bash output"}, []bool{false, true})

	head := r.Head()
	if head.Current.Tool <= 0 {
		t.Fatalf("tool sums = %d, want > 0", head.Current.Tool)
	}
	detail := r.Detail()
	var errNode *SurfaceNode
	for i := range detail.Nodes {
		if detail.Nodes[i].CallID == "c2" {
			errNode = &detail.Nodes[i]
		}
	}
	if errNode == nil || !errNode.Err || errNode.Tool != "run_bash" {
		t.Fatalf("error tool node not recorded correctly: %+v", errNode)
	}

	// 微压缩替换 c1
	oldBytes := messageBytes(clients.Message{Role: "tool", Content: strings.Repeat("x", 200), ToolCallID: "c1"})
	r.RecordMicroCompact([]ReplacedTool{{CallID: "c1", OldBytes: oldBytes, NewBytes: 40, Text: "[omitted]"}})
	detail = r.Detail()
	liveC1 := 0
	for _, n := range detail.Nodes {
		if n.CallID == "c1" {
			liveC1++
			if n.Text != "[omitted]" {
				t.Fatalf("placeholder text = %q", n.Text)
			}
		}
	}
	if liveC1 != 1 {
		t.Fatalf("live c1 nodes = %d, want 1 (placeholder only)", liveC1)
	}
	foundPrune := false
	for _, e := range detail.Events {
		if e.Kind == "prune" && e.Name == "micro-compact" && e.Bytes > 0 {
			foundPrune = true
		}
	}
	if !foundPrune {
		t.Fatal("prune event not recorded")
	}
}

// TestCompactionArchiveAndReconstruct 验证完整压缩的归档标记与历史请求重建判定
func TestCompactionArchiveAndReconstruct(t *testing.T) {
	r := New(100000, "m")
	r.RecordUserMessage("first question")
	r.RecordStep(stepInput(""))
	reqSeq := r.Head().Last.Seq
	r.RecordToolResults([]tools.ToolCall{{ID: "c1", Name: "read_file"}}, []string{"content"}, []bool{false})

	// 压缩：仅保留摘要 + 尾部工具配对
	after := []clients.Message{
		{Role: "user", Content: "This conversation was compacted for continuity.\n\nsummary"},
		{Role: "tool", Content: "content", ToolCallID: "c1"},
	}
	r.RecordCompaction(after, 500)

	detail := r.Detail()
	// 重建请求 reqSeq 时刻的存活节点：seq < reqSeq 且 (gone==0 或 gone > reqSeq)
	aliveAtReq := 0
	for _, n := range detail.Nodes {
		if n.Seq < reqSeq && (n.Gone == 0 || n.Gone > reqSeq) {
			aliveAtReq++
		}
	}
	for _, n := range detail.Archive {
		if n.Seq < reqSeq && (n.Gone == 0 || n.Gone > reqSeq) {
			aliveAtReq++
		}
	}
	// 请求时刻存活：仅 user("first question")。assistant 回复在该请求之后才入表面，
	// 属于下一请求的历史，不应计入本请求的组装。
	if aliveAtReq != 1 {
		t.Fatalf("alive at request = %d, want 1", aliveAtReq)
	}
	// 摘要节点入表面
	foundSummary := false
	for _, n := range detail.Nodes {
		if n.Name == "compaction" && strings.Contains(n.Text, "summary") {
			foundSummary = true
		}
	}
	if !foundSummary {
		t.Fatal("compaction summary node missing")
	}
	if r.Head().Counts.Compactions != 1 {
		t.Fatal("compaction count not incremented")
	}
	foundEvent := false
	for _, e := range detail.Events {
		if e.Kind == "compaction" && e.Bytes == 500 {
			foundEvent = true
		}
	}
	if !foundEvent {
		t.Fatal("compaction event with freed bytes missing")
	}
}

// TestTrimWholeTurns 验证整轮裁剪：不切半轮、最新一轮完整保留
func TestTrimWholeTurns(t *testing.T) {
	r := New(100000, "m")
	for i := 0; i < 60; i++ {
		r.RecordUserMessage("q")
		r.RecordStep(stepInput(""))
		r.RecordStep(stepInput("")) // 每轮 2 步
	}
	reqs := r.Detail().Requests
	if len(reqs) != maxKeptTurns*2 {
		t.Fatalf("requests = %d, want %d (whole turns)", len(reqs), maxKeptTurns*2)
	}
	if reqs[0].Step != 1 {
		t.Fatalf("first retained request step = %d, want 1 (no half turn)", reqs[0].Step)
	}
	if reqs[0].Turn != 11 {
		t.Fatalf("first retained turn = %d, want 11", reqs[0].Turn)
	}
	// 单轮超步数上限时不裁掉最新一轮
	r2 := New(100000, "m")
	r2.RecordUserMessage("q")
	for i := 0; i < maxRequests+50; i++ {
		r2.RecordStep(stepInput(""))
	}
	if n := len(r2.Detail().Requests); n != maxRequests+50 {
		t.Fatalf("single-turn requests = %d, want all kept (%d)", n, maxRequests+50)
	}
}

// TestTextTruncation 验证超长内容截断与 UTF-8 边界安全
func TestTextTruncation(t *testing.T) {
	r := New(100000, "m")
	long := strings.Repeat("中文内容", 8000) // 远超 16KB
	r.RecordUserMessage(long)
	n := r.Detail().Nodes[0]
	if !n.Truncated || len(n.Text) > maxNodeText {
		t.Fatalf("truncated = %v, len = %d, want truncated within cap", n.Truncated, len(n.Text))
	}
	if !json.Valid([]byte(`"` + n.Text + `"`)) {
		// 截断不应产生非法 UTF-8（JSON 字符串可序列化即间接验证）
		t.Fatal("truncated text is not valid UTF-8")
	}
	if n.Bytes <= maxNodeText {
		t.Fatalf("bytes = %d should reflect full content size", n.Bytes)
	}
}

// TestExportRestoreRoundtrip 验证持久化往返后状态一致
func TestExportRestoreRoundtrip(t *testing.T) {
	r := New(100000, "m")
	r.RecordUserMessage("hi")
	r.RecordStep(stepInput("reminder"))
	r.RecordToolResults([]tools.ToolCall{{ID: "c1", Name: "read_file"}}, []string{"x"}, []bool{false})
	r.RecordModelSwitch("m", "m2")

	data, err := json.Marshal(r.Export())
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	r2 := New(0, "")
	r2.Restore(st)
	h1, h2 := r.Head(), r2.Head()
	if h1.Current != h2.Current || h1.Counts != h2.Counts || h1.DetailRev != h2.DetailRev || h1.Model != h2.Model {
		t.Fatalf("head mismatch after restore:\n%+v\n%+v", h1, h2)
	}
	d1, d2 := r.Detail(), r2.Detail()
	if len(d1.Requests) != len(d2.Requests) || len(d1.Nodes) != len(d2.Nodes) || len(d1.Events) != len(d2.Events) {
		t.Fatal("detail collections mismatch after restore")
	}
}

// TestClearAndReset 验证清空归档与新建会话重置
func TestClearAndReset(t *testing.T) {
	r := New(100000, "m")
	r.RecordUserMessage("hi")
	r.RecordStep(stepInput(""))
	r.RecordClear()
	if r.Head().Current.User != 0 || r.Head().Current.Assistant != 0 {
		t.Fatal("clear should zero message-side sums")
	}
	if len(r.Detail().Nodes) != 0 {
		t.Fatal("clear should archive all live nodes")
	}
	foundClear := false
	for _, e := range r.Detail().Events {
		if e.Kind == "clear" {
			foundClear = true
		}
	}
	if !foundClear {
		t.Fatal("clear event missing")
	}
	r.Reset()
	d := r.Detail()
	if len(d.Requests) != 0 || len(d.Events) != 0 || len(d.Archive) != 0 || r.Head().LimitBytes != 100000 {
		t.Fatal("reset should empty history but keep limit")
	}
}

// TestNilResponseAndEmptyRecorder 验证空值边界
func TestNilResponseAndEmptyRecorder(t *testing.T) {
	r := New(0, "")
	h := r.Head()
	if h.Last != nil || h.Current.Total != 0 {
		t.Fatalf("empty head = %+v", h)
	}
	in := stepInput("")
	in.Response = nil
	r.RecordStep(in) // 不应 panic
	if r.Head().Counts.Steps != 1 {
		t.Fatal("step with nil response should still be recorded")
	}
	r.RecordToolResults(nil, nil, nil) // 空输入不应 panic
	r.RecordMicroCompact(nil)
	r.RecordPersistPrune("t", 0) // 零释放不记事件
	for _, e := range r.Detail().Events {
		if e.Kind == "prune" {
			t.Fatal("zero-byte persist prune should not emit event")
		}
	}
}

// TestDetailCopyIsolation 验证 Detail 快照与内部状态隔离（调用方修改不影响折叠器）
func TestDetailCopyIsolation(t *testing.T) {
	r := New(100000, "m")
	r.RecordUserMessage("hi")
	d := r.Detail()
	d.Nodes[0].Text = "tampered"
	d.Requests = append(d.Requests, RequestRecord{})
	if r.Detail().Nodes[0].Text == "tampered" {
		t.Fatal("detail snapshot shares backing array with recorder")
	}
}

// TestConcurrentReadWrite 并发读写冒烟测试（配合 -race 检测数据竞争）
func TestConcurrentReadWrite(t *testing.T) {
	r := New(100000, "m")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			r.RecordUserMessage("q")
			r.RecordStep(stepInput("reminder"))
			r.RecordToolResults([]tools.ToolCall{{ID: "c", Name: "t"}}, []string{"x"}, []bool{false})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = r.Head()
			_ = r.Detail()
			_ = r.Export()
		}
	}()
	wg.Wait()
	if r.Head().Counts.Steps != 100 {
		t.Fatalf("steps = %d, want 100", r.Head().Counts.Steps)
	}
}

// TestDetailEmptyCollectionsNotNull 回归：空详情集合必须序列化为 [] 而非 null，
// 前端 aliveNodesAt/趋势图直接遍历这些集合，null 会导致渲染期抛错。
func TestDetailEmptyCollectionsNotNull(t *testing.T) {
	for _, r := range []*Recorder{New(0, ""), nil} {
		data, err := json.Marshal(r.Detail())
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		for _, key := range []string{"requests", "events", "nodes", "archive", "headers"} {
			if string(m[key]) != "[]" {
				t.Fatalf("%s = %s, want []", key, m[key])
			}
		}
	}
}
