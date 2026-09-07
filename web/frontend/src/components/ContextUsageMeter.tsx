import { useRef, useState } from "preact/hooks";
import type { ContextUsage } from "../types";

// Props —— 上下文占用快照与主动压缩回调
interface Props {
  usage: ContextUsage | null;
  onCompact: () => void;
}

// usagePct 计算某部分字节数占总阈值的百分比；阈值无效时返回 0
function usagePct(bytes: number, limit: number): number {
  if (limit <= 0) return 0;
  return Math.round((bytes / limit) * 100);
}

// levelClass 按百分比返回着色修饰类：>=100% 报错色、>=85% 警告色
function levelClass(pct: number): string {
  if (pct >= 100) return "is-error";
  if (pct >= 85) return "is-warning";
  return "";
}

// ContextUsageMeter —— 右下角上下文窗口占用指示器。
// 折叠态仅显示迷你进度条与总百分比；悬停展开明细面板（各组成部分占比 + 压缩按钮），
// 鼠标移开延迟 200ms 关闭，保证指针可以移入面板内操作。
export function ContextUsageMeter({ usage, onCompact }: Props) {
  const [open, setOpen] = useState(false);
  // closeTimer 延迟关闭定时器：进入面板时取消，避免面板闪闭
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const limit = usage ? usage.limit_bytes : 0;
  const loaded = !!usage && limit > 0;
  const total = loaded ? usagePct(usage!.total_bytes, limit) : 0;
  const level = levelClass(total);

  const cancelClose = () => {
    if (closeTimer.current) {
      clearTimeout(closeTimer.current);
      closeTimer.current = null;
    }
  };
  const handleEnter = () => {
    cancelClose();
    setOpen(true);
  };
  const handleLeave = () => {
    cancelClose();
    closeTimer.current = setTimeout(() => setOpen(false), 200);
  };
  // 点击亦可切换展开，作为触屏设备无 hover 的兜底交互
  const handleClick = () => {
    if (open) handleLeave();
    else handleEnter();
  };

  const rows = usage
    ? [
        { label: "系统提示词", bytes: usage.system_prompt_bytes, dot: "ctx-meter__dot--prompt" },
        { label: "系统工具", bytes: usage.tools_bytes, dot: "ctx-meter__dot--tools" },
        { label: `Skill（${usage.skills_count} 个）`, bytes: usage.skills_bytes, dot: "ctx-meter__dot--skills" },
        { label: "消息", bytes: usage.messages_bytes, dot: "ctx-meter__dot--messages" },
      ]
    : [];

  return (
    <div class="ctx-meter" onMouseEnter={handleEnter} onMouseLeave={handleLeave}>
      {open && (
        <div class="ctx-meter__panel">
          <div class="ctx-meter__head">
            <span class="ctx-meter__title">上下文窗口</span>
            <span class={`ctx-meter__pct ${level}`}>{loaded ? `${total}%` : "--"}</span>
          </div>
          <p class="ctx-meter__desc">展示当前任务的上下文占用情况；压缩会摘要早期内容，需等待片刻。</p>
          <div class="ctx-meter__track">
            <div
              class={`ctx-meter__fill ${level}`}
              style={{ width: `${Math.min(total, 100)}%` }}
            />
          </div>
          <ul class="ctx-meter__rows">
            {rows.map((r) => (
              <li class="ctx-meter__row" key={r.label}>
                <span class={`ctx-meter__dot ${r.dot}`} />
                <span class="ctx-meter__label">{r.label}</span>
                <span class="ctx-meter__row-pct">{loaded ? `${usagePct(r.bytes, limit)}%` : "--"}</span>
              </li>
            ))}
          </ul>
          <button class="ctx-meter__compact" onClick={onCompact}>
            <span class="ctx-meter__compact-icon">🗜</span>
            压缩上下文
          </button>
        </div>
      )}
      <button
        class="ctx-meter__trigger"
        onClick={handleClick}
        title="上下文窗口占用"
        aria-label="上下文窗口占用"
      >
        <span class="ctx-meter__mini-track">
          <span
            class={`ctx-meter__mini-fill ${level}`}
            style={{ width: `${loaded ? Math.min(total, 100) : 0}%` }}
          />
        </span>
        <span class={`ctx-meter__value ${level}`}>{loaded ? `${total}%` : "--"}</span>
      </button>
    </div>
  );
}
