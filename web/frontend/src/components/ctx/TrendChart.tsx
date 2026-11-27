// components/ctx/TrendChart.tsx
//
// 上下文趋势：手写 SVG 逐请求堆叠柱状图（零依赖）。
// 支持 Step/Turn 粒度与 Total/Delta 模式切换；压缩/裁剪事件在对应柱上标 ✂；
// hover 显示该步组成与 provider 真实用量，点击柱选中该步联动 Context Browser。

import { useMemo, useState } from "preact/hooks";
import type { CtxEvent, CtxRequestRecord } from "../../types";
import {
  CAT_META,
  buildTrendBars,
  fmtBytes,
  fmtTime,
  pctOf,
} from "../../lib/ctxinsight";

interface Props {
  requests: CtxRequestRecord[];
  events: CtxEvent[];
  selectedSeq: number | null;
  onSelect: (seq: number) => void;
}

// 图表几何常量
const BAR_W = 18;
const GAP = 6;
const CHART_H = 140;
const LABEL_H = 18;

export function TrendChart({ requests, events, selectedSeq, onSelect }: Props) {
  const [granularity, setGranularity] = useState<"step" | "turn">("step");
  const [mode, setMode] = useState<"total" | "delta">("total");
  const [hoverIdx, setHoverIdx] = useState<number | null>(null);

  const bars = useMemo(
    () => buildTrendBars(requests, granularity, mode),
    [requests, granularity, mode]
  );

  // 压缩/裁剪事件 → 柱下标：事件发生前最后一个请求即受影响的柱
  const marks = useMemo(() => {
    const out = new Map<number, string>();
    for (const e of events) {
      if (e.kind !== "compaction" && e.kind !== "prune") continue;
      for (let i = bars.length - 1; i >= 0; i--) {
        if (bars[i].req.seq < e.seq) {
          out.set(i, e.kind === "compaction" ? "✂" : "⌫");
          break;
        }
      }
    }
    return out;
  }, [events, bars]);

  if (bars.length === 0) {
    return (
      <section class="ctx-card">
        <header class="ctx-card__head"><h3>上下文趋势</h3></header>
        <p class="ctx-empty">还没有模型请求。发送一条消息后，这里会逐请求展示上下文的增长。</p>
      </section>
    );
  }

  const maxTotal = Math.max(...bars.map((b) => b.req.total), 1);
  const maxAbsNet = Math.max(...bars.map((b) => Math.abs(b.net)), 1);
  const width = bars.length * (BAR_W + GAP) + GAP;
  const height = CHART_H + LABEL_H;
  // Delta 模式基线居中，正增长向上、压缩回落向下
  const baseline = mode === "delta" ? CHART_H / 2 : CHART_H;

  const hover = hoverIdx !== null ? bars[hoverIdx] : null;

  return (
    <section class="ctx-card">
      <header class="ctx-card__head">
        <h3>上下文趋势</h3>
        <div class="ctx-trend__toggles">
          <div class="ctx-seg">
            <button
              class={granularity === "step" ? "is-on" : ""}
              onClick={() => setGranularity("step")}
            >步</button>
            <button
              class={granularity === "turn" ? "is-on" : ""}
              onClick={() => setGranularity("turn")}
            >轮</button>
          </div>
          <div class="ctx-seg">
            <button
              class={mode === "total" ? "is-on" : ""}
              onClick={() => setMode("total")}
            >总量</button>
            <button
              class={mode === "delta" ? "is-on" : ""}
              onClick={() => setMode("delta")}
            >增量</button>
          </div>
        </div>
      </header>
      <div class="ctx-trend__scroll">
        <div class="ctx-trend__canvas" style={{ width }}>
          <svg width={width} height={height} role="img" aria-label="上下文趋势图">
            {/* 基线 */}
            <line x1={0} y1={baseline} x2={width} y2={baseline} class="ctx-trend__axis" />
            {bars.map((b, i) => {
              const x = GAP + i * (BAR_W + GAP);
              const selected = b.req.seq === selectedSeq;
              let segments: { y: number; h: number; color: string; key: string }[] = [];
              if (mode === "total") {
                let acc = 0;
                for (const m of CAT_META) {
                  const v = b.req[m.cat];
                  if (v <= 0) continue;
                  const h = (v / maxTotal) * (CHART_H - 14);
                  segments.push({
                    y: baseline - acc - h,
                    h,
                    color: m.color,
                    key: m.cat,
                  });
                  acc += h;
                }
              } else {
                const h = (Math.abs(b.net) / maxAbsNet) * (CHART_H / 2 - 10);
                segments = [{
                  y: b.net >= 0 ? baseline - h : baseline,
                  h: Math.max(h, 1),
                  color: b.net >= 0 ? "var(--mauve)" : "var(--peach)",
                  key: "net",
                }];
              }
              // 轮模式下标注轮号（每轮首柱）
              const prev = bars[i - 1];
              const showTurn = !prev || prev.req.turn !== b.req.turn;
              return (
                <g
                  key={b.req.seq}
                  class={`ctx-trend__bar ${selected ? "is-selected" : ""}`}
                  onMouseEnter={() => setHoverIdx(i)}
                  onMouseLeave={() => setHoverIdx(null)}
                  onClick={() => onSelect(b.req.seq)}
                >
                  {/* 透明命中区，保证矮柱也能 hover/点击 */}
                  <rect x={x} y={0} width={BAR_W} height={CHART_H} fill="transparent" />
                  {segments.map((s) => (
                    <rect
                      key={s.key}
                      x={x}
                      y={s.y}
                      width={BAR_W}
                      height={s.h}
                      fill={s.color}
                      rx={1.5}
                    />
                  ))}
                  {selected && (
                    <rect
                      x={x - 1.5}
                      y={Math.min(...segments.map((s) => s.y)) - 1.5}
                      width={BAR_W + 3}
                      height={segments.reduce((a, s) => a + s.h, 0) + 3}
                      fill="none"
                      stroke="var(--fg-1)"
                      stroke-width={1}
                      rx={2.5}
                    />
                  )}
                  {marks.has(i) && (
                    <text x={x + BAR_W / 2} y={10} class="ctx-trend__mark" text-anchor="middle">
                      {marks.get(i)}
                    </text>
                  )}
                  {showTurn && (
                    <text x={x + BAR_W / 2} y={CHART_H + 13} class="ctx-trend__label" text-anchor="middle">
                      T{b.req.turn}
                    </text>
                  )}
                </g>
              );
            })}
          </svg>
          {hover && hoverIdx !== null && (
            <div
              class="ctx-trend__tip"
              style={{
                left: Math.min(GAP + hoverIdx * (BAR_W + GAP), width - 230),
              }}
            >
              <div class="ctx-trend__tip-head">
                T{hover.req.turn} · S{hover.req.step}
                {granularity === "turn" && hover.stepCount > 1 && `（${hover.stepCount} 步）`}
                <span>{fmtTime(hover.req.time)}</span>
              </div>
              <div class="ctx-trend__tip-total">
                {mode === "delta"
                  ? `${hover.net >= 0 ? "+" : ""}${fmtBytes(hover.net)}`
                  : fmtBytes(hover.req.total)}
                {hover.req.active_ms ? ` · ${hover.req.active_ms} ms` : ""}
              </div>
              <ul>
                {CAT_META.map((m) => {
                  const v = hover.req[m.cat];
                  if (v <= 0) return null;
                  return (
                    <li key={m.cat}>
                      <span class="ctx-comp__dot" style={{ background: m.color }} />
                      {m.label}
                      <span class="ctx-trend__tip-v">{fmtBytes(v)} · {pctOf(v, hover.req.total).toFixed(0)}%</span>
                    </li>
                  );
                })}
              </ul>
              {(hover.req.prompt_tokens || hover.req.output_tokens) && (
                <div class="ctx-trend__tip-actual">
                  实际 prompt {hover.req.prompt_tokens?.toLocaleString() ?? 0} · output{" "}
                  {hover.req.output_tokens?.toLocaleString() ?? 0}
                  {hover.req.cache_hit_tokens
                    ? ` · cache ${hover.req.cache_hit_tokens.toLocaleString()}`
                    : ""}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
      <p class="ctx-card__note">✂ 压缩 ⌫ 裁剪 · 点击柱可在下方浏览器查看该请求的实际组装</p>
    </section>
  );
}