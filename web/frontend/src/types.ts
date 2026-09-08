// SSE event from backend
export interface SSEEvent {
  ch: "agent" | "system" | "emotion";
  data: Record<string, unknown>;
}

// Chat message types displayed in the UI
export type ChatMessage =
  | { _id: number; role: "user"; content: string }
  | { _id: number; role: "assistant"; content: string; streaming?: boolean }
  | { _id: number; role: "reasoning"; content: string }
  | { _id: number; role: "tool_call"; name: string; args: string; result?: string; isError?: boolean }
  | { _id: number; role: "sub_agent"; prompt: string }
  | { _id: number; role: "hook_blocked"; tool: string; reason: string }
  | { _id: number; role: "system"; content: string };

// Permission ask event from backend
export interface PermissionAsk {
  id: string;
  tool: string;
  args: string;
  reason: string;
}

// Session metadata (for sidebar list)
export interface SessionMeta {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
  turn_count: number;
  workdir: string;
}

// 后端持久化的原始工具调用（对应 Go 的 tools.ToolCall）
export interface PersistedToolCall {
  id?: string;
  name: string;
  arguments?: Record<string, unknown>;
}

// 后端持久化的原始消息（对应 Go 的 fsm.Message）。
// 与前端展示用的 ChatMessage 不同：调用工具的 assistant 轮次 content 为空、
// 调用信息在 tool_calls 中，工具结果单独以 role:"tool" + tool_call_id 存储。
export interface PersistedMessage {
  role: string;
  content?: unknown; // 字符串或工具结果数组
  tool_calls?: PersistedToolCall[];
  tool_call_id?: string;
  reasoning_content?: string;
}

// Full session record (with messages)
export interface SessionRecord extends SessionMeta {
  messages: PersistedMessage[];
  model: string;
}

// App state
export interface AppState {
  messages: ChatMessage[];
  model: string;
  connected: boolean;
  busy: boolean;
  turnCount: number;
  pendingPermission: PermissionAsk | null;
}

// Model preset (for model selector)
export interface ModelPreset {
  name: string;
  type: string;
  base_url?: string;
  api_key?: string;
  model_name: string;
}

// Model list response
export interface ModelsResponse {
  models: ModelPreset[];
  active: string;
}

// ─── 工作目录 ───

// GET /api/workdir 的响应体
export interface WorkDirResponse {
  path: string;
}

// ─── 权限管理 ───

// 权限模式：
//   default —— 未命中规则时询问用户
//   plan    —— 只读，禁止任何写/执行类工具
//   auto    —— 只读工具自动放行，写/执行类工具询问用户
export type PermissionMode = "default" | "plan" | "auto";

// 单条权限规则，与后端 permission.Rule 对应。
//   tool     —— 目标工具名，"" 或 "*" 表示任意工具
//   behavior —— 命中后的行为（allow / deny / ask）
//   path     —— 可选，匹配 filename / path / file 参数（支持 "re:" 前缀正则）
//   content  —— 可选，匹配 command / content / prompt 参数（支持 "re:" 前缀正则）
export interface PermissionRule {
  tool: string;
  behavior: string;
  path: string;
  content: string;
}

// GET /api/permission/config 的响应体，用于权限管理面板展示与编辑
export interface PermissionConfig {
  mode: PermissionMode;
  interactive: boolean;
  deny_rules: PermissionRule[];
  allow_rules: PermissionRule[];
}

// ─── 上下文窗口占用 ───

// 上下文占用快照（对应 Go compact.UsageSnapshot），所有尺寸单位为字节
export interface ContextUsage {
  limit_bytes: number;         // 配置的上下文总阈值
  total_bytes: number;         // 各部分之和
  system_prompt_bytes: number; // system prompt 主体（不含 skills 段）
  skills_bytes: number;        // skills 目录段
  skills_count: number;        // 已加载的 skill 数量
  tools_bytes: number;         // 工具 schema
  messages_bytes: number;      // 消息历史估算
}

// ─── 技能目录 ───

// 单个 skill 的展示元信息（对应 Go 的 skills.SkillManifest）。
// 后端只返回 frontmatter 级字段，不含 SKILL.md 正文——正文由模型调用 load_skill 工具载入。
export interface SkillMeta {
  name: string;
  description: string;
  version?: string;
  author?: string;
  compatibility?: string;
}

// GET /api/skills 的响应体
export interface SkillsResponse {
  skills: SkillMeta[];
  count: number;
}
