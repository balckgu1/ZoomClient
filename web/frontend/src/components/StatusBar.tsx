import type { AppState } from "../types";

interface Props {
  status: AppState;
}

export function StatusBar({ status }: Props) {
  return (
    <header class="status-bar">
      <div class="status-left">
        <span class="logo">⚡</span>
        <span class="title">ZoomClient</span>
        <span class="model-badge">{status.model || "connecting..."}</span>
      </div>
      <div class="status-right">
        <span class={`conn-dot ${status.connected ? "connected" : "disconnected"}`} />
        <span class="status-text">{status.connected ? "Connected" : "Disconnected"}</span>
        {status.turnCount > 0 && (
          <span class="turn-badge">Turn {status.turnCount}</span>
        )}
      </div>
    </header>
  );
}
