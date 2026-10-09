package memory

// 本文件实现每轮会话结束后的同步增量记忆抽取：
// REPL 侧在回合结束时内联调用 Extractor.Run（阻塞），抽取跑完后才结束该轮。
// 增量游标避免重复抽取；抽取核心与 skip 判定在 memory_extraction.go。

import (
	"hash/fnv"

	"zoomClient/clients"
	"zoomClient/emitter"
	"zoomClient/logger"
	"zoomClient/tools"

	"go.uber.org/zap"
)

// memoryCursor 记录抽取覆盖进度，用于计算增量窗口、防止重复抽取。
// 指纹（而非裸索引）用于检测消息序列被整体压缩 / 替换 / 清空后游标失效。
type memoryCursor struct {
	index int    // messages[:index] 已处理
	fp    uint64 // messages[index-1] 的指纹
}

// window 返回尚未处理的增量窗口。
// 游标失效时返回全量消息作兜底；实际抽取仍受 memoryExtractionTail 截断
// 与抽取 prompt 内 manifest 去重约束，兜底只会带来一次冗余抽取，不给下游引入重复记忆。
func (c *memoryCursor) window(messages []clients.Message) []clients.Message {
	if c.index < 0 || c.index > len(messages) {
		return messages
	}
	if c.index == 0 || fingerprint(messages[c.index-1]) != c.fp {
		return messages
	}
	return messages[c.index:]
}

// advance 把游标推进到消息序列末尾，标记 messages 已全部处理。
// skip（无散文 / 已手写 / 用户要求忽略）与抽取成功都调用；
// 抽取失败 / 超时不调用，使该窗口在下次触发时自动重试。
func (c *memoryCursor) advance(messages []clients.Message) {
	c.index = len(messages)
	if len(messages) == 0 {
		c.fp = 0
		return
	}
	c.fp = fingerprint(messages[len(messages)-1])
}

// fingerprint 计算单条消息的稳态指纹（role + 内容 + toolCallID）。
// MicroCompact 替换旧 tool 结果内容会使指纹误变，触发一次冗余兜底抽取；
// 方向上是安全的（宁多抽不漏抽），且被 manifest 去重吸收。
func fingerprint(m clients.Message) uint64 {
	h := fnv.New64a()
	h.Write([]byte(m.Role))
	h.Write([]byte{0})
	h.Write([]byte(messageContentToString(m.Content)))
	h.Write([]byte{0})
	h.Write([]byte(m.ToolCallID))
	return h.Sum64()
}

// ExtractionRequest 是一次抽取的输入：消息 + 当轮客户端/模型 + 工具上下文。
// 由 REPL 主 goroutine 构造并同步消费，抽取过程不修改 Messages。
type ExtractionRequest struct {
	Messages []clients.Message
	Client   clients.ChatClient
	Model    string
	ToolCtx  tools.ToolContext
}

// Extractor 在调用方 goroutine 内同步执行 per-turn 增量抽取。
// 无内部并发：REPL 串行调用 Run，游标仅在该 goroutine 访问，故无需加锁；
// 记忆目录写入串行化由调用时序 + 工具侧包级锁共同保证。
type Extractor struct {
	cursor    memoryCursor
	memoryDir string
	em        emitter.Emitter // info 提示；announce 时另发 memory 相位事件
}

// NewExtractor 创建同步抽取器。
func NewExtractor(memoryDir string, em emitter.Emitter) *Extractor {
	return &Extractor{memoryDir: memoryDir, em: em}
}

// Run 阻塞执行一次增量抽取：算窗口 → skip 判定 → 抽取 → 推进游标。
// announce=true（回合路径）时在真正开始抽取前推送 memory 相位事件，
// 供前端显示"记忆更新中…"；skip 时不推送，避免琐碎轮次状态闪现。
// 成功或 skip 推进游标；失败 / 超时不推进，使该窗口在下次触发时自动重试。
func (e *Extractor) Run(req ExtractionRequest, announce bool) {
	window := e.cursor.window(req.Messages)
	if len(window) == 0 {
		return
	}

	// 判断是否值得抽取记忆
	run, reason := shouldRunMemoryExtraction(window)
	if !run {
		if logger.Log != nil {
			logger.Log.Info("turn memory extraction skipped",
				zap.String("reason", reason), zap.Int("window", len(window)))
		}
		e.cursor.advance(req.Messages)
		return
	}

	// 抽取即将开始：回合路径通知前端进入"记忆更新中…"相位
	if announce && e.em != nil {
		e.em.EmitEmotion("memory", nil)
	}

	// 对窗口执行一次记忆抽取（阻塞，内部有 ExtractionTimeout 硬超时兜底）
	err := extractMemoryFromWindow(req.Client, req.Model, &req.ToolCtx, e.memoryDir, window, e.em)
	if err != nil {
		// 失败不推进游标，下轮触发时同一窗口自动重试
		if logger.Log != nil {
			logger.Log.Warn("turn memory extraction failed",
				zap.Error(err), zap.Int("window", len(window)))
		}
		return
	}
	e.cursor.advance(req.Messages)
	if logger.Log != nil {
		logger.Log.Info("turn memory extraction done", zap.Int("window", len(window)))
	}
}
