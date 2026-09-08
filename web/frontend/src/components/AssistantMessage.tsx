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

// AssistantMessage —— 智能体回复：满宽纯正文，不戴气泡不戴头像。
// 流式输出时退化为纯文本 + 光标，避免每帧重新解析 Markdown（性能优化）。
export function AssistantMessage({ content, streaming }: Props) {
  // 流式时使用纯文本渲染，逐字符更新不高频触发 marked 解析
  const html = useMemo(() => {
    if (streaming) return null;
    try {
      return marked.parse(content || "") as string;
    } catch {
      return content || "";
    }
  }, [content, streaming]);

  return (
    <div class="msg-assistant">
      {streaming ? (
        <>
          <span class="md-plaintext">{content || ""}</span>
          <span class="caret" aria-hidden="true" />
        </>
      ) : (
        <div class="md-content" dangerouslySetInnerHTML={{ __html: html || "" }} />
      )}
    </div>
  );
}
