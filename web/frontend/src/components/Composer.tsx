import { useState, useRef } from "preact/hooks";
import type { JSX } from "preact";
import type { ModelPreset, PermissionMode } from "../types";
import { ModelSelector } from "./ModelSelector";
import { permissionModeMeta, workDirBaseName } from "../lib/labels";

// Composer 的 props —— 聚合输入框与上下文控制 chip
interface Props {
  disabled: boolean;
  busy: boolean;
  workDir: string;
  permissionMode: PermissionMode;
  models: ModelPreset[];
  activeModel: string;
  onSend: (message: string) => void;
  onSlashCommand: (cmd: string) => void;
  onStop: () => void;
  onNewSession: () => void;
  onOpenWorkDir: () => void;
  onOpenPermission: () => void;
  onSelectModel: (name: string) => void;
  onAddModel: (preset: ModelPreset) => void;
  onEditModel: (name: string, preset: ModelPreset) => void;
}

// Composer —— 底部输入区。
// 结构参考设计图：上方是上下文 chip 行（工作目录 / 权限模式 / 模型），
// 中间是多行输入框，下方是操作行（新建会话 + 发送/停止）。
export function Composer({
  disabled, busy, workDir, permissionMode, models, activeModel,
  onSend, onSlashCommand, onStop, onNewSession,
  onOpenWorkDir, onOpenPermission, onSelectModel, onAddModel, onEditModel,
}: Props) {
  const [text, setText] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const permMeta = permissionModeMeta(permissionMode);

  // handleSubmit 区分斜杠命令与普通消息，提交后清空并复位输入框高度
  const handleSubmit = () => {
    const trimmed = text.trim();
    if (!trimmed || disabled) return;
    if (trimmed.startsWith("/")) onSlashCommand(trimmed);
    else onSend(trimmed);
    setText("");
    if (textareaRef.current) textareaRef.current.style.height = "auto";
  };

  // Enter 发送、Shift+Enter 换行
  const handleKeyDown = (e: JSX.TargetedKeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  };

  // handleInput 让输入框随内容自动增高（上限由 CSS max-height 控制）
  const handleInput = () => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = el.scrollHeight + "px";
  };

  return (
    <footer class="composer">
      {/* ── 上下文 chip 行 ── */}
      <div class="composer__chips">
        <button class="chip chip--workdir" onClick={onOpenWorkDir} title={workDir || "未设置工作目录"}>
          <span class="chip__icon">📁</span>
          <span class="chip__label">{workDirBaseName(workDir)}</span>
        </button>

        <button
          class={`chip chip--permission tone-${permMeta.tone}`}
          onClick={onOpenPermission}
          title={permMeta.hint}
        >
          <span class="chip__icon">🛡</span>
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

      {/* ── 输入框 ── */}
      <div class="composer__input-wrap">
        <textarea
          ref={textareaRef}
          class="composer__textarea"
          placeholder={busy ? "智能体正在思考…" : "随心输入（/ 使用命令，Enter 发送，Shift+Enter 换行）"}
          value={text}
          onInput={(e) => { setText((e.target as HTMLTextAreaElement).value); handleInput(); }}
          onKeyDown={handleKeyDown}
          disabled={disabled}
          rows={1}
        />
      </div>

      {/* ── 操作行 ── */}
      <div class="composer__actions">
        <button class="composer__plus" onClick={onNewSession} title="新建会话">＋</button>
        <div class="composer__actions-right">
          <button
            class={`composer__send ${busy ? "composer__send--stop" : ""}`}
            onClick={busy ? onStop : handleSubmit}
            disabled={!busy && (!text.trim() || disabled)}
            title={busy ? "停止生成" : "发送"}
          >
            {busy ? "⏹" : "↑"}
          </button>
        </div>
      </div>
    </footer>
  );
}
