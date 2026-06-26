import { useState } from "preact/hooks";

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

export function ToolCallCard({ name, args, result, isError }: Props) {
  const isPending = result === undefined;
  const statusIcon = isPending ? null : isError ? "❌" : "✅";
  const [expanded, setExpanded] = useState(false);

  const lines = result ? result.split("\n").length : 0;
  const needsTruncate = result !== undefined && result.length > RESULT_PREVIEW_LEN;
  const displayResult = expanded || !needsTruncate
    ? result || ""
    : safeTruncate(result || "", RESULT_PREVIEW_LEN);

  return (
    <div class={`message tool-card ${isError ? "tool-error" : ""}`}>
      <div class="tool-header">
        {isPending ? (
          <span class="tool-pending-icon">⏳</span>
        ) : (
          <span class="tool-icon">{statusIcon}</span>
        )}
        <span class="tool-name">{name}</span>
      </div>
      {args && <pre class="tool-args">{args}</pre>}
      {result !== undefined && (
        <div class="tool-result">
          <span class="tool-meta">
            {lines} lines / {result.length} bytes
            {needsTruncate && (
              <button
                class="tool-expand-btn"
                onClick={(e) => { e.stopPropagation(); setExpanded(!expanded); }}
              >
                {expanded ? "▲ Collapse" : "▼ Expand"}
              </button>
            )}
          </span>
          <pre class="tool-result-content">{displayResult}</pre>
        </div>
      )}
    </div>
  );
}
