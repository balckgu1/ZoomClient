import { useState, useEffect } from "preact/hooks";

// WorkDirPanel 的 props
interface Props {
  // current 为后端返回的当前工作目录绝对路径
  current: string;
  // onSave 提交新目录；后端串行处理，真正结果通过 SSE 反馈，故此处只需发起请求
  onSave: (path: string) => void;
  onClose: () => void;
}

// WorkDirPanel —— 工作目录切换面板（模态）。
// 展示当前目录、提供输入框修改，并给出异步生效的提示。
export function WorkDirPanel({ current, onSave, onClose }: Props) {
  const [path, setPath] = useState(current);
  const [error, setError] = useState("");

  // 当外部 current 变化（如切换成功后的 SSE 回推）时同步输入框
  useEffect(() => {
    setPath(current);
  }, [current]);

  // handleSave 做基本校验后提交，空路径与未变更不发起请求
  const handleSave = () => {
    const trimmed = path.trim();
    if (!trimmed) {
      setError("路径不能为空");
      return;
    }
    if (trimmed === current) {
      onClose();
      return;
    }
    setError("");
    onSave(trimmed);
    onClose();
  };

  // 支持 Ctrl/⌘+Enter 快速提交、Esc 关闭
  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) handleSave();
    if (e.key === "Escape") onClose();
  };

  return (
    <div class="modal-overlay" onClick={onClose}>
      <div class="panel-dialog" onClick={(e) => e.stopPropagation()} onKeyDown={handleKeyDown}>
        <div class="panel-dialog__header">
          <h3>切换工作目录</h3>
          <button class="panel-dialog__close" onClick={onClose} title="关闭">&times;</button>
        </div>

        <div class="panel-dialog__body">
          <div class="field">
            <span class="field__label">当前目录</span>
            <code class="field__current" title={current}>{current || "未设置"}</code>
          </div>

          <label class="field">
            <span class="field__label">新目录（绝对路径）</span>
            <input
              class="field__input"
              value={path}
              onInput={(e) => setPath((e.target as HTMLInputElement).value)}
              placeholder="例如 D:\Projects\my-app 或 /home/user/project"
              autoFocus
            />
          </label>

          {error && <div class="field__error">{error}</div>}

          <p class="field__hint">
            工作目录是工具读写的沙箱根路径。切换请求会串行下发给后端处理，
            成功后顶部会收到提示，请留意路径是否存在且为目录。
          </p>
        </div>

        <div class="panel-dialog__footer">
          <button class="btn btn--ghost" onClick={onClose}>取消</button>
          <button class="btn btn--primary" onClick={handleSave} disabled={!path.trim()}>
            应用
          </button>
        </div>
      </div>
    </div>
  );
}
