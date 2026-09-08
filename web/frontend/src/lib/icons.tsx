// src/lib/icons.tsx
//
// 全站图标集：统一 24×24 视图框、1.6px 描边、currentColor 取色的线性图标。
//
// 之所以自建而不用 emoji：emoji 在不同平台字形差异极大、无法随语义色变化，
// 且会把"仪表盘"气质拉回消费级聊天应用。线性图标只描轮廓、由父级文本色决定颜色，
// 因此同一枚图标放在工具卡片里是蓝的、放在错误提示里就是红的，无需维护多套资源。
import type { JSX } from "preact";

interface IconProps extends JSX.SVGAttributes<SVGSVGElement> {
  /** 渲染尺寸（px），默认 16，与 14px 正文行高协调 */
  size?: number | string;
}

// Icon 公共外壳：只保留描边，颜色继承当前文本色。
function Icon({ size = 16, children, ...rest }: IconProps) {
  return (
    <svg
      class={`icon ${rest.class ?? ""}`.trim()}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.6"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
      focusable="false"
      {...rest}
    >
      {children}
    </svg>
  );
}

// ─── 品牌与通用 ───

/** 品牌标记：一道折线闪电，取自 ZoomClient 的"即时驱动"含义 */
export const IconBolt = (p: IconProps) => (
  <Icon {...p}>
    <path d="M13 2 4.5 13.5H11l-1 8.5 8.5-11.5H12z" />
  </Icon>
);

export const IconPlus = (p: IconProps) => (
  <Icon {...p}>
    <path d="M12 5v14M5 12h14" />
  </Icon>
);

export const IconClose = (p: IconProps) => (
  <Icon {...p}>
    <path d="M18 6 6 18M6 6l12 12" />
  </Icon>
);

export const IconCheck = (p: IconProps) => (
  <Icon {...p}>
    <path d="m20 6-11 11-5-5" />
  </Icon>
);

export const IconChevronDown = (p: IconProps) => (
  <Icon {...p}>
    <path d="m6 9 6 6 6-6" />
  </Icon>
);

export const IconChevronUp = (p: IconProps) => (
  <Icon {...p}>
    <path d="m18 15-6-6-6 6" />
  </Icon>
);

export const IconChevronRight = (p: IconProps) => (
  <Icon {...p}>
    <path d="m9 18 6-6-6-6" />
  </Icon>
);

export const IconArrowUp = (p: IconProps) => (
  <Icon {...p}>
    <path d="M12 20V4M5 11l7-7 7 7" />
  </Icon>
);

export const IconCornerEnter = (p: IconProps) => (
  <Icon {...p}>
    <path d="m9 10-5 5 5 5" />
    <path d="M20 4v7a4 4 0 0 1-4 4H4" />
  </Icon>
);

// ─── 上下文与位置 ───

export const IconFolder = (p: IconProps) => (
  <Icon {...p}>
    <path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.7-.9L9.6 3.9A2 2 0 0 0 7.9 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
  </Icon>
);

export const IconShield = (p: IconProps) => (
  <Icon {...p}>
    <path d="M12 22s8-3.5 8-10V5.5L12 3 4 5.5V12c0 6.5 8 10 8 10z" />
  </Icon>
);

export const IconGauge = (p: IconProps) => (
  <Icon {...p}>
    <path d="m12 14 4-4" />
    <path d="M3.3 19a10 10 0 1 1 17.4 0" />
  </Icon>
);

export const IconCompress = (p: IconProps) => (
  <Icon {...p}>
    <path d="M8 3v3a2 2 0 0 1-2 2H3M21 8h-3a2 2 0 0 1-2-2V3M3 16h3a2 2 0 0 1 2 2v3M16 21v-3a2 2 0 0 1 2-2h3" />
  </Icon>
);

export const IconCpu = (p: IconProps) => (
  <Icon {...p}>
    <rect x="5" y="5" width="14" height="14" rx="2" />
    <rect x="9.5" y="9.5" width="5" height="5" rx="1" />
    <path d="M9 2v3M15 2v3M9 19v3M15 19v3M2 9h3M2 15h3M19 9h3M19 15h3" />
  </Icon>
);

// ─── 会话与消息 ───

export const IconLayers = (p: IconProps) => (
  <Icon {...p}>
    <path d="m12 2 9 5-9 5-9-5z" />
    <path d="m3 12 9 5 9-5" />
    <path d="m3 17 9 5 9-5" />
  </Icon>
);

export const IconTrash = (p: IconProps) => (
  <Icon {...p}>
    <path d="M3 6h18M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2" />
    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6M10 11v6M14 11v6" />
  </Icon>
);

export const IconPencil = (p: IconProps) => (
  <Icon {...p}>
    <path d="M17 3a2.8 2.8 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5z" />
  </Icon>
);

// ─── 智能体活动 ───

/** 工具调用：扳手，代表"智能体动手做事" */
export const IconWrench = (p: IconProps) => (
  <Icon {...p}>
    <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.8-3.8a6 6 0 0 1-7.9 7.9l-6.9 6.9a2.1 2.1 0 0 1-3-3l6.9-6.9a6 6 0 0 1 7.9-7.9z" />
  </Icon>
);

/** 推理过程：一段脉冲波形，表示"内部正在推演" */
export const IconPulse = (p: IconProps) => (
  <Icon {...p}>
    <path d="M2 12h4l3 8 6-16 3 8h4" />
  </Icon>
);

/** 技能：翻开的书，代表"可复用的操作手册" */
export const IconBook = (p: IconProps) => (
  <Icon {...p}>
    <path d="M12 7v14" />
    <path d="M3 18a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h5a4 4 0 0 1 4 4 4 4 0 0 1 4-4h5a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1h-6a3 3 0 0 0-3 3 3 3 0 0 0-3-3z" />
  </Icon>
);

/** 斜杠命令：终端提示符 */
export const IconTerminal = (p: IconProps) => (
  <Icon {...p}>
    <path d="m4 17 6-6-6-6M12 19h8" />
  </Icon>
);

export const IconBot = (p: IconProps) => (
  <Icon {...p}>
    <rect x="4" y="8" width="16" height="12" rx="2" />
    <path d="M12 8V4M8 4h8M2 14h2M20 14h2M9 13v2M15 13v2" />
  </Icon>
);

export const IconAlert = (p: IconProps) => (
  <Icon {...p}>
    <path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
    <path d="M12 9v4M12 17h.01" />
  </Icon>
);

export const IconClock = (p: IconProps) => (
  <Icon {...p}>
    <circle cx="12" cy="12" r="9" />
    <path d="M12 7v5l3 2" />
  </Icon>
);

export const IconStop = (p: IconProps) => (
  <Icon {...p}>
    <rect x="6" y="6" width="12" height="12" rx="2" />
  </Icon>
);

// ─── 空状态起点卡片 ───

export const IconSearch = (p: IconProps) => (
  <Icon {...p}>
    <circle cx="11" cy="11" r="7" />
    <path d="m20 20-3.5-3.5" />
  </Icon>
);

export const IconHammer = (p: IconProps) => (
  <Icon {...p}>
    <path d="m15 12-8.5 8.5a2.1 2.1 0 0 1-3-3L12 9" />
    <path d="M17.6 14.5 21 11l-8-8-3.5 3.5" />
    <path d="m13 5 6 6" />
  </Icon>
);

export const IconEye = (p: IconProps) => (
  <Icon {...p}>
    <path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7-10-7-10-7z" />
    <circle cx="12" cy="12" r="3" />
  </Icon>
);

export const IconBug = (p: IconProps) => (
  <Icon {...p}>
    <rect x="8" y="6" width="8" height="14" rx="4" />
    <path d="M8 10 4 8M16 10l4-2M8 14H3M16 14h5M8 18l-4 2M16 18l4 2M9 6a3 3 0 0 1 6 0" />
  </Icon>
);

export const IconSpark = (p: IconProps) => (
  <Icon {...p}>
    <path d="M12 3.5 13.8 9l5.7 1.8-5.7 1.8L12 18.5 10.2 12.6 4.5 10.8 10.2 9z" />
    <path d="M18.5 3v3M20 4.5h-3" />
  </Icon>
);
