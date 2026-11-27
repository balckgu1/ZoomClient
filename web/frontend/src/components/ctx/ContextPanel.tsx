// components/ctx/ContextPanel.tsx
//
// 上下文洞察面板（模态）：组成条 + 趋势图 + 浏览器 + 事件日志。
// 头部快照由 App 经 SSE 维护并传入；详情在面板打开期间按 detailRev 增量重拉。

import { useEffect, useRef, useState } from "preact/hooks";
import type { ContextTimelineDetail, ContextTimelineHead } from "../../types";
import { fetchContextTimeline, fetchContextTimelineDetail } from "../../lib/api";
import { fmtBytes } from "../../lib/ctxinsight";
import { CompositionBar } from "./CompositionBar";
import { TrendChart } from "./TrendChart";
import { ContextBrowser } from "./ContextBrowser";
import { EventsList } from "./EventsList";

interface Props {
  // head 为 SSE 维护的头部快照；为 null 时面板自行拉取一次
  head: ContextTimelineHead | null;
  onClose: () => void;
}

export function ContextPanel({ head, onClose }: Props) {
  const [localHead, setLocalHead] = useState<ContextTimelineHead | null>(null);
  const [detail, setDetail] = useState<ContextTimelineDetail | null>(null);
  const [error, setError] = useState("");
  // selectedSeq 为趋势图/浏览器共享的选中请求；null 表示 Live
  const [selectedSeq, setSelectedSeq] = useState<number | null>(null);
  // fetchedRev 记录已拉取详情的版本号，避免同一版本重复请求
  const fetchedRev = useRef<number>(-1);

  const effectiveHead = head ?? localHead;
  const rev = effectiveHead?.detail_rev ?? 0;

  // 打开时与每次 detailRev 变化时重拉详情；head 缺失时补拉头部
  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        if (!head) {
          const h = await fetchContextTimeline();
          if (cancelled) return;
          setLocalHead(h);
        }
        if (fetchedRev.current === rev && detail) return;
        const d = await fetchContextTimelineDetail();
        if (cancelled) return;
        fetchedRev.current = d.rev;
        setDetail(d);
        setError("");
      } catch (err) {
        if (!cancelled) setError(String(err));
      }
    };
    load();
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rev]);

  // Esc 关闭
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const counts = effectiveHead?.counts;

  return (
    <div class="modal-overlay" onClick={onClose}>
      <div class="ctx-panel" onClick={(e) => e.stopPropagation()} role="dialog" aria-label="上下文洞察">
        <header class="ctx-panel__head">
          <div class="ctx-panel__title">
            <h2>上下文洞察</h2>
            {effectiveHead?.model && <span class="ctx-panel__model">{effectiveHead.model}</span>}
          </div>
          {counts && (
            <div class="ctx-panel__stats">
              <span><b>{counts.turns}</b> 轮</span>
              <span><b>{counts.steps}</b> 请求</span>
              <span><b>{counts.human_inputs}</b> 次输入</span>
              <span><b>{counts.compactions}</b> 压缩</span>
              <span><b>{counts.prunes}</b> 裁剪</span>
            </div>
          )}
          <button class="ctx-panel__close" onClick={onClose} aria-label="关闭">✕</button>
        </header>
        <div class="ctx-panel__body">
          {error && <p class="ctx-empty is-error">加载失败：{error}</p>}
          {!effectiveHead && !error && <p class="ctx-empty">加载中…</p>}
          {effectiveHead && (
            <CompositionBar comp={effectiveHead.current} limit={effectiveHead.limit_bytes} />
          )}
          {detail && (
            <>
              <TrendChart
                requests={detail.requests}
                events={detail.events}
                selectedSeq={selectedSeq}
                onSelect={(seq) => setSelectedSeq(seq)}
              />
              <ContextBrowser
                detail={detail}
                selectedSeq={selectedSeq}
                onSelect={setSelectedSeq}
              />
              <EventsList events={detail.events} />
            </>
          )}
          {effectiveHead && !detail && !error && <p class="ctx-empty">详情加载中…</p>}
          {effectiveHead && effectiveHead.counts.steps === 0 && (
            <p class="ctx-card__note">
              本会话还没有请求记录{effectiveHead.limit_bytes > 0
                ? `（上下文阈值 ${fmtBytes(effectiveHead.limit_bytes)}）`
                : ""}。旧会话若未保存洞察快照，历史趋势不可恢复。
            </p>
          )}
        </div>
      </div>
    </div>
  );
}