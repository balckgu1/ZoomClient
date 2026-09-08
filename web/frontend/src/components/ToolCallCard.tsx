import { useState } from "preact/hooks";
import { IconCheck, IconChevronRight, IconClose, IconWrench } from "../lib/icons";

interface Props {
  name: string;
  args: string;
  result?: string;
  isError?: boolean;
}

/** 安全的字符串截断，正确处理 Unicode 多字节字符（如 emoji、中文） */
function safeTruncate(str: string, maxLen: number): string {
  const chars = Array.from(str);
  if (chars.length <= maxLen) return str;
  return chars.slice(0, maxLen).join("") + "…";
}

const RESULT_PREVIEW_LEN = 300;

// ToolCallCard —— 工具调用：蓝色色轨 + 可折叠参数与结果。
// 头部一行承载"名字 + 状态 + 行数/字节 + 展开箭头"，正文按需展开，
// 让频繁的工具调用在对话流里保持低视觉噪音。
export function ToolCallCard({ name, args, result, isError }: Props) {
  const isPending = result === undefined;
  const [expanded, setExpanded] = useState(false);
  const toggle = () => setExpanded((v) => !v);

  const lines = result ? result.split("\n").length : 0;
  const bytes = result ? result.length : 0;
  const needsTruncate = result !== undefined && result.length > RESULT_PREVIEW_LEN;
  const displayResult = expanded || !needsTruncate
    ? result || ""
    : safeTruncate(result || "", RESULT_PREVIEW_LEN);

  // 状态图标：等待=旋转的扳手、成功=绿勾、失败=红叉
  const stateClass = isPending ? "tool__state--wait" : isError ? "tool__state--err" : "tool__state--ok";
  const stateIcon = isPending
    ? <IconWrench size={13} />
    : isError
      ? <IconClose size={13} />
      : <IconCheck size={13} />;

  return (
    <div class={`rail rail--tool ${isError ? "is-error" : ""} ${isPending ? "is-pending" : ""}`}>
      <button class="tool__head" onClick={toggle} aria-expanded={expanded}>
        <span class="tool__kind">tool</span>
        <span class="tool__name">{name}</span>
        <span class={`tool__state ${stateClass}`}>{stateIcon}</span>
        {result !== undefined && (
          <span class="tool__brief">
            <span>{lines} 行 / {bytes} 字节</span>
          </span>
        )}
        <span class="tool__chev"><IconChevronRight size={13} /></span>
      </button>

      {(args || result !== undefined) && expanded && (
        <div class="tool__body">
          {args && (
            <>
              <span class="tool__label">参数</span>
              <pre class="tool__args">{args}</pre>
            </>
          )}
          {result !== undefined && (
            <>
              <span class="tool__label">结果</span>
              <pre class={`tool__out ${isError ? "tool__out--err" : ""}`}>{displayResult}</pre>
            </>
          )}
        </div>
      )}
    </div>
  );
}
