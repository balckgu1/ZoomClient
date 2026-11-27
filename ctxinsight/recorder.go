// ctxinsight/recorder.go
//
// Recorder 是上下文洞察折叠器：由 agentLoop / REPL 在消息变更的确切位置
// 直接调用（不走 hook——hook payload 缺少组装后的完整载荷），
// 增量维护消息表面、请求头纪元、逐请求记录与事件日志。
//
// 并发约定：写入仅发生在 REPL/agentLoop goroutine；HTTP 读取走
// Head/Detail 的读锁快照拷贝，与 web.Session 的 usage 缓存模式一致。
package ctxinsight

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"zoomClient/clients"
	"zoomClient/tools"
)

// 保留边界（参照 dsh-context 插件 config 默认值，常量即可不做配置）
const (
	maxRequests   = 200   // 逐请求记录硬上限
	maxKeptTurns  = 50    // 整轮保留窗口（裁剪 never 切半轮）
	maxEvents     = 200   // 事件日志上限
	maxSurface    = 400   // 存活表面节点上限（病态会话兜底）
	maxArchive    = 400   // 归档节点上限
	maxHeaders    = 10    // 请求头纪元上限
	maxNodeText   = 16384 // 单节点展示内容截断上限（字节）
	maxSchemaText = 8192  // 单工具 schema 展示截断上限（字节）
)

// Recorder 上下文洞察折叠器。
type Recorder struct {
	mu sync.RWMutex
	st State
}

// New 创建折叠器。limitBytes 为上下文阈值（与 compact.ContextLimit 一致），
// model 为初始模型名。
func New(limitBytes int, model string) *Recorder {
	return &Recorder{st: State{
		LimitBytes: limitBytes,
		Model:      model,
		Surface:    []SurfaceNode{},
		Archive:    []SurfaceNode{},
		Headers:    []HeaderEpoch{},
		Requests:   []RequestRecord{},
		Events:     []ContextEvent{},
	}}
}

// StepInput RecordStep 的输入载荷。
type StepInput struct {
	SystemPrompt    string                // 组装后的完整 system prompt（含 skills 段）
	SkillsSection   string                // skills 目录段（从 system prompt 中拆出单列）
	ToolList        []tools.Tool          // 当前注册的工具列表
	PayloadMessages []clients.Message     // 流水线组装后的消息（含注入的 reminder）
	Response        *clients.ChatResponse // 本次 LLM 响应（携带 provider 真实用量）
	ActiveMs        int64                 // 本次调用耗时（毫秒）
}

// ReplacedTool 微压缩中被占位符替换的工具结果。
type ReplacedTool struct {
	CallID   string // 工具调用 ID（定位存活节点）
	OldBytes int    // 替换前字节数
	NewBytes int    // 替换后字节数
	Text     string // 替换后的内容（占位符）
}

// ─── 内部工具 ───

// nowMs 当前 Unix 毫秒时间戳
func nowMs() int64 { return time.Now().UnixMilli() }

// nextSeq 分配全局单调序号（须持锁调用）
func (r *Recorder) nextSeq() int64 {
	r.st.Seq++
	return r.st.Seq
}

// capText 按字节上限截断文本，回退到合法 UTF-8 边界，返回截断标记。
func capText(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	cut := max
	for cut > 0 && !isRuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

// isRuneStart 判断字节是否为 UTF-8 字符首字节（续字节高两位为 10）
func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

// messageBytes 估算单条消息的上下文字节占用，口径与 compact.EstimateSize 一致。
func messageBytes(m clients.Message) int {
	total := len(m.Role)
	switch v := m.Content.(type) {
	case string:
		total += len(v)
	default:
		b, _ := json.Marshal(v)
		total += len(b)
	}
	total += len(m.ReasoningContent)
	total += len(m.ToolCallID)
	for _, tc := range m.ToolCalls {
		total += len(tc.Name)
		b, _ := json.Marshal(tc.Arguments)
		total += len(b)
	}
	return total
}

// messageFingerprint 计算消息内容指纹（role + 内容 + reasoning + 工具调用），
// 用于跨步骤对齐与注入去重。
func messageFingerprint(m clients.Message) string {
	h := sha256.New()
	h.Write([]byte(m.Role))
	h.Write([]byte{0})
	switch v := m.Content.(type) {
	case string:
		h.Write([]byte(v))
	default:
		b, _ := json.Marshal(v)
		h.Write(b)
	}
	h.Write([]byte{0})
	h.Write([]byte(m.ReasoningContent))
	h.Write([]byte(m.ToolCallID))
	for _, tc := range m.ToolCalls {
		h.Write([]byte(tc.Name))
		b, _ := json.Marshal(tc.Arguments)
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// messageText 构造节点的展示文本：正文 + reasoning + 工具调用摘要。
func messageText(m clients.Message) string {
	var sb strings.Builder
	switch v := m.Content.(type) {
	case string:
		sb.WriteString(v)
	case nil:
	default:
		b, _ := json.Marshal(v)
		sb.Write(b)
	}
	if m.ReasoningContent != "" {
		sb.WriteString("\n[reasoning] ")
		sb.WriteString(m.ReasoningContent)
	}
	for _, tc := range m.ToolCalls {
		sb.WriteString("\n[tool_call] ")
		sb.WriteString(tc.Name)
		args, _ := json.Marshal(tc.Arguments)
		sb.WriteString(" ")
		sb.Write(args)
	}
	return sb.String()
}

// appendNode 追加存活节点并累计消息侧占用（须持锁调用）。
func (r *Recorder) appendNode(n SurfaceNode) {
	if n.Seq == 0 {
		n.Seq = r.nextSeq()
	}
	if n.Time == 0 {
		n.Time = nowMs()
	}
	r.st.Surface = append(r.st.Surface, n)
	r.addSum(n.Cat, n.Bytes)
	r.st.DetailRev++
	r.trimSurface()
}

// addSum 累加某分类的消息侧占用（须持锁调用）
func (r *Recorder) addSum(cat Category, bytes int) {
	switch cat {
	case CatUser:
		r.st.Sums.User += bytes
	case CatInject:
		r.st.Sums.Inject += bytes
	case CatAssistant:
		r.st.Sums.Assistant += bytes
	case CatTool:
		r.st.Sums.Tool += bytes
	}
	r.st.Sums.Total = r.st.Sums.User + r.st.Sums.Inject + r.st.Sums.Assistant + r.st.Sums.Tool
}

// archiveNode 将存活节点移入归档并扣减占用（须持锁调用）。
// goneSeq 为移除事件的 seq，供历史请求重建判定。
func (r *Recorder) archiveNode(idx int, goneSeq int64) {
	n := r.st.Surface[idx]
	n.Gone = goneSeq
	r.addSum(n.Cat, -n.Bytes)
	r.st.Surface = append(r.st.Surface[:idx], r.st.Surface[idx+1:]...)
	r.st.Archive = append(r.st.Archive, n)
	r.st.DetailRev++
	r.trimArchive()
}

// addEvent 追加一条上下文事件（须持锁调用）
func (r *Recorder) addEvent(kind string, bytes int, name, detail string) {
	r.st.Events = append(r.st.Events, ContextEvent{
		Seq:    r.nextSeq(),
		Time:   nowMs(),
		Kind:   kind,
		Bytes:  bytes,
		Name:   name,
		Detail: detail,
		Turn:   r.st.CurTurn,
		Step:   r.st.CurStep,
	})
	r.st.DetailRev++
	if len(r.st.Events) > maxEvents {
		r.st.Events = r.st.Events[len(r.st.Events)-maxEvents:]
	}
}

// trimSurface 存活表面超限时丢弃最旧节点。占用累计（Sums）不受影响——
// 被丢弃节点仍在模型上下文中，只是浏览器不再展示（SurfaceFloor 标记覆盖下限）。
func (r *Recorder) trimSurface() {
	for len(r.st.Surface) > maxSurface {
		dropped := r.st.Surface[0]
		r.st.Surface = r.st.Surface[1:]
		r.st.DroppedNodes++
		r.st.SurfaceFloor = dropped.Seq
	}
}

// trimArchive 归档超限时丢弃最旧条目
func (r *Recorder) trimArchive() {
	for len(r.st.Archive) > maxArchive {
		dropped := r.st.Archive[0]
		r.st.Archive = r.st.Archive[1:]
		r.st.ArchiveFloor = dropped.Gone
	}
}

// trimRequests 按整轮裁剪请求记录：超过步数硬上限或整轮窗口时，
// 从头部丢弃完整的一轮（never 切半轮），保证轮粒度视图始终完整。
// 仅剩一轮时停止裁剪——最新一轮必须完整保留，哪怕它本身超步数上限。
func (r *Recorder) trimRequests() {
	for len(r.st.Requests) > 0 {
		runs := r.turnRuns()
		if runs <= 1 || (len(r.st.Requests) <= maxRequests && runs <= maxKeptTurns) {
			return
		}
		firstTurn := r.st.Requests[0].Turn
		j := 1
		for j < len(r.st.Requests) && r.st.Requests[j].Turn == firstTurn {
			j++
		}
		r.st.Requests = r.st.Requests[j:]
	}
}

// turnRuns 统计请求记录中的轮次数（连续同 Turn 为一段）
func (r *Recorder) turnRuns() int {
	runs := 0
	prev := -1
	for _, req := range r.st.Requests {
		if req.Turn != prev {
			runs++
			prev = req.Turn
		}
	}
	return runs
}

// ─── 记录入口 ───

// RecordUserMessage 记录一条用户输入：开启新轮、累计人类输入计数。
func (r *Recorder) RecordUserMessage(content string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.st.CurTurn++
	r.st.CurStep = 0
	r.st.Counts.Turns++
	r.st.Counts.HumanInputs++
	m := clients.Message{Role: "user", Content: content}
	text, truncated := capText(content, maxNodeText)
	r.appendNode(SurfaceNode{
		Cat:       CatUser,
		Bytes:     messageBytes(m),
		Text:      text,
		Truncated: truncated,
		Fp:        messageFingerprint(m),
	})
}

// RecordStep 记录一次 LLM 请求：更新请求头纪元、对账注入节点、
// 追加请求记录，随后把 assistant 回复记入表面（属于下一请求的历史）。
func (r *Recorder) RecordStep(in StepInput) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.foldHeaders(in)
	r.reconcileInjects(in.PayloadMessages)

	// 请求记录：发出时刻的组成快照（不含本次回复）
	r.st.CurStep++
	r.st.Counts.Steps++
	req := RequestRecord{
		Seq:       r.nextSeq(),
		Time:      nowMs(),
		Turn:      r.st.CurTurn,
		Step:      r.st.CurStep,
		HeaderSeq: r.lastHeaderSeq(),
		System:    r.lastHeader().SystemBytes,
		Skills:    r.lastHeader().SkillsBytes,
		Tools:     r.lastHeader().ToolsBytes,
		User:      r.st.Sums.User,
		Inject:    r.st.Sums.Inject,
		Assistant: r.st.Sums.Assistant,
		Tool:      r.st.Sums.Tool,
		ActiveMs:  in.ActiveMs,
	}
	req.Total = req.System + req.Skills + req.Tools + req.User + req.Inject + req.Assistant + req.Tool
	if in.Response != nil {
		req.PromptTokens = in.Response.Usage.PromptTokens
		req.OutputTokens = in.Response.Usage.CompletionTokens
		req.CacheHitTokens = in.Response.Usage.PromptCacheHitTokens
	}
	r.st.Requests = append(r.st.Requests, req)
	r.st.DetailRev++
	r.trimRequests()

	// assistant 回复进入表面，成为后续请求的历史
	if in.Response != nil {
		m := in.Response.Message
		text, truncated := capText(messageText(m), maxNodeText)
		names := make([]string, 0, len(m.ToolCalls))
		for _, tc := range m.ToolCalls {
			names = append(names, tc.Name)
		}
		r.appendNode(SurfaceNode{
			Cat:       CatAssistant,
			Bytes:     messageBytes(m),
			Text:      text,
			Truncated: truncated,
			Tool:      strings.Join(names, ","),
			Fp:        messageFingerprint(m),
		})
	}
}

// foldHeaders 请求头内容变化时追加新纪元（须持锁调用）。
// 比对用内容指纹而非截断后的展示文本，避免超长 prompt 每步误判为变化。
func (r *Recorder) foldHeaders(in StepInput) {
	skillsBytes := len(in.SkillsSection)
	systemBytes := len(in.SystemPrompt) - skillsBytes
	if systemBytes < 0 {
		systemBytes = 0
	}
	toolsBytes := clients.ToolsSchemaBytes(in.ToolList)

	names := make([]string, 0, len(in.ToolList))
	for _, t := range in.ToolList {
		names = append(names, t.Name())
	}
	fp := fingerprintOf(in.SystemPrompt + "\x00" + in.SkillsSection + "\x00" + strings.Join(names, ","))
	if len(r.st.Headers) > 0 && r.st.Headers[len(r.st.Headers)-1].Fp == fp {
		return
	}

	sysText, _ := capText(in.SystemPrompt, maxNodeText)
	skillsText, _ := capText(in.SkillsSection, maxNodeText)
	infos := make([]ToolInfo, 0, len(in.ToolList))
	for _, t := range in.ToolList {
		schema, _ := json.Marshal(map[string]any{
			"name":        t.Name(),
			"description": t.Description(),
			"parameters":  t.Parameters(),
		})
		schemaText, _ := capText(string(schema), maxSchemaText)
		infos = append(infos, ToolInfo{Name: t.Name(), Bytes: len(schema), Schema: schemaText})
	}
	r.st.Headers = append(r.st.Headers, HeaderEpoch{
		Seq:         r.nextSeq(),
		Time:        nowMs(),
		Fp:          fp,
		SystemBytes: systemBytes,
		SkillsBytes: skillsBytes,
		ToolsBytes:  toolsBytes,
		SystemText:  sysText,
		SkillsText:  skillsText,
		Tools:       infos,
	})
	if len(r.st.Headers) > maxHeaders {
		r.st.Headers = r.st.Headers[len(r.st.Headers)-maxHeaders:]
	}
	r.st.DetailRev++
}

// fingerprintOf 计算任意字符串的 sha256 短指纹
func fingerprintOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}

// lastHeader 返回最新请求头纪元；无纪元时返回零值（须持锁调用）
func (r *Recorder) lastHeader() HeaderEpoch {
	if len(r.st.Headers) == 0 {
		return HeaderEpoch{}
	}
	return r.st.Headers[len(r.st.Headers)-1]
}

// lastHeaderSeq 返回最新纪元 seq（须持锁调用）
func (r *Recorder) lastHeaderSeq() int64 { return r.lastHeader().Seq }

// reconcileInjects 对账注入节点：payload 中 system-role 消息即流水线注入的
// reminder。内容指纹不在存活注入节点中的追加为新节点；存活注入节点不再出现
// 于本次 payload 的（OneShot 提醒）移入归档。持久 reminder 因指纹去重不会重复计入。
func (r *Recorder) reconcileInjects(payload []clients.Message) {
	live := make(map[string]bool)
	for _, n := range r.st.Surface {
		if n.Cat == CatInject {
			live[n.Fp] = true
		}
	}
	seen := make(map[string]bool)
	var incoming []clients.Message
	for _, m := range payload {
		if m.Role != "system" {
			continue
		}
		fp := messageFingerprint(m)
		if seen[fp] {
			continue
		}
		seen[fp] = true
		incoming = append(incoming, m)
	}
	// 归档不再出现的注入节点（倒序遍历避免下标漂移）
	for i := len(r.st.Surface) - 1; i >= 0; i-- {
		n := r.st.Surface[i]
		if n.Cat == CatInject && !seen[n.Fp] {
			r.archiveNode(i, r.nextSeq())
		}
	}
	// 追加新注入节点
	for _, m := range incoming {
		fp := messageFingerprint(m)
		if live[fp] {
			continue
		}
		text, truncated := capText(messageText(m), maxNodeText)
		r.appendNode(SurfaceNode{
			Cat:       CatInject,
			Bytes:     messageBytes(m),
			Text:      text,
			Truncated: truncated,
			Name:      "reminder",
			Fp:        fp,
		})
		r.st.Counts.Injects++
		r.addEvent("inject", messageBytes(m), "reminder", firstLine(messageText(m)))
	}
}

// firstLine 取文本首行并限长，作为事件摘要
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 80 {
		s, _ = capText(s, 80)
	}
	return s
}

// RecordToolResults 记录写回消息历史的工具结果。
// contents 必须是实际追加的内容（大输出落盘后的预览），errs 为对应错误标记。
func (r *Recorder) RecordToolResults(calls []tools.ToolCall, contents []string, errs []bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, call := range calls {
		content := ""
		if i < len(contents) {
			content = contents[i]
		}
		isErr := i < len(errs) && errs[i]
		m := clients.Message{Role: "tool", Content: content, ToolCallID: call.ID}
		text, truncated := capText(content, maxNodeText)
		r.appendNode(SurfaceNode{
			Cat:       CatTool,
			Bytes:     messageBytes(m),
			Text:      text,
			Truncated: truncated,
			Tool:      call.Name,
			CallID:    call.ID,
			Err:       isErr,
			Fp:        messageFingerprint(m),
		})
	}
}

// RecordMicroCompact 记录微压缩：被占位符替换的工具结果旧节点归档、
// 占位节点入表面，并记一条 prune 事件。
func (r *Recorder) RecordMicroCompact(replaced []ReplacedTool) {
	if r == nil {
		return
	}
	if len(replaced) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	freed := 0
	for _, rp := range replaced {
		idx := -1
		for i, n := range r.st.Surface {
			if n.Cat == CatTool && n.CallID == rp.CallID {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue
		}
		r.archiveNode(idx, r.nextSeq())
		m := clients.Message{Role: "tool", Content: rp.Text, ToolCallID: rp.CallID}
		text, truncated := capText(rp.Text, maxNodeText)
		r.appendNode(SurfaceNode{
			Cat:       CatTool,
			Bytes:     messageBytes(m),
			Text:      text,
			Truncated: truncated,
			CallID:    rp.CallID,
			Name:      "micro-compact",
			Fp:        messageFingerprint(m),
		})
		freed += rp.OldBytes - rp.NewBytes
	}
	if freed > 0 {
		r.st.Counts.Prunes++
		r.addEvent("prune", freed, "micro-compact", "较早的工具结果被占位符替换")
	}
}

// RecordPersistPrune 记录大输出落盘裁剪（节点本身记录的已是预览内容，
// 此处仅记事件，不改动表面）。
func (r *Recorder) RecordPersistPrune(toolName string, freedBytes int) {
	if r == nil {
		return
	}
	if freedBytes <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.st.Counts.Prunes++
	r.addEvent("prune", freedBytes, toolName, "大输出已落盘，上下文仅保留预览")
}

// RecordCompaction 记录完整压缩：after 中不再存活的消息节点归档，
// 摘要消息作为新节点入表面，记 compaction 事件（净释放字节）。
// 注入节点不在此处理——它们由流水线在下一请求重新注入，对账逻辑自动对齐。
func (r *Recorder) RecordCompaction(after []clients.Message, freedBytes int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	afterFps := make(map[string]bool, len(after))
	for _, m := range after {
		afterFps[messageFingerprint(m)] = true
	}
	liveFps := make(map[string]bool, len(r.st.Surface))
	for _, n := range r.st.Surface {
		liveFps[n.Fp] = true
	}
	// 归档不再存活的消息节点（注入节点除外，倒序遍历）
	for i := len(r.st.Surface) - 1; i >= 0; i-- {
		n := r.st.Surface[i]
		if n.Cat == CatInject {
			continue
		}
		if !afterFps[n.Fp] {
			r.archiveNode(i, r.nextSeq())
		}
	}
	// after 中的新消息（压缩摘要）入表面
	for _, m := range after {
		fp := messageFingerprint(m)
		if liveFps[fp] {
			continue
		}
		cat := CatUser
		name := "compaction"
		if m.Role == "assistant" {
			cat, name = CatAssistant, ""
		} else if m.Role == "tool" {
			cat, name = CatTool, ""
		}
		text, truncated := capText(messageText(m), maxNodeText)
		r.appendNode(SurfaceNode{
			Cat:       cat,
			Bytes:     messageBytes(m),
			Text:      text,
			Truncated: truncated,
			Name:      name,
			CallID:    m.ToolCallID,
			Fp:        fp,
		})
	}
	r.st.Counts.Compactions++
	r.addEvent("compaction", freedBytes, "compact", "早期对话被摘要压缩")
}

// RecordModelSwitch 记录模型切换事件并更新当前模型名。
func (r *Recorder) RecordModelSwitch(from, to string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.st.Model = to
	r.addEvent("model", 0, to, "模型切换："+from+" → "+to)
}

// SetModel 静默更新模型名（会话恢复等场景，不记事件）。
func (r *Recorder) SetModel(model string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.st.Model = model
}

// SetLimit 更新上下文阈值（会话恢复时以当前配置为准）。
func (r *Recorder) SetLimit(bytes int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.st.LimitBytes = bytes
}

// RecordClear 记录会话清空：全部存活节点归档，记 clear 事件。
// 请求历史与事件日志保留（它们是已发生事实的记录）。
func (r *Recorder) RecordClear() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.st.Surface) - 1; i >= 0; i-- {
		r.archiveNode(i, r.nextSeq())
	}
	r.addEvent("clear", 0, "clear", "会话历史已清空")
}

// Reset 重置为空状态（新建会话），保留阈值与模型名。
func (r *Recorder) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	limit, model := r.st.LimitBytes, r.st.Model
	r.st = State{
		LimitBytes: limit,
		Model:      model,
		Surface:    []SurfaceNode{},
		Archive:    []SurfaceNode{},
		Headers:    []HeaderEpoch{},
		Requests:   []RequestRecord{},
		Events:     []ContextEvent{},
		DetailRev:  r.st.DetailRev + 1,
	}
}

// ─── 读取与持久化 ───

// Head 返回轻量头部快照（读锁下拷贝）。
func (r *Recorder) Head() HeadSnapshot {
	if r == nil {
		return HeadSnapshot{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	h := r.lastHeader()
	cur := Composition{
		System:    h.SystemBytes,
		Skills:    h.SkillsBytes,
		Tools:     h.ToolsBytes,
		User:      r.st.Sums.User,
		Inject:    r.st.Sums.Inject,
		Assistant: r.st.Sums.Assistant,
		Tool:      r.st.Sums.Tool,
	}
	cur.Total = cur.System + cur.Skills + cur.Tools + cur.User + cur.Inject + cur.Assistant + cur.Tool
	head := HeadSnapshot{
		Model:      r.st.Model,
		LimitBytes: r.st.LimitBytes,
		Current:    cur,
		Counts:     r.st.Counts,
		DetailRev:  r.st.DetailRev,
	}
	if n := len(r.st.Requests); n > 0 {
		last := r.st.Requests[n-1]
		head.Last = &last
	}
	return head
}

// Detail 返回详情快照（读锁下深拷贝切片，调用方可自由持有）。
func (r *Recorder) Detail() DetailSnapshot {
	if r == nil {
		return emptyDetail()
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return DetailSnapshot{
		Rev:          r.st.DetailRev,
		Requests:     append([]RequestRecord{}, r.st.Requests...),
		Events:       append([]ContextEvent{}, r.st.Events...),
		Nodes:        append([]SurfaceNode{}, r.st.Surface...),
		Archive:      append([]SurfaceNode{}, r.st.Archive...),
		Headers:      append([]HeaderEpoch{}, r.st.Headers...),
		DroppedNodes: r.st.DroppedNodes,
		SurfaceFloor: r.st.SurfaceFloor,
		ArchiveFloor: r.st.ArchiveFloor,
	}
}

// emptyDetail 空详情快照：集合为 [] 而非 null，前端可直接遍历
func emptyDetail() DetailSnapshot {
	return DetailSnapshot{
		Requests: []RequestRecord{},
		Events:   []ContextEvent{},
		Nodes:    []SurfaceNode{},
		Archive:  []SurfaceNode{},
		Headers:  []HeaderEpoch{},
	}
}

// Export 导出完整状态用于持久化（读锁下拷贝）。
func (r *Recorder) Export() State {
	if r == nil {
		return State{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := r.st
	out.Surface = append([]SurfaceNode(nil), r.st.Surface...)
	out.Archive = append([]SurfaceNode(nil), r.st.Archive...)
	out.Headers = append([]HeaderEpoch(nil), r.st.Headers...)
	out.Requests = append([]RequestRecord(nil), r.st.Requests...)
	out.Events = append([]ContextEvent(nil), r.st.Events...)
	return out
}

// Restore 从持久化状态恢复（会话加载）。nil 切片规范化为空切片。
func (r *Recorder) Restore(st State) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if st.Surface == nil {
		st.Surface = []SurfaceNode{}
	}
	if st.Archive == nil {
		st.Archive = []SurfaceNode{}
	}
	if st.Headers == nil {
		st.Headers = []HeaderEpoch{}
	}
	if st.Requests == nil {
		st.Requests = []RequestRecord{}
	}
	if st.Events == nil {
		st.Events = []ContextEvent{}
	}
	r.st = st
}
