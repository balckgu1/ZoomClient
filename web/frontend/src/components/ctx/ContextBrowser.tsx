// components/ctx/ContextBrowser.tsx
//
// 上下文浏览器：打开任一请求的"盒子"，按分类展开每条元素的实际内容。
// Live 视图展示当前表面；选择历史步骤时用 seq/gone 判定重建该请求时刻的表面。

import { useMemo, useState } from "preact/hooks";
import type { ContextTimelineDetail, CtxCategory } from "../../types";
import {
  aliveNodesAt,
  catColor,
  catLabel,
  fmtBytes,
  headerForStep,
} from "../../lib/ctxinsight";

interface Props {
  detail: ContextTimelineDetail;
  selectedSeq: number | null;
  onSelect: (seq: number | null) => void;
}

// Row 浏览器中的一行元素
interface Row {
  key: string;
  title: string;
  bytes: number;
  text?: string;
  err?: boolean;
  truncated?: boolean;
  sub?: string;
}

// Section 一个分类分区
interface Section {
  cat: CtxCategory;
  rows: Row[];
  bytes: number;
}

// nodeTitle 节点行的标题：工具名 / 来源标记 / 内容首行
function nodeTitle(n: { cat: CtxCategory; tool?: string; name?: string; text?: string }): string {
  if (n.tool) return n.tool;
  if (n.name) return n.name;
  const first = (n.text ?? "").split("\n")[0];
  return first.length > 60 ? first.slice(0, 60) + "…" : first || "(空)";
}

export function ContextBrowser({ detail, selectedSeq, onSelect }: Props) {
  // openCats 记录展开的分类；openRows 记录展开内容的行
  const [openCats, setOpenCats] = useState<Set<string>>(
    () => new Set(["user", "inject", "assistant", "tool"])
  );
  const [openRows, setOpenRows] = useState<Set<string>>(() => new Set());

  const isLive = selectedSeq === null;
  const header = isLive
    ? detail.headers[detail.headers.length - 1]
    : headerForStep(detail.headers, selectedSeq);
  const nodes = isLive ? detail.nodes : aliveNodesAt(detail, selectedSeq);

  // sections 组装七个分类分区：头部三类来自纪元快照，消息四类来自表面节点
  const sections = useMemo<Section[]>(() => {
    const out: Section[] = [];
    if (header) {
      out.push({
        cat: "system",
        bytes: header.system_bytes,
        rows: [{ key: "system", title: "系统提示词", bytes: header.system_bytes, text: header.system_text }],
      });
      if (header.skills_bytes > 0) {
        out.push({
          cat: "skills",
          bytes: header.skills_bytes,
          rows: [{ key: "skills", title: "Skill 目录", bytes: header.skills_bytes, text: header.skills_text }],
        });
      }
      if (header.tools && header.tools.length > 0) {
        out.push({
          cat: "tools",
          bytes: header.tools_bytes,
          rows: header.tools.map((t) => ({
            key: `tool-schema-${t.name}`,
            title: t.name,
            bytes: t.bytes,
            text: t.schema,
          })),
        });
      }
    }
    for (const cat of ["user", "inject", "assistant", "tool"] as CtxCategory[]) {
      const catNodes = nodes.filter((n) => n.cat === cat);
      if (catNodes.length === 0) continue;
      out.push({
        cat,
        bytes: catNodes.reduce((a, n) => a + n.bytes, 0),
        rows: catNodes.map((n) => ({
          key: `node-${n.seq}`,
          title: nodeTitle(n),
          bytes: n.bytes,
          text: n.text,
          err: n.err,
          truncated: n.truncated,
          sub: n.name && n.tool ? n.name : undefined,
        })),
      });
    }
    return out;
  }, [header, nodes]);

  const toggleCat = (cat: string) => {
    setOpenCats((prev) => {
      const next = new Set(prev);
      if (next.has(cat)) next.delete(cat);
      else next.add(cat);
      return next;
    });
  };
  const toggleRow = (key: string) => {
    setOpenRows((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  return (
    <section class="ctx-card">
      <header class="ctx-card__head">
        <h3>上下文浏览器</h3>
        <select
          class="ctx-browser__picker"
          value={isLive ? "live" : String(selectedSeq)}
          onChange={(e) => {
            const v = (e.target as HTMLSelectElement).value;
            onSelect(v === "live" ? null : Number(v));
          }}
        >
          <option value="live">Live（当前表面）</option>
          {[...detail.requests].reverse().map((r) => (
            <option key={r.seq} value={r.seq}>
              T{r.turn} · S{r.step} — {fmtBytes(r.total)}
            </option>
          ))}
        </select>
      </header>
      {!isLive && (
        <p class="ctx-card__note">
          正在查看历史请求的组装重建（seq/gone 判定）
          {detail.archive_floor ? "；更早的移除记录已被保留边界裁剪，重建可能不完整" : ""}
        </p>
      )}
      {sections.length === 0 && <p class="ctx-empty">该请求暂无可浏览的内容。</p>}
      <div class="ctx-browser__sections">
        {sections.map((sec) => {
          const open = openCats.has(sec.cat);
          return (
            <div key={sec.cat} class="ctx-browser__section">
              <button class="ctx-browser__cat" onClick={() => toggleCat(sec.cat)}>
                <span class="ctx-browser__caret">{open ? "▾" : "▸"}</span>
                <span class="ctx-comp__dot" style={{ background: catColor(sec.cat) }} />
                <span>{catLabel(sec.cat)}</span>
                <span class="ctx-browser__count">{sec.rows.length}</span>
                <span class="ctx-browser__bytes">{fmtBytes(sec.bytes)}</span>
              </button>
              {open && (
                <ul class="ctx-browser__rows">
                  {sec.rows.map((row) => {
                    const rowOpen = openRows.has(row.key);
                    return (
                      <li key={row.key} class="ctx-browser__row">
                        <button class="ctx-browser__row-head" onClick={() => toggleRow(row.key)}>
                          <span class="ctx-browser__caret">{rowOpen ? "▾" : "▸"}</span>
                          <span class={`ctx-browser__row-title ${row.err ? "is-error" : ""}`}>
                            {row.title}
                            {row.err && <span class="ctx-browser__err">错误</span>}
                          </span>
                          <span class="ctx-browser__bytes">{fmtBytes(row.bytes)}</span>
                        </button>
                        {rowOpen && (
                          <div class="ctx-browser__content">
                            {row.truncated && (
                              <p class="ctx-browser__truncated">内容过长，已截断展示</p>
                            )}
                            <pre>{row.text || "(无内容)"}</pre>
                          </div>
                        )}
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}