interface Props {
  content: string;
}

export function AssistantMessage({ content }: Props) {
  return (
    <div class="message assistant-message">
      <div class="avatar">🤖</div>
      <div class="bubble assistant-bubble">{content}</div>
    </div>
  );
}
