interface Props {
  name: string;
  args: string;
  result?: string;
  isError?: boolean;
}

export function ToolCallCard({ name, args, result, isError }: Props) {
  const statusIcon = result === undefined ? "⏳" : isError ? "❌" : "✅";
  const lines = result ? result.split("\n").length : 0;

  return (
    <div class={`message tool-card ${isError ? "tool-error" : ""}`}>
      <div class="tool-header">
        <span class="tool-icon">{statusIcon}</span>
        <span class="tool-name">{name}</span>
      </div>
      {args && <pre class="tool-args">{args}</pre>}
      {result !== undefined && (
        <div class="tool-result">
          <span class="tool-meta">{lines} lines / {result.length} bytes</span>
          <pre class="tool-result-content">{result.length > 300 ? result.slice(0, 300) + "…" : result}</pre>
        </div>
      )}
    </div>
  );
}
