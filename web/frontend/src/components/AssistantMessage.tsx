import { useMemo } from "preact/hooks";
import { marked } from "marked";
import DOMPurify from "dompurify";

// Configure marked for GFM and line breaks
marked.setOptions({
  gfm: true,
  breaks: true,
});

// 外链一律在新标签打开：应用本身是单页界面，点击 <a href> 在原标签导航会把整个
// 会话界面替换成目标页面。用 hook 在消毒后补属性，而不是用正则改写 HTML。
DOMPurify.addHook("afterSanitizeAttributes", (node) => {
  if (node.tagName === "A" && node.getAttribute("href")) {
    node.setAttribute("target", "_blank");
    node.setAttribute("rel", "noopener noreferrer");
  }
});

interface Props {
  content: string;
  streaming?: boolean;
}

// AssistantMessage —— 智能体回复：满宽纯正文，不戴气泡不戴头像。
// 流式输出时退化为纯文本 + 光标，避免每帧重新解析 Markdown（性能优化）。
export function AssistantMessage({ content, streaming }: Props) {
  // 流式时使用纯文本渲染，逐字符更新不高频触发 marked 解析。
  //
  // marked 不做任何 HTML 转义（<img src=x onerror=...>、<iframe>、javascript: 链接
  // 都会原样输出），而这里的内容来自模型——模型会读网页、读文件、跑工具，
  // 一条 prompt injection 就足以让恶意标签进入 innerHTML 并执行脚本。
  // 所以必须先用 DOMPurify 消毒，再交给 dangerouslySetInnerHTML。
  const html = useMemo(() => {
    if (streaming) return null;
    try {
      const raw = marked.parse(content || "") as string;
      // USE_PROFILES 只放行 HTML 语法，顺带丢掉 svg/mathml 这类已知的 mXSS 载体
      return DOMPurify.sanitize(raw, { USE_PROFILES: { html: true } });
    } catch {
      // 解析失败时退回纯文本分支（Preact 渲染文本会自动转义），绝不直接回填原文
      return null;
    }
  }, [content, streaming]);

  return (
    <div class="msg-assistant">
      {html === null ? (
        <>
          <span class="md-plaintext">{content || ""}</span>
          {streaming && <span class="caret" aria-hidden="true" />}
        </>
      ) : (
        <div class="md-content" dangerouslySetInnerHTML={{ __html: html }} />
      )}
    </div>
  );
}
