import type { ChatMessage, PersistedMessage } from "../types";

// 本文件把后端持久化的原始消息（fsm.Message[]）无损重建为前端展示用的
// ChatMessage[]，使"从磁盘重载历史"的视图与"实时 SSE 渲染"的视图保持一致。
//
// 关键点：调用工具的 assistant 轮次持久化时 content 为空、调用信息在 tool_calls
// 字段中，工具结果单独以 role:"tool" + tool_call_id 存储。旧实现忽略了 tool_calls
// 并跳过 tool 消息，导致切换会话再切回后工具卡片丢失、该轮渲染为空气泡。

/** 按 Unicode 字符安全截断（正确处理中文/emoji 等多字节字符）。 */
function safeTruncate(str: string, maxLen: number): string {
  const chars = Array.from(str);
  if (chars.length <= maxLen) return str;
  return chars.slice(0, maxLen).join("") + "…";
}

/** 把 fsm.Message.Content（可能是字符串、数组或 null）安全转为可读字符串。 */
export function contentToStr(v: unknown): string {
  if (v === null || v === undefined) return "";
  if (typeof v === "string") return v;
  try {
    return JSON.stringify(v);
  } catch {
    return String(v);
  }
}

/**
 * 生成工具参数预览，镜像后端 formatArgsPreview（agent_loop.go）：
 * JSON 序列化后按 120 字符截断。空参数返回空串（卡片不渲染参数区）。
 */
export function previewArgs(args?: Record<string, unknown>): string {
  if (!args || Object.keys(args).length === 0) return "";
  let s: string;
  try {
    s = JSON.stringify(args);
  } catch {
    s = String(args);
  }
  return safeTruncate(s, 120);
}

/**
 * 把持久化的 fsm.Message[] 重建为 ChatMessage[]。
 *
 * @param raw    后端 SessionRecord.messages（原始 JSON）
 * @param genId  稳定 key 生成器（与实时渲染共用同一计数器）
 *
 * 重建规则：
 *  - user      → user 气泡
 *  - assistant → 依次推 reasoning（若有）、assistant 文本（若非空）、每个 tool_call 一张卡片
 *  - tool      → 按 tool_call_id 命中对应卡片并回填结果；命中失败则回退到最近一张未决卡片
 *  - 其它（system 等）跳过
 *
 * 限制：持久化的 tool 消息未存储 isError 标记，故重建的结果卡片默认按成功（✅）显示。
 */
export function reconstructMessages(
  raw: PersistedMessage[] | undefined,
  genId: () => number
): ChatMessage[] {
  const out: ChatMessage[] = [];
  if (!raw || raw.length === 0) return out;

  // tool_call_id → out 中对应 tool_call 卡片的下标，用于精确回填结果
  const idToIndex = new Map<string, number>();

  for (const m of raw) {
    if (m.role === "user") {
      out.push({ _id: genId(), role: "user", content: contentToStr(m.content) });
      continue;
    }

    if (m.role === "assistant") {
      if (m.reasoning_content) {
        out.push({ _id: genId(), role: "reasoning", content: String(m.reasoning_content) });
      }
      const text = contentToStr(m.content);
      // 仅在确有文本时推 assistant 气泡，避免"调用工具的空 content 轮次"渲染成空气泡
      if (text) {
        out.push({ _id: genId(), role: "assistant", content: text });
      }
      for (const tc of m.tool_calls ?? []) {
        const idx = out.length;
        out.push({
          _id: genId(),
          role: "tool_call",
          name: tc.name,
          args: previewArgs(tc.arguments),
        });
        if (tc.id) idToIndex.set(tc.id, idx);
      }
      continue;
    }

    if (m.role === "tool") {
      const result = contentToStr(m.content);
      // 优先按 tool_call_id 精确命中；否则回退到最近一张尚未回填结果的卡片
      let target = m.tool_call_id ? idToIndex.get(m.tool_call_id) : undefined;
      if (target === undefined) {
        for (let i = out.length - 1; i >= 0; i--) {
          const c = out[i];
          if (c.role === "tool_call" && c.result === undefined) {
            target = i;
            break;
          }
        }
      }
      if (target !== undefined) {
        const card = out[target];
        if (card.role === "tool_call") {
          out[target] = { ...card, result };
        }
      }
      continue;
    }
    // system 等其它角色：不展示
  }

  return out;
}
