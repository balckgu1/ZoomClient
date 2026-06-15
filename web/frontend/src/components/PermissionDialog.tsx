import { useState } from "preact/hooks";
import type { PermissionAsk } from "../types";

interface Props {
  permission: PermissionAsk;
  onResolve: (allow: boolean, reason: string) => void;
}

export function PermissionDialog({ permission, onResolve }: Props) {
  const [reason, setReason] = useState("");

  const handleAllow = () => onResolve(true, "");
  const handleDeny = () => onResolve(false, reason || "denied by user");

  return (
    <div class="modal-overlay">
      <div class="permission-dialog">
        <h3>⚠️ Permission Required</h3>
        <div class="perm-field">
          <span class="perm-label">Tool:</span>
          <code>{permission.tool}</code>
        </div>
        {permission.args && (
          <div class="perm-field">
            <span class="perm-label">Args:</span>
            <pre class="perm-args">{permission.args}</pre>
          </div>
        )}
        <div class="perm-field">
          <span class="perm-label">Reason:</span>
          <span>{permission.reason}</span>
        </div>
        <input
          class="perm-reason-input"
          placeholder="Denial reason (optional)"
          value={reason}
          onInput={(e) => setReason((e.target as HTMLInputElement).value)}
        />
        <div class="perm-actions">
          <button class="btn-allow" onClick={handleAllow}>Allow</button>
          <button class="btn-deny" onClick={handleDeny}>Deny</button>
        </div>
      </div>
    </div>
  );
}
