// components/ctx/EventsList.tsx
//
// 上下文事件日志：压缩、裁剪、注入、模型切换、清空。
// kind 筛选 chips 携带全量计数，行内展示净字节变化与来源说明。

import { useMemo, useState } from "preact/hooks";
import type { CtxEvent } from "../../types";
import { fmtBytes, fmtTime } from "../../lib/ctxinsight";

interface Props {
  events: CtxEvent[];
}

// KIND_META 事件类型的展示元信息
const KIND_META: { kind: CtxEvent["kind"]; label: string; color: string }[] = [
  { kind: "compaction", label: "压缩", color: "var(--peach)" },
  { kind: "prune", label: "裁剪", color: "var(--yellow)" },
  { kind: "inject", label: "注入", color: "var(--teal)" },
  { kind: "model", label: "模型", color: "var(--blue)" },
  { kind: "clear", label: "清空", color: "var(--red)" },
];

export function EventsList({ events }: Props) {
  const [filter, setFilter] = useState<CtxEvent["kind"] | null>(null);

  const counts = useMemo(() => {
    const m = new Map<string, number>();
    for (const e of events) m.set(e.kind, (m.get(e.kind) ?? 0) + 1);
    return m;
  }, [events]);

  const shown = filter ? events.filter((e) => e.kind === filter) : events;

  return (
    <section class="ctx-card">
      <header class="ctx-card__head">
        <h3>上下文事件</h3>
        <span class="ctx-card__meta">{events.length} 条</span>
      </header>
      <div class="ctx-events__chips">
        {KIND_META.map((m) => {
          const n = counts.get(m.kind) ?? 0;
          if (n === 0) return null;
          return (
            <button
              key={m.kind}
              class={`ctx-events__chip ${filter === m.kind ? "is-on" : ""}`}
              style={{ "--chip-color": m.color } as Record<string, string>}
              onClick={() => setFilter(filter === m.kind ? null : m.kind)}
            >
              {m.label} {n}
            </button>
          );
        })}
      </div>
      {shown.length === 0 ? (
        <p class="ctx-empty">暂无事件。压缩、注入提醒、切换模型等操作会记录在这里。</p>
      ) : (
        <ul class="ctx-events__list">
          {[...shown].reverse().map((e) => {
            const meta = KIND_META.find((m) => m.kind === e.kind);
            return (
              <li key={e.seq} class="ctx-events__row">
                <span class="ctx-events__time">{fmtTime(e.time)}</span>
                <span
                  class="ctx-events__kind"
                  style={{ color: meta?.color ?? "var(--fg-2)" }}
                >
                  {meta?.label ?? e.kind}
                </span>
                <span class="ctx-events__name">{e.name || ""}</span>
                <span class="ctx-events__detail">{e.detail || ""}</span>
                {e.bytes ? (
                  <span class="ctx-events__bytes">
                    {e.kind === "compaction" || e.kind === "prune" ? "−" : "+"}
                    {fmtBytes(e.bytes)}
                  </span>
                ) : null}
                {e.turn ? <span class="ctx-events__turn">T{e.turn}{e.step ? `·S${e.step}` : ""}</span> : null}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}