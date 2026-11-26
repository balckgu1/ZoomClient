// components/ctx/CompositionBar.tsx
//
// 当前上下文组成：七色堆叠条 + 图例。斜线纹理表示剩余空余，
// 每类展示字节数、占窗口百分比与近似 token。

import type { CtxComposition } from "../../types";
import { CAT_META, approxTokens, fmtBytes, pctOf } from "../../lib/ctxinsight";

interface Props {
  comp: CtxComposition;
  limit: number;
}

export function CompositionBar({ comp, limit }: Props) {
  const total = comp.total;
  const occupancy = pctOf(total, limit);
  const free = Math.max(limit - total, 0);

  return (
    <section class="ctx-card">
      <header class="ctx-card__head">
        <h3>当前上下文</h3>
        <span class="ctx-card__meta">
          {fmtBytes(total)} / {fmtBytes(limit)} · {occupancy.toFixed(1)}%
        </span>
      </header>
      <div class="ctx-comp__track" role="img" aria-label={`上下文占用 ${occupancy.toFixed(1)}%`}>
        {CAT_META.map((m) => {
          const bytes = comp[m.cat];
          if (bytes <= 0 || limit <= 0) return null;
          return (
            <span
              key={m.cat}
              class="ctx-comp__seg"
              style={{ width: `${pctOf(bytes, limit)}%`, background: m.color }}
              title={`${m.label}：${fmtBytes(bytes)}`}
            />
          );
        })}
        {limit > 0 && free > 0 && (
          <span class="ctx-comp__free" style={{ width: `${pctOf(free, limit)}%` }} />
        )}
      </div>
      <ul class="ctx-comp__legend">
        {CAT_META.map((m) => {
          const bytes = comp[m.cat];
          return (
            <li key={m.cat} class="ctx-comp__row">
              <span class="ctx-comp__dot" style={{ background: m.color }} />
              <span class="ctx-comp__label">{m.label}</span>
              <span class="ctx-comp__bytes">{fmtBytes(bytes)}</span>
              <span class="ctx-comp__pct">{pctOf(bytes, limit).toFixed(1)}%</span>
              <span class="ctx-comp__tokens">≈{approxTokens(bytes).toLocaleString()} tok</span>
            </li>
          );
        })}
      </ul>
    </section>
  );
}