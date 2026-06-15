interface Props {
  content: string;
}

export function UserMessage({ content }: Props) {
  return (
    <div class="message user-message">
      <div class="bubble user-bubble">{content}</div>
    </div>
  );
}
