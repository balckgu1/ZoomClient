import { useState } from "preact/hooks";
import type { PermissionAsk } from "../types";
import { IconShield, IconCheck, IconClose } from "../lib/icons";

interface Props {
  permission: PermissionAsk;
  onResolve: (allow: boolean, reason: string) => void;
}

// PermissionDialog —— 需要用户授权的模态。
// 这是界面里唯一打断流程的控件，因此给它最完整的视觉重量：
// 黄色盾形标记 + 允许=绿 / 拒绝=红的双按钮，与权限模式的色彩语义一致。
export function PermissionDialog({ permission, onResolve }: Props) {
  const [reason, setReason] = useState("");

  const handleAllow = () => onResolve(true, "");
  const handleDeny = () => onResolve(false, reason || "用户拒绝");

  return (
    <div class="modal-overlay">
      <div class="permission-dialog">
        <div class="permission-dialog__head">
          <span class="permission-dialog__mark"><IconShield size={16} /></span>
          <div>
            <h3>需要授权</h3>
            <p class="permission-dialog__sub">智能体请求执行以下操作，请你决定是否放行</p>
          </div>
        </div>

        <div class="permission-dialog__body">
          <div class="perm-field">
            <span class="perm-label">工具</span>
            <code class="perm-value">{permission.tool}</code>
          </div>
          {permission.args && (
            <div class="perm-field perm-field--block">
              <span class="perm-label">参数</span>
              <pre class="perm-args">{permission.args}</pre>
            </div>
          )}
          <div class="perm-field perm-field--block">
            <span class="perm-label">原因</span>
            <span class="perm-value">{permission.reason}</span>
          </div>
          <input
            class="field__input perm-reason-input"
            placeholder="拒绝原因（可选）"
            value={reason}
            onInput={(e) => setReason((e.target as HTMLInputElement).value)}
          />
        </div>

        <div class="perm-actions">
          <button class="btn btn--ghost" onClick={handleDeny}>
            <IconClose size={14} /> 拒绝
          </button>
          <button class="btn btn-allow" onClick={handleAllow}>
            <IconCheck size={14} /> 允许
          </button>
        </div>
      </div>
    </div>
  );
}
