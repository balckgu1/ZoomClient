// Package ctxinsight 实现上下文洞察折叠器：按请求(step)记录上下文组成、
// 逐请求趋势、上下文事件与可浏览的消息表面，供 Web 前端 Context 面板展示。
// 数据模型参照 dsh-context 插件（plugin/dsh-context/lib/index.d.ts），
// 尺寸单位统一为字节，与 compact.UsageSnapshot / ContextLimit 保持一致。
package ctxinsight

// Category 上下文组成的分类。skills 单列以对齐现有占用指示器的口径。
type Category string

const (
	CatSystem    Category = "system"    // 系统提示词主体
	CatSkills    Category = "skills"    // skills 目录段
	CatTools     Category = "tools"     // 工具 schema
	CatUser      Category = "user"      // 用户消息
	CatInject    Category = "inject"    // 注入的提醒（reminder）
	CatAssistant Category = "assistant" // 助手回复（含 reasoning 与工具调用）
	CatTool      Category = "tool"      // 工具结果
)

// SurfaceNode 模型可见表面上的一条消息节点。
// Seq 为全局单调序号；Gone 非零表示已被移除（压缩/微压缩），
// 值为移除事件的 seq。节点在请求 R 中存活的判定：
// seq < R.Seq 且 (Gone == 0 或 Gone > R.Seq)。
type SurfaceNode struct {
	Seq       int64    `json:"seq"`
	Time      int64    `json:"time"` // Unix 毫秒
	Cat       Category `json:"cat"`
	Bytes     int      `json:"bytes"`
	Text      string   `json:"text,omitempty"`      // 展示内容（超上限截断）
	Truncated bool     `json:"truncated,omitempty"` // 内容是否被截断
	Tool      string   `json:"tool,omitempty"`      // 工具名（tool/assistant 节点）
	CallID    string   `json:"call_id,omitempty"`   // 工具调用 ID（tool 节点）
	Err       bool     `json:"err,omitempty"`       // 工具结果是否出错
	Name      string   `json:"name,omitempty"`      // 来源标记（reminder / compaction 等）
	Gone      int64    `json:"gone,omitempty"`      // 移除标记（仅归档节点携带）
	Fp        string   `json:"fp,omitempty"`        // 内容指纹，用于跨步骤对齐
}

// ToolInfo 单个工具的 schema 摘要，供 Context Browser 展示。
type ToolInfo struct {
	Name   string `json:"name"`
	Bytes  int    `json:"bytes"`
	Schema string `json:"schema,omitempty"` // JSON 序列化后的 schema（截断上限内）
}

// HeaderEpoch 请求头内容纪元：system prompt / skills 段 / 工具 schema 的
// 一组快照。内容变化时追加新纪元，RequestRecord 通过 HeaderSeq 引用，
// 使浏览器能还原任一历史请求实际携带的头部内容。
type HeaderEpoch struct {
	Seq         int64      `json:"seq"`
	Time        int64      `json:"time"`
	Fp          string     `json:"fp,omitempty"` // 内容指纹，用于纪元去重比对
	SystemBytes int        `json:"system_bytes"`
	SkillsBytes int        `json:"skills_bytes"`
	ToolsBytes  int        `json:"tools_bytes"`
	SystemText  string     `json:"system_text,omitempty"`
	SkillsText  string     `json:"skills_text,omitempty"`
	Tools       []ToolInfo `json:"tools,omitempty"`
}

// RequestRecord 一次 LLM 请求（step）的上下文组成记录。
// 分类字节为请求发出时刻的快照；Prompt/Output/CacheHit 为 provider 真实用量。
type RequestRecord struct {
	Seq            int64 `json:"seq"`
	Time           int64 `json:"time"`
	Turn           int   `json:"turn"`
	Step           int   `json:"step"` // 轮内步骤序号，从 1 开始
	HeaderSeq      int64 `json:"header_seq"`
	System         int   `json:"system"`
	Skills         int   `json:"skills"`
	Tools          int   `json:"tools"`
	User           int   `json:"user"`
	Inject         int   `json:"inject"`
	Assistant      int   `json:"assistant"`
	Tool           int   `json:"tool"`
	Total          int   `json:"total"`
	PromptTokens   int   `json:"prompt_tokens,omitempty"`
	OutputTokens   int   `json:"output_tokens,omitempty"`
	CacheHitTokens int   `json:"cache_hit_tokens,omitempty"`
	ActiveMs       int64 `json:"active_ms,omitempty"`
}

// ContextEvent 上下文事件：压缩、裁剪、注入、模型切换、清空。
type ContextEvent struct {
	Seq    int64  `json:"seq"`
	Time   int64  `json:"time"`
	Kind   string `json:"kind"` // compaction | prune | inject | model | clear
	Bytes  int    `json:"bytes,omitempty"`
	Name   string `json:"name,omitempty"`
	Detail string `json:"detail,omitempty"`
	Turn   int    `json:"turn,omitempty"`
	Step   int    `json:"step,omitempty"`
}

// Counts 全会话累计计数（不随保留边界裁剪）。
type Counts struct {
	Turns       int `json:"turns"`
	Steps       int `json:"steps"`
	HumanInputs int `json:"human_inputs"`
	Compactions int `json:"compactions"`
	Prunes      int `json:"prunes"`
	Injects     int `json:"injects"`
}

// Composition 各分类字节占用。Total 为各部分之和。
type Composition struct {
	System    int `json:"system"`
	Skills    int `json:"skills"`
	Tools     int `json:"tools"`
	User      int `json:"user"`
	Inject    int `json:"inject"`
	Assistant int `json:"assistant"`
	Tool      int `json:"tool"`
	Total     int `json:"total"`
}

// State 折叠器的完整持久化状态，全部字段可 JSON 序列化。
type State struct {
	Seq        int64           `json:"seq"` // 全局序号计数器
	Model      string          `json:"model,omitempty"`
	LimitBytes int             `json:"limit_bytes,omitempty"`
	Surface    []SurfaceNode   `json:"surface"`  // 存活消息节点
	Archive    []SurfaceNode   `json:"archive"`  // 已移除节点（携带 Gone）
	Headers    []HeaderEpoch   `json:"headers"`  // 请求头纪元（最新在末尾）
	Requests   []RequestRecord `json:"requests"` // 逐请求记录
	Events     []ContextEvent  `json:"events"`
	// Sums 消息侧（user/inject/assistant/tool）运行累计，
	// 与 Surface 裁剪解耦：节点被保留边界丢弃时占用统计仍准确。
	Sums         Composition `json:"sums"`
	Counts       Counts      `json:"counts"`
	DetailRev    int64       `json:"detail_rev"` // 详情版本号，每次折叠递增
	DroppedNodes int         `json:"dropped_nodes,omitempty"`
	SurfaceFloor int64       `json:"surface_floor,omitempty"` // 被丢弃存活节点的最新 seq
	ArchiveFloor int64       `json:"archive_floor,omitempty"` // 被丢弃归档节点的最新 Gone
	CurTurn      int         `json:"cur_turn"`
	CurStep      int         `json:"cur_step"`
}

// HeadSnapshot 轻量头部快照：当前组成、计数与最新请求锚点，
// 经 SSE 与 GET /api/context/timeline 高频传输。
type HeadSnapshot struct {
	Model      string         `json:"model,omitempty"`
	LimitBytes int            `json:"limit_bytes"`
	Current    Composition    `json:"current"`
	Counts     Counts         `json:"counts"`
	Last       *RequestRecord `json:"last,omitempty"`
	DetailRev  int64          `json:"detail_rev"`
}

// DetailSnapshot 按需拉取的详情载荷：逐请求历史、事件、存活表面与归档。
type DetailSnapshot struct {
	Rev          int64           `json:"rev"`
	Requests     []RequestRecord `json:"requests"`
	Events       []ContextEvent  `json:"events"`
	Nodes        []SurfaceNode   `json:"nodes"`
	Archive      []SurfaceNode   `json:"archive"`
	Headers      []HeaderEpoch   `json:"headers"`
	DroppedNodes int             `json:"dropped_nodes,omitempty"`
	SurfaceFloor int64           `json:"surface_floor,omitempty"`
	ArchiveFloor int64           `json:"archive_floor,omitempty"`
}
