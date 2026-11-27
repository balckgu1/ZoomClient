// src/lib/ctxinsight.ts
//
// 上下文洞察面板的纯逻辑辅助：分类元信息、尺寸格式化、趋势变换与历史重建。
// 与组件分离放在这里，便于推理与复用（口径与后端 ctxinsight 包一致）。

import type {
  CtxCategory,
  CtxComposition,
  CtxHeaderEpoch,
  CtxRequestRecord,
  CtxSurfaceNode,
  ContextTimelineDetail,
} from "../types";

// CAT_META 分类的展示元信息：顺序即堆叠条/图例的排列顺序。
// 颜色沿用 Catppuccin 信息色约定：彩色只出现在承载信息的地方。
export const CAT_META: { cat: CtxCategory; label: string; color: string }[] = [
  { cat: "system", label: "系统提示词", color: "var(--fg-3)" },
  { cat: "skills", label: "Skill 目录", color: "var(--teal)" },
  { cat: "tools", label: "工具 Schema", color: "var(--blue)" },
  { cat: "user", label: "用户消息", color: "var(--green)" },
  { cat: "inject", label: "注入提醒", color: "var(--yellow)" },
  { cat: "assistant", label: "助手回复", color: "var(--mauve)" },
  { cat: "tool", label: "工具结果", color: "var(--peach)" },
];

// catColor 返回分类对应的 CSS 颜色变量
export function catColor(cat: CtxCategory): string {
  return CAT_META.find((m) => m.cat === cat)?.color ?? "var(--fg-4)";
}

// catLabel 返回分类的中文标签
export function catLabel(cat: CtxCategory): string {
  return CAT_META.find((m) => m.cat === cat)?.label ?? cat;
}

// fmtBytes 把字节数格式化为人类可读尺寸（B / KB / MB）
export function fmtBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "0 B";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(2)} MB`;
}

// approxTokens 字节 → 近似 token（与后端 estimateTokens 的 chars/4 启发式同口径）
export function approxTokens(bytes: number): number {
  return Math.round(bytes / 4);
}

// pctOf 计算部分占整体的百分比（整体无效时返回 0）
export function pctOf(part: number, whole: number): number {
  if (whole <= 0) return 0;
  return (part / whole) * 100;
}

// compositionOf 从请求记录提取分类组成
export function compositionOf(req: CtxRequestRecord): CtxComposition {
  return {
    system: req.system,
    skills: req.skills,
    tools: req.tools,
    user: req.user,
    inject: req.inject,
    assistant: req.assistant,
    tool: req.tool,
    total: req.total,
  };
}

// TrendBar 趋势图的一根柱：携带来源请求与派生字段
export interface TrendBar {
  req: CtxRequestRecord;
  // stepCount 轮模式下该轮聚合的请求数
  stepCount: number;
  // net Delta 模式的带符号净变化（首根柱为自身 total）
  net: number;
}

// aggregateByTurn 轮粒度聚合：每轮取最后一步的记录作为该轮柱，
// stepCount 记录轮内步数。输入须按时间升序。
export function aggregateByTurn(requests: CtxRequestRecord[]): CtxRequestRecord[] {
  const out: CtxRequestRecord[] = [];
  for (const req of requests) {
    const last = out[out.length - 1];
    if (last && last.turn === req.turn) {
      out[out.length - 1] = req;
    } else {
      out.push(req);
    }
  }
  return out;
}

// buildTrendBars 生成趋势柱序列：granularity 决定步/轮粒度，
// delta 模式计算相邻柱的带符号净变化。
export function buildTrendBars(
  requests: CtxRequestRecord[],
  granularity: "step" | "turn",
  mode: "total" | "delta"
): TrendBar[] {
  const base = granularity === "turn" ? aggregateByTurn(requests) : requests;
  // 轮模式下统计每轮步数
  const stepCounts = new Map<number, number>();
  if (granularity === "turn") {
    for (const req of requests) {
      stepCounts.set(req.turn, (stepCounts.get(req.turn) ?? 0) + 1);
    }
  }
  const bars: TrendBar[] = [];
  let prevTotal = 0;
  for (let i = 0; i < base.length; i++) {
    const req = base[i];
    const net = mode === "delta" ? (i === 0 ? req.total : req.total - prevTotal) : req.total;
    bars.push({ req, stepCount: stepCounts.get(req.turn) ?? 1, net });
    prevTotal = req.total;
  }
  return bars;
}

// aliveNodesAt 重建请求 reqSeq 时刻的存活表面：
// seq < reqSeq 且 (gone 未设 或 gone > reqSeq)。合并存活节点与归档节点。
export function aliveNodesAt(detail: ContextTimelineDetail, reqSeq: number): CtxSurfaceNode[] {
  const alive = (n: CtxSurfaceNode) => n.seq < reqSeq && (!n.gone || n.gone > reqSeq);
  return [...detail.nodes.filter(alive), ...detail.archive.filter(alive)].sort(
    (a, b) => a.seq - b.seq
  );
}

// headerForStep 找到请求 reqSeq 时刻生效的请求头纪元（最后一个 seq <= reqSeq）。
// 早于所有纪元时返回 undefined（浏览器降级为不展示头部内容）。
export function headerForStep(
  headers: CtxHeaderEpoch[],
  reqSeq: number
): CtxHeaderEpoch | undefined {
  let found: CtxHeaderEpoch | undefined;
  for (const h of headers) {
    if (h.seq <= reqSeq) found = h;
    else break;
  }
  return found;
}

// fmtTime 把 Unix 毫秒格式化为 HH:MM:SS
export function fmtTime(ms: number): string {
  if (!ms) return "--:--:--";
  const d = new Date(ms);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}