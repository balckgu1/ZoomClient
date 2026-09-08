import { useState, useRef, useCallback } from "preact/hooks";
import type { JSX } from "preact";
import type { ContextUsage, ModelPreset, PermissionMode, SkillMeta } from "../types";
import { ModelSelector } from "./ModelSelector";
import { ContextUsageMeter } from "./ContextUsageMeter";
import { SlashMenu } from "./SlashMenu";
import { permissionModeMeta, workDirBaseName } from "../lib/labels";
import { buildSlashItems, filterSlashItems } from "../lib/slash";
import type { SlashItem } from "../lib/slash";
import { IconArrowUp, IconFolder, IconShield, IconStop } from "../lib/icons";

// Composer 的 props —— 聚合输入板与其上的上下文控制 chip
interface Props {
  disabled: boolean;
  busy: boolean;
  workDir: string;
  permissionMode: PermissionMode;
  models: ModelPreset[];
  activeModel: string;
  contextUsage: ContextUsage | null;
  /** 后端已加载的技能目录，驱动 "/" 扩展框的技能分区 */
  skills: SkillMeta[];
  onSend: (message: string) => void;
  onSlashCommand: (cmd: string) => void;
  onStop: () => void;
  onOpenWorkDir: () => void;
  onOpenPermission: () => void;
  onSelectModel: (name: string) => void;
  onAddModel: (preset: ModelPreset) => void;
  onEditModel: (name: string, preset: ModelPreset) => void;
  onCompact: () => void;
}

// Composer —— 底部输入区，一块"控制台输入板"。
//
// 板上是自由输入，板内底栏承载上下文 chip（工作目录 / 权限模式 / 模型）与发送键，
// 让"我要说什么"和"这句话会在什么条件下执行"落在同一个视觉容器里。
// 键入 "/" 时扩展框从板的上沿浮出，与板共用同一套圆角与发丝线，读作板的延伸。
export function Composer({
  disabled, busy, workDir, permissionMode, models, activeModel, contextUsage, skills,
  onSend, onSlashCommand, onStop,
  onOpenWorkDir, onOpenPermission, onSelectModel, onAddModel, onEditModel,
  onCompact,
}: Props) {
  const [text, setText] = useState("");
  const [focused, setFocused] = useState(false);
  // activeIdx 为扩展框当前高亮项；escDismissed 记录用户是否主动按 Esc 关掉了扩展框
  const [activeIdx, setActiveIdx] = useState(0);
  const [escDismissed, setEscDismissed] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const permMeta = permissionModeMeta(permissionMode);
  const allItems = buildSlashItems(skills);

  // ── 扩展框可见性判定 ──
  // 只在"整段输入就是一个尚未写完的斜杠词"时展开：一旦敲入空格说明用户已在描述任务，
  // 此时再弹菜单只会遮挡输入。
  const query = text.startsWith("/") ? text.slice(1) : "";
  const slashTyping = text.startsWith("/") && !/\s/.test(text) && !escDismissed;
  const items = slashTyping ? filterSlashItems(allItems, query) : [];
  const menuOpen = focused && slashTyping;

  // resize 让输入板随内容自动增高（上限由 CSS max-height 控制）
  const resize = useCallback(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${el.scrollHeight}px`;
  }, []);

  // handleSubmit 区分斜杠命令与普通消息，提交后清空并复位输入板高度
  const handleSubmit = useCallback(() => {
    const trimmed = text.trim();
    if (!trimmed || disabled) return;
    if (trimmed.startsWith("/")) onSlashCommand(trimmed);
    else onSend(trimmed);
    setText("");
    setActiveIdx(0);
    setEscDismissed(false);
    if (textareaRef.current) textareaRef.current.style.height = "auto";
  }, [text, disabled, onSend, onSlashCommand]);

  // pickItem 处理扩展框选中项。
  //
  // 命令直接执行；技能则回填 "/name " 到输入板并把光标停在空格后，
  // 因为选中技能只是选定了"用哪本手册"，用户还需要说清要用它做什么。
  const pickItem = useCallback((item: SlashItem) => {
    if (item.kind === "command") {
      setText("");
      setEscDismissed(false);
      if (textareaRef.current) textareaRef.current.style.height = "auto";
      onSlashCommand(`/${item.name}`);
      return;
    }
    setText(`/${item.name} `);
    setActiveIdx(0);
    // 回填后需要重新量一次高度，并把光标移到末尾
    requestAnimationFrame(() => {
      const el = textareaRef.current;
      if (!el) return;
      resize();
      el.focus();
      const pos = el.value.length;
      el.setSelectionRange(pos, pos);
    });
  }, [onSlashCommand, resize]);

  // stepActive 在扩展框内循环移动高亮项
  const stepActive = (delta: number) => {
    if (items.length === 0) return;
    setActiveIdx((i) => (i + delta + items.length) % items.length);
  };

  // handleKeyDown 扩展框展开时接管导航键，收起时只处理 Enter 发送。
  const handleKeyDown = (e: JSX.TargetedKeyboardEvent<HTMLTextAreaElement>) => {
    if (menuOpen && items.length > 0) {
      if (e.key === "ArrowDown") { e.preventDefault(); stepActive(1); return; }
      if (e.key === "ArrowUp") { e.preventDefault(); stepActive(-1); return; }
      if (e.key === "Tab") { e.preventDefault(); stepActive(e.shiftKey ? -1 : 1); return; }
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        const item = items[Math.min(activeIdx, items.length - 1)];
        if (item) pickItem(item);
        return;
      }
    }
    if (e.key === "Escape" && menuOpen) {
      e.preventDefault();
      setEscDismissed(true);
      return;
    }
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  };

  // handleInput 同步文本、复位高度与高亮项。
  // 重新键入单个 "/" 视为一次全新的唤出意图，需要清掉 Esc 造成的关闭状态。
  const handleInput = (e: JSX.TargetedInputEvent<HTMLTextAreaElement>) => {
    const value = (e.target as HTMLTextAreaElement).value;
    setText(value);
    setActiveIdx(0);
    if (value === "/" || !value.startsWith("/")) setEscDismissed(false);
    resize();
  };

  return (
    <footer class="composer">
      <div class="composer__inner">
        {/* ── "/" 扩展框：从输入板上沿浮出 ── */}
        {menuOpen && (
          <SlashMenu
            items={items}
            activeIndex={Math.min(activeIdx, Math.max(items.length - 1, 0))}
            skillTotal={skills.length}
            onPick={pickItem}
            onHover={setActiveIdx}
          />
        )}

        {/* ── 输入板 ── */}
        <div class={`plate ${focused ? "is-focused" : ""} ${busy ? "is-busy" : ""}`}>
          <textarea
            ref={textareaRef}
            class="plate__input"
            placeholder={busy ? "智能体正在工作，可以先把下一步想清楚…" : "说说你想让它做什么"}
            value={text}
            onInput={handleInput}
            onKeyDown={handleKeyDown}
            onFocus={() => setFocused(true)}
            onBlur={() => setFocused(false)}
            disabled={disabled}
            rows={1}
            aria-label="消息输入框"
            aria-expanded={menuOpen}
            aria-controls={menuOpen ? "slash-menu" : undefined}
            aria-autocomplete="list"
          />

          {/* 板内底栏：上下文 chip + 上下文占用 + 发送 */}
          <div class="plate__bar">
            <div class="plate__chips">
              <button
                class="chip"
                onClick={onOpenWorkDir}
                title={workDir || "未设置工作目录"}
              >
                <IconFolder size={13} />
                <span class="chip__label">{workDirBaseName(workDir)}</span>
              </button>

              <button
                class={`chip chip--tone-${permMeta.tone}`}
                onClick={onOpenPermission}
                title={permMeta.hint}
              >
                <IconShield size={13} />
                <span class="chip__label">{permMeta.label}</span>
              </button>

              <ModelSelector
                models={models}
                active={activeModel}
                onSelect={onSelectModel}
                onAdd={onAddModel}
                onEdit={onEditModel}
                disabled={busy}
              />
            </div>

            <div class="plate__right">
              <ContextUsageMeter usage={contextUsage} onCompact={onCompact} />
              <button
                class={`plate__send ${busy ? "plate__send--stop" : ""}`}
                onClick={busy ? onStop : handleSubmit}
                disabled={!busy && (!text.trim() || disabled)}
                title={busy ? "停止生成" : "发送（Enter）"}
                aria-label={busy ? "停止生成" : "发送"}
              >
                {busy ? <IconStop size={15} /> : <IconArrowUp size={16} />}
              </button>
            </div>
          </div>
        </div>

        {/* ── 板下提示行：教会用户 "/" 这个新入口 ── */}
        <div class="composer__hint">
          <span>
            键入 <kbd class="kbd">/</kbd> 调出命令与技能（{skills.length} 个）
          </span>
          <span class="composer__hint-sep">·</span>
          <span><kbd class="kbd">Enter</kbd> 发送</span>
          <span class="composer__hint-sep">·</span>
          <span><kbd class="kbd">Shift</kbd>+<kbd class="kbd">Enter</kbd> 换行</span>
        </div>
      </div>
    </footer>
  );
}
