import type { AppState } from "../types";
import { workDirBaseName } from "../lib/labels";

interface Props {
  status: AppState;
  // workDir 为当前工作目录绝对路径，用于顶部面包屑展示
  workDir: string;
}

// StatusBar —— 主区域顶部的纤细状态条。
// 左侧展示当前工作目录面包屑，右侧展示连接状态与对话轮次。
export function StatusBar({ status, workDir }: Props) {
  return (
    <header class="topbar">
      <div class="topbar__left">
        <span class="topbar__crumb-icon">📁</span>
        <span class="topbar__crumb" title={workDir || "未设置工作目录"}>
          {workDirBaseName(workDir)}
        </span>
        {status.busy && <span class="topbar__busy">处理中…</span>}
      </div>
      <div class="topbar__right">
        <span class={`conn-dot ${status.connected ? "connected" : "disconnected"}`} />
        <span class="topbar__conn">{status.connected ? "已连接" : "未连接"}</span>
        {status.turnCount > 0 && (
          <span class="turn-badge">第 {status.turnCount} 轮</span>
        )}
      </div>
    </header>
  );
}
