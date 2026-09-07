import { useState } from "preact/hooks";
import type { PermissionAsk } from "../types";

interface Props {
  permission: PermissionAsk;
  onResolve: (allow: boolean, reason: string) => void;
}

export function PermissionDialog({ permission, onResolve }: Props) {
  const [reason, setReason] = useState("");

  const handleAllow = () => onResolve(true, "");
  const handleDeny = () => onResolve(false, reason || "用户拒绝");

  return (
    <div class="modal-overlay">
      <div class="permission-dialog">
        <h3>⚠️ 需要授权</h3>
        <div class="perm-field">
          <span class="perm-label">工具：</span>
          <code>{permission.tool}</code>
        </div>
        {permission.args && (
          <div class="perm-field">
            <span class="perm-label">参数：</span>
            <pre class="perm-args">{permission.args}</pre>
          </div>
        )}
        <div class="perm-field">
          <span class="perm-label">原因：</span>
          <span>{permission.reason}</span>
        </div>
        <input
          class="perm-reason-input"
          placeholder="拒绝原因（可选）"
          value={reason}
          onInput={(e) => setReason((e.target as HTMLInputElement).value)}
        />
        <div class="perm-actions">
          <button class="btn-allow" onClick={handleAllow}>允许</button>
          <button class="btn-deny" onClick={handleDeny}>拒绝</button>
        </div>
      </div>
    </div>
  );
}
