import { useMemo } from "preact/hooks";
import { marked } from "marked";

// Configure marked for GFM and line breaks
marked.setOptions({
  gfm: true,
  breaks: true,
});

interface Props {
  content: string;
  streaming?: boolean;
}

export function AssistantMessage({ content, streaming }: Props) {
  // 流式输出时使用纯文本渲染，避免每帧都重新解析 Markdown（性能优化）
  const html = useMemo(() => {
    if (streaming) return null; // 流式时不解析
    try {
      return marked.parse(content || "") as string;
    } catch {
      return content || "";
    }
  }, [content, streaming]);

  return (
    <div class="message assistant-message">
      <div class="avatar">🤖</div>
      <div class={`bubble assistant-bubble${streaming ? " streaming" : ""}`}>
        {streaming ? (
          <span class="md-plaintext">{content || ""}</span>
        ) : (
          <div class="md-content" dangerouslySetInnerHTML={{ __html: html || "" }} />
        )}
        {streaming && <span class="typing-cursor">|</span>}
      </div>
    </div>
  );
}
