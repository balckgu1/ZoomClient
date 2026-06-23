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
  const html = useMemo(() => {
    try {
      return marked.parse(content || "") as string;
    } catch {
      return content || "";
    }
  }, [content]);

  return (
    <div class="message assistant-message">
      <div class="avatar">🤖</div>
      <div class={`bubble assistant-bubble${streaming ? " streaming" : ""}`}>
        <div class="md-content" dangerouslySetInnerHTML={{ __html: html }} />
        {streaming && <span class="typing-cursor">|</span>}
      </div>
    </div>
  );
}
