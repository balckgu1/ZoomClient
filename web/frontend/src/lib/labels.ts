import type { PermissionMode } from "../types";

// 权限模式的展示元数据：中文标签、说明与主题色调。
// 供 composer 徽章与权限管理面板复用，避免多处硬编码造成不一致。
export interface PermissionModeMeta {
  value: PermissionMode;
  label: string;
  hint: string;
  // tone 决定徽章配色：warn=橙色（自动放行）、neutral=中性（默认询问）、safe=绿色（只读）
  tone: "warn" | "neutral" | "safe";
}

// 按"从宽松到严格"排序，方便用户在面板中理解差异
export const PERMISSION_MODES: PermissionModeMeta[] = [
  {
    value: "auto",
    label: "自动放行",
    hint: "只读工具自动执行，写入 / 执行类工具仍会询问你",
    tone: "warn",
  },
  {
    value: "default",
    label: "默认询问",
    hint: "未命中任何规则的操作都会先询问你",
    tone: "neutral",
  },
  {
    value: "plan",
    label: "计划只读",
    hint: "禁止一切写入与执行类工具，仅可读取和分析",
    tone: "safe",
  },
];

// permissionModeMeta 返回指定模式的元数据，未知模式回退到 default。
export function permissionModeMeta(mode: PermissionMode): PermissionModeMeta {
  return (
    PERMISSION_MODES.find((m) => m.value === mode) ??
    (PERMISSION_MODES[1] as PermissionModeMeta)
  );
}

// workDirBaseName 从完整路径中截取最后一段目录名，用于 chip 的简短展示。
// 同时兼容 Windows（\）与 POSIX（/）分隔符。
export function workDirBaseName(path: string): string {
  if (!path) return "未设置";
  const normalized = path.replace(/[\\/]+$/, "");
  const idx = Math.max(normalized.lastIndexOf("/"), normalized.lastIndexOf("\\"));
  return idx >= 0 ? normalized.slice(idx + 1) : normalized;
}
