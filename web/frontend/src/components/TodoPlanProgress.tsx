import { useRef, useState } from "preact/hooks";
import type { PlanItem, TodoPlan } from "../types";
import { IconCheck, IconLayers } from "../lib/icons";

// Props —— 当前任务计划与收起回调用途的占位（保留扩展位）
interface Props {
  plan: TodoPlan | null;
}

// nextProgressLabel 返回进行中条目的进度文案；无进行中条目时返回空串
function currentStep(items: PlanItem[]): string {
  const running = items.find((it) => it.status === "in_progress");
  return running?.progressLabel || "";
}

// completedCount 统计已完成的步骤数量
function completedCount(items: PlanItem[]): number {
  return items.filter((it) => it.status === "completed").length;
}

// TodoPlanProgress —— 对话框顶部居中的任务计划进度条。
// 模型通过 todo tool 生成计划后，顶部显示"已进行/总数"（如 3/4）的细长条；
// 悬停展开全部步骤列表，完成某一步后由后端推送最新计划实时更新。
// 未调用 todo tool 时 plan 为 null，组件不渲染任何内容。
export function TodoPlanProgress({ plan }: Props) {
  const [open, setOpen] = useState(false);
  // closeTimer 延迟关闭：进入面板时取消，避免指针移到面板内时闪闭
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  if (!plan) return null;

  const items = plan.items;
  const total = items.length;
  const done = completedCount(items);
  const progressLabel = currentStep(items);

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
  // 无 hover 的触屏设备以点击切换展开
  const handleClick = () => {
    if (open) handleLeave();
    else handleEnter();
  };

  return (
    <div
      class="todo-plan"
      onMouseEnter={handleEnter}
      onMouseLeave={handleLeave}
      onClick={handleClick}
      aria-expanded={open}
    >
      {open && (
        <div class="todo-plan__panel">
          <div class="todo-plan__head">
            <span class="todo-plan__title">
              <IconLayers size={13} />
              当前计划
            </span>
            <span class="todo-plan__count">
              {done}/{total} 已完成
            </span>
          </div>
          <ul class="todo-plan__items">
            {items.map((it) => {
              const running = it.status === "in_progress";
              return (
                <li
                  class={`todo-plan__item todo-plan__item--${it.status}`}
                  key={it.id}
                >
                  <span class="todo-plan__mark" aria-hidden="true">
                    {it.status === "completed" ? <IconCheck size={11} /> : running ? "→" : "•"}
                  </span>
                  <span class="todo-plan__content">
                    {it.content}
                    {running && it.progressLabel ? (
                      <em class="todo-plan__progress">（{it.progressLabel}）</em>
                    ) : null}
                  </span>
                </li>
              );
            })}
          </ul>
        </div>
      )}
      <button
        class="todo-plan__trigger"
        title={progressLabel || `计划进度 ${done}/${total}`}
        aria-label="当前任务计划"
      >
        <span class="todo-plan__pct">{done}/{total}</span>
        <span class="todo-plan__tip">{progressLabel || "计划进行中"}</span>
      </button>
    </div>
  );
}
