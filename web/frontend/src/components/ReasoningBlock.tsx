interface Props {
  content: string;
}

export function ReasoningBlock({ content }: Props) {
  return (
    <div class="message reasoning-message">
      <div class="avatar">💭</div>
      <details class="reasoning-details">
        <summary>Thinking...</summary>
        <pre class="reasoning-content">{content}</pre>
      </details>
    </div>
  );
}
