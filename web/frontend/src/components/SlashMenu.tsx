// src/components/SlashMenu.tsx
//
// 输入框 "/" 扩展框：在输入板上方浮出，分区列出内置命令与全部已加载 skill。
//
// 本组件只负责呈现；键盘导航状态由 Composer 持有，因为焦点始终在 textarea 上，
// 由它统一处理 ↑↓ / Tab / Enter / Esc 才不会与输入行为打架。
import { useEffect, useRef } from "preact/hooks";
import type { SlashItem } from "../lib/slash";
import { IconBook, IconTerminal } from "../lib/icons";

interface Props {
  /** 已按查询串过滤并排序的条目 */
  items: SlashItem[];
  /** 当前高亮条目的下标 */
  activeIndex: number;
  /** 技能总数，用于分区标题上的计数 */
  skillTotal: number;
  onPick: (item: SlashItem) => void;
  onHover: (index: number) => void;
}

export function SlashMenu({ items, activeIndex, skillTotal, onPick, onHover }: Props) {
  const listRef = useRef<HTMLDivElement>(null);

  // 键盘导航时把高亮项滚入可视区：菜单可滚动，否则连续按 ↓ 会选中看不见的条目。
  useEffect(() => {
    const list = listRef.current;
    if (!list) return;
    const node = list.querySelector<HTMLElement>(`[data-idx="${activeIndex}"]`);
    node?.scrollIntoView({ block: "nearest" });
  }, [activeIndex]);

  return (
    <div class="slash" id="slash-menu" role="listbox" aria-label="命令与技能">
      <div class="slash__list" ref={listRef}>
        {items.length === 0 && (
          <div class="slash__empty">
            没有匹配的命令或技能
          </div>
        )}

        {items.map((item, i) => {
          // 只在 kind 发生切换处插入分区标题：过滤后的列表也能正确分组
          const prevKind = i > 0 ? items[i - 1].kind : null;
          const showHeader = item.kind !== prevKind;
          const header =
            item.kind === "command" ? "命令" : `技能 · ${skillTotal}`;

          return (
            <div key={`${item.kind}:${item.name}`}>
              {showHeader && <div class="slash__group">{header}</div>}
              <div
                data-idx={i}
                id={`slash-opt-${i}`}
                role="option"
                aria-selected={i === activeIndex}
                class={`slash__item slash__item--${item.kind} ${i === activeIndex ? "is-active" : ""}`}
                onMouseEnter={() => onHover(i)}
                // 用 onMouseDown 而非 onClick：textarea 的 blur 会先触发并关闭菜单，
                // 抢在 blur 之前处理点击才能可靠选中。
                onMouseDown={(e) => {
                  e.preventDefault();
                  onPick(item);
                }}
              >
                <span class="slash__icon">
                  {item.kind === "command" ? <IconTerminal size={14} /> : <IconBook size={14} />}
                </span>
                <span class="slash__name">/{item.name}</span>
                <span class="slash__hint">{item.hint}</span>
                {item.meta && <span class="slash__meta">{item.meta}</span>}
              </div>
            </div>
          );
        })}
      </div>

      <div class="slash__footer">
        <span class="slash__key">↑↓</span> 选择
        <span class="slash__key">⏎</span> 确认
        <span class="slash__key">esc</span> 关闭
        <span class="slash__footer-note">选中技能后可继续输入你的任务</span>
      </div>
    </div>
  );
}
