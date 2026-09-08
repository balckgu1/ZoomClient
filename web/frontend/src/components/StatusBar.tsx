import type { AppState } from "../types";
import { workDirBaseName } from "../lib/labels";
import { IconFolder } from "../lib/icons";

interface Props {
  status: AppState;
  // workDir 为当前工作目录绝对路径，用于顶部面包屑展示
  workDir: string;
}

// StatusBar —— 主区域顶部的状态条。
// 左侧是当前工作目录面包屑（带一盏"灯丝"指示智能体是否在运行），
// 右侧展示连接状态与对话轮次。
export function StatusBar({ status, workDir }: Props) {
  return (
    <header class={`topbar ${status.busy ? "is-live" : ""}`}>
      <div class="topbar__left">
        <span class="eyebrow">workspace</span>
        <span class="topbar__crumb" title={workDir || "未设置工作目录"}>
          <IconFolder size={13} />
          <span class="topbar__crumb-name">{workDirBaseName(workDir)}</span>
        </span>
        <span class="topbar__crumb-path" title={workDir}>{workDir || "未设置"}</span>
      </div>
      <div class="topbar__right">
        <span class="filament" aria-hidden="true" />
        <span class="topbar__turn">
          <span class="eyebrow">turn</span> {status.turnCount}
        </span>
        <span class={`topbar__conn ${status.connected ? "is-on" : "is-off"}`}>
          <span class="led led--on" aria-hidden="true" />
          {status.connected ? "已连接" : "未连接"}
        </span>
      </div>
    </header>
  );
}
